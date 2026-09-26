package cli

// pp:data-source auto

import (
	"context"
	"database/sql"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"openpayments-pp-cli/internal/op"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		addNovelCommandIfAbsent(root, newSQLCmd(flags))
		addNovelCommandIfAbsent(root, newSchemaCmd(flags))
		addNovelCommandIfAbsent(root, newQueryCmd(flags))
	})
}

// dkanRefRE matches "FROM general:2024" style references inside bracket SQL.
var dkanRefRE = regexp.MustCompile(`(?i)FROM\s+(general|research|ownership):(\d{4})`)

func newSQLCmd(flags *rootFlags) *cobra.Command {
	var limit int
	cmd := &cobra.Command{
		Use:   "sql <query>",
		Short: "Run read-only SQL against the local store, or DKAN bracket SQL against the live API",
		Long: `Runs a read-only SELECT against the local SQLite store. Read 'schema --json'
for tables and columns; amounts are REAL USD, dates are YYYY-MM-DD.

A query that starts with '[' is DKAN bracket SQL and runs against the live
CMS API. 'FROM general:2024' is rewritten to that year's distribution ID,
and Title_Case result keys are normalized to snake_case.`,
		Example: `  openpayments-pp-cli sql "SELECT company, ROUND(SUM(amount)) total FROM payments_general WHERE state='PA' AND program_year=2024 GROUP BY company ORDER BY total DESC LIMIT 10"
  openpayments-pp-cli sql "[SELECT record_id,total_amount_of_payment_usdollars FROM general:2024][WHERE recipient_state = \"PA\"][LIMIT 5]"`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if helpOnly(cmd, args) {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "sql")
			}
			if len(args) == 0 {
				return usageErr(fmt.Errorf("a query argument is required"))
			}
			q := strings.TrimSpace(strings.Join(args, " "))
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			if strings.HasPrefix(q, "[") || flags.dataSource == "live" {
				return runRemoteSQL(ctx, cmd, flags, q)
			}
			_, db, err := openOPStoreRead(ctx)
			if err != nil {
				return err
			}
			rows, err := queryLocal(ctx, db, q, limit)
			if err != nil {
				return usageErr(err)
			}
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().IntVar(&limit, "limit", 1000, "Maximum rows returned from a local query (0 = no cap)")
	return cmd
}

var writeSQLRE = regexp.MustCompile(`(?i)^\s*(insert|update|delete|drop|alter|create|replace|attach|detach|pragma|vacuum|reindex)\b`)

// queryLocal runs a read-only statement and returns rows as maps.
func queryLocal(ctx context.Context, db *sql.DB, q string, limit int) ([]map[string]any, error) {
	if writeSQLRE.MatchString(q) {
		return nil, fmt.Errorf("sql is read-only: only SELECT/WITH statements are allowed")
	}
	rows, err := db.QueryContext(ctx, q)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cols, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	out := []map[string]any{}
	for rows.Next() {
		vals := make([]any, len(cols))
		ptrs := make([]any, len(cols))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return nil, err
		}
		m := make(map[string]any, len(cols))
		for i, c := range cols {
			if b, ok := vals[i].([]byte); ok {
				m[c] = string(b)
			} else {
				m[c] = vals[i]
			}
		}
		out = append(out, m)
		if limit > 0 && len(out) >= limit {
			break
		}
	}
	return out, rows.Err()
}

func runRemoteSQL(ctx context.Context, cmd *cobra.Command, flags *rootFlags, q string) error {
	_, db, err := openOPStore(ctx)
	if err != nil {
		return err
	}
	var rerr error
	q = dkanRefRE.ReplaceAllStringFunc(q, func(m string) string {
		sub := dkanRefRE.FindStringSubmatch(m)
		year, _ := strconv.Atoi(sub[2])
		d, err := resolveDataset(ctx, flags, db, year, strings.ToLower(sub[1]))
		if err != nil {
			rerr = err
			return m
		}
		return "FROM " + d.DistributionID
	})
	if rerr != nil {
		return rerr
	}
	c, err := flags.newClient()
	if err != nil {
		return err
	}
	rows, err := op.RunSQL(ctx, c, q)
	if err != nil {
		return apiErr(err)
	}
	cmd.Annotations["pp:data-source"] = "live"
	return flags.printJSON(cmd, rows)
}

// SchemaColumn describes one column for agents.
type SchemaColumn struct {
	Name string `json:"name"`
	Type string `json:"type"`
}

// SchemaTable describes one table for agents.
type SchemaTable struct {
	Name        string         `json:"name"`
	Description string         `json:"description"`
	Rows        int64          `json:"rows"`
	Columns     []SchemaColumn `json:"columns"`
}

func describeSchema(ctx context.Context, db *sql.DB) ([]SchemaTable, error) {
	var out []SchemaTable
	for _, t := range op.DomainTables {
		st := SchemaTable{Name: t, Description: op.TableDescriptions[t]}
		rows, err := db.QueryContext(ctx, "SELECT name, type FROM pragma_table_info(?)", t)
		if err != nil {
			return nil, err
		}
		for rows.Next() {
			var c SchemaColumn
			if err := rows.Scan(&c.Name, &c.Type); err != nil {
				rows.Close()
				return nil, err
			}
			st.Columns = append(st.Columns, c)
		}
		rows.Close()
		_ = db.QueryRowContext(ctx, "SELECT COUNT(*) FROM "+t).Scan(&st.Rows)
		out = append(out, st)
	}
	return out, nil
}

func newSchemaCmd(flags *rootFlags) *cobra.Command {
	var table string
	cmd := &cobra.Command{
		Use:   "schema",
		Short: "Describe every local table and column so agents can write their own SQL",
		Example: `  openpayments-pp-cli schema --json
  openpayments-pp-cli schema --table payments_research`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "schema")
			}
			if err := validateDataSourceStrategy(flags, "local"); err != nil {
				return usageErr(err)
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			_, db, err := openOPStore(ctx)
			if err != nil {
				return err
			}
			tables, err := describeSchema(ctx, db)
			if err != nil {
				return err
			}
			if table != "" {
				for _, t := range tables {
					if t.Name == table {
						return flags.printJSON(cmd, t)
					}
				}
				return notFoundErr(fmt.Errorf("unknown table %q; run 'schema' to list tables", table))
			}
			return flags.printJSON(cmd, tables)
		},
	}
	cmd.Flags().StringVar(&table, "table", "", "Describe only this table")
	return cmd
}

func newQueryCmd(flags *rootFlags) *cobra.Command {
	var year, limit, offset int
	var typ, sum, sortBy string
	var where, props, groupBy []string
	var all, count bool
	cmd := &cobra.Command{
		Use:   "query",
		Short: "Query a CMS dataset live with filters, columns, sums and sorts (DKAN datastore)",
		Long: `Runs GET /datastore/query against the live API using a program year and type
instead of dataset UUIDs. Filters use property<op>value with = != > >= < <=
or ~ (contains). A comma list after = becomes IN. --sum with --group-by runs
server-side aggregation (exact totals without downloading rows). Pages are
capped at 500 rows by CMS; --all paginates.

Wildcard (~) filters on 15M-row General years can take 30 s; prefer exact
filters or sync the scope and use 'sql'.`,
		Example: `  openpayments-pp-cli query --year 2024 --type general --where recipient_state=PA --where covered_recipient_npi=1234567890 --limit 20
  openpayments-pp-cli query --year 2024 --type general --where recipient_state=PA --group-by applicable_manufacturer_or_applicable_gpo_making_payment_name --sum total_amount_of_payment_usdollars --sort total:desc --limit 10`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "live"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if helpOnly(cmd, args) {
				return cmd.Help()
			}
			if dryRunOK(flags) && year == 0 {
				return writeDryRun(cmd.OutOrStdout(), flags, "query")
			}
			if err := validateDataSourceStrategy(flags, "live"); err != nil {
				return usageErr(err)
			}
			if year == 0 {
				return usageErr(fmt.Errorf("--year is required (2019-2025)"))
			}
			q := op.Query{Limit: limit, Offset: offset, Count: count, Properties: props, Groupings: groupBy}
			for _, w := range where {
				c, err := op.ParseCondition(w)
				if err != nil {
					return usageErr(err)
				}
				q.Conditions = append(q.Conditions, c)
			}
			if sum != "" {
				q.Aggregates = append(q.Aggregates, op.Aggregate{Operator: "sum", Property: sum, Alias: "total"})
				q.Properties = append(append([]string{}, groupBy...), props...)
			}
			if sortBy != "" {
				prop, dir, _ := strings.Cut(sortBy, ":")
				q.Sorts = append(q.Sorts, op.Sort{Property: prop, Desc: strings.EqualFold(dir, "desc")})
			}
			if err := q.Validate(); err != nil {
				return usageErr(err)
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			_, db, err := openOPStore(ctx)
			if err != nil {
				return err
			}
			d, err := resolveDataset(ctx, flags, db, year, typ)
			if err != nil {
				return err
			}
			if flags.dryRun {
				return flags.printJSON(cmd, map[string]any{"dry_run": true, "dataset": d.Title, "path": "/api/1/datastore/query/" + d.DatasetID + "/0", "params": q.Values()})
			}
			c, err := flags.newClient()
			if err != nil {
				return err
			}
			var rows []map[string]any
			var total *int
			for {
				resp, err := op.RunQuery(ctx, c, d.DatasetID, q)
				if err != nil {
					return apiErr(err)
				}
				if resp.Count != nil {
					total = resp.Count
				}
				for _, r := range resp.Results {
					r["_program_year"] = year
					r["_dataset_modified"] = d.Modified
					rows = append(rows, r)
				}
				if !all || len(resp.Results) < pageLimit(q) {
					break
				}
				q.Offset += len(resp.Results)
				q.Count = false
			}
			if count && total != nil {
				return flags.printJSON(cmd, map[string]any{"count": *total, "dataset": d.Title, "dataset_modified": d.Modified, "results": rows})
			}
			return flags.printJSON(cmd, rows)
		},
	}
	cmd.Flags().IntVar(&year, "year", 0, "Program year (2019-2025)")
	cmd.Flags().StringVar(&typ, "type", op.TypeGeneral, "general, research or ownership")
	cmd.Flags().StringArrayVar(&where, "where", nil, "Filter property<op>value (repeatable), e.g. recipient_state=PA")
	cmd.Flags().StringSliceVar(&props, "columns", nil, "Columns to return (comma list); default all")
	cmd.Flags().StringSliceVar(&groupBy, "group-by", nil, "Group by these columns (use with --sum)")
	cmd.Flags().StringVar(&sum, "sum", "", "Sum this numeric column per group (alias 'total')")
	cmd.Flags().StringVar(&sortBy, "sort", "", "Sort by column or alias, e.g. total:desc")
	cmd.Flags().IntVar(&limit, "limit", 100, "Rows per page (CMS max 500)")
	cmd.Flags().IntVar(&offset, "offset", 0, "Row offset")
	cmd.Flags().BoolVar(&all, "all", false, "Paginate through every matching row (can be slow)")
	cmd.Flags().BoolVar(&count, "count", false, "Include the total matching row count (adds ~5 s on large years)")
	return cmd
}

func pageLimit(q op.Query) int {
	if q.Limit <= 0 || q.Limit > op.MaxPageSize {
		return op.MaxPageSize
	}
	return q.Limit
}

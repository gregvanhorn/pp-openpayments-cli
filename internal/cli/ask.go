// Copyright 2026 Claude and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source computed

package cli

import (
	"sort"
	"strings"

	"github.com/spf13/cobra"

	"openpayments-pp-cli/internal/op"
)

func newNovelAskCmd(flags *rootFlags) *cobra.Command {
	var examples int
	cmd := &cobra.Command{
		Use:   "ask <question>",
		Short: "Print the local schema, a plain-word glossary and worked examples for any question",
		Long: `No LLM is embedded. ask returns what a calling agent needs to answer a
question itself: the closest worked examples (commands and SQL) ranked by
keyword overlap, the plain-word glossary (e.g. "company" → company), and every
local table with columns and row counts.

Use this command when no dedicated command fits and you need schema and
examples to write SQL. Do NOT use it to run a query; use 'sql' instead.`,
		Example:     `  openpayments-pp-cli ask "top companies paying PA pain doctors"`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "computed"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if helpOnly(cmd, args) {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "ask")
			}
			q := strings.ToLower(strings.Join(args, " "))
			words := map[string]bool{}
			for _, w := range strings.FieldsFunc(q, func(r rune) bool { return !(r >= 'a' && r <= 'z' || r >= '0' && r <= '9') }) {
				words[w] = true
			}
			type scored struct {
				op.Example
				Score int `json:"score"`
			}
			var ranked []scored
			for _, e := range op.Examples {
				s := 0
				for _, k := range e.Keywords {
					if words[k] {
						s += 2
					}
				}
				for _, w := range strings.Fields(strings.ToLower(e.Question)) {
					if len(w) > 3 && words[w] {
						s++
					}
				}
				ranked = append(ranked, scored{e, s})
			}
			sort.SliceStable(ranked, func(i, j int) bool { return ranked[i].Score > ranked[j].Score })
			if examples > 0 && len(ranked) > examples {
				ranked = ranked[:examples]
			}
			out := map[string]any{"question": strings.Join(args, " "), "examples": ranked, "glossary": op.Glossary,
				"tips": []string{
					"Amounts are REAL USD; dates are YYYY-MM-DD; states are USPS codes.",
					"Match specialties with LIKE '%Pain Medicine%' on specialties (all six slots).",
					"Research PIs live in research_investigators; join on record_id and program_year.",
					"Local data covers only synced scopes; see sync_scopes for what is loaded.",
					"Report published facts only; dispute='Yes' marks recipient-disputed records.",
				}}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			if _, db, err := openOPStore(ctx); err == nil {
				if tables, err := describeSchema(ctx, db); err == nil {
					out["tables"] = tables
				}
				if scopes, err := queryArgs(ctx, db, `SELECT scope, rows, dataset_modified, last_run FROM sync_scopes ORDER BY scope`); err == nil {
					out["synced_scopes"] = scopes
				}
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().IntVar(&examples, "examples", 5, "How many worked examples to return (0 = all)")
	return cmd
}

package cli

// pp:data-source auto

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/spf13/cobra"

	"openpayments-pp-cli/internal/cliutil"
	"openpayments-pp-cli/internal/op"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		addNovelCommandIfAbsent(root, newDatasetsCmd(flags))
		addNovelCommandIfAbsent(root, newDownloadCmd(flags))
	})
}

func newDatasetsCmd(flags *rootFlags) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "datasets",
		Short: "List and resolve CMS Open Payments dataset IDs by program year and type",
		Long: `CMS publishes one dataset per program year and payment type (General,
Research, Ownership) plus pre-grouped summaries and profile tables. Dataset
IDs change on every June publish and January refresh, so the CLI resolves
them from the live metastore and caches them for 24 hours.`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto"},
	}
	cmd.AddCommand(newDatasetsListCmd(flags), newDatasetsResolveCmd(flags))
	return cmd
}

func newDatasetsListCmd(flags *rootFlags) *cobra.Command {
	var year int
	var typ string
	var all, refresh bool
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List datasets with year, type, dataset ID, distribution ID and modified date",
		Example: `  openpayments-pp-cli datasets list --year 2024
  openpayments-pp-cli datasets list --all --json --select title,dataset_id,modified`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "datasets list")
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			_, db, err := openOPStore(ctx)
			if err != nil {
				return err
			}
			reg, err := resolveRegistry(ctx, flags, db, refresh)
			if err != nil {
				return err
			}
			var out []op.Dataset
			for _, d := range reg {
				isPayment := d.Type == op.TypeGeneral || d.Type == op.TypeResearch || d.Type == op.TypeOwnership
				if !all && !isPayment {
					continue
				}
				if year != 0 && d.Year != year {
					continue
				}
				if typ != "" && d.Type != typ {
					continue
				}
				out = append(out, d)
			}
			return flags.printJSON(cmd, out)
		},
	}
	cmd.Flags().IntVar(&year, "year", 0, "Only this program year (2019-2025)")
	cmd.Flags().StringVar(&typ, "type", "", "Only this type: general, research, ownership (or a summary slug with --all)")
	cmd.Flags().BoolVar(&all, "all", false, "Include pre-grouped summaries and profile tables (74 datasets)")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Re-read the live metastore instead of the 24h cache")
	return cmd
}

func newDatasetsResolveCmd(flags *rootFlags) *cobra.Command {
	var year int
	var typ string
	var refresh bool
	cmd := &cobra.Command{
		Use:   "resolve",
		Short: "Resolve one program year and type to its dataset and distribution IDs",
		Example: `  openpayments-pp-cli datasets resolve --year 2024 --type general
  openpayments-pp-cli datasets resolve --year 2024 --type research --refresh --json`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if helpOnly(cmd, args) {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "datasets resolve")
			}
			if year == 0 || typ == "" {
				return usageErr(fmt.Errorf("--year and --type are required (e.g. --year 2024 --type general)"))
			}
			ctx, cancel := boundCtx(cmd.Context(), flags)
			defer cancel()
			_, db, err := openOPStore(ctx)
			if err != nil {
				return err
			}
			if refresh {
				if _, err := resolveRegistry(ctx, flags, db, true); err != nil {
					return err
				}
			}
			d, err := resolveDataset(ctx, flags, db, year, typ)
			if err != nil {
				return err
			}
			return flags.printJSON(cmd, d)
		},
	}
	cmd.Flags().IntVar(&year, "year", 0, "Program year (2019-2025)")
	cmd.Flags().StringVar(&typ, "type", "", "general, research or ownership")
	cmd.Flags().BoolVar(&refresh, "refresh", false, "Re-read the live metastore first")
	return cmd
}

func newDownloadCmd(flags *rootFlags) *cobra.Command {
	var year int
	var typ, out string
	var force bool
	cmd := &cobra.Command{
		Use:   "download",
		Short: "Download the CMS bulk CSV for a program year and payment type",
		Long: `Streams the official bulk CSV to disk. National General files are several
GB (15M+ rows); use --dry-run to see the URL first, or 'sync --full --bulk
--year Y --type T --states PA' to load only a scope into the local store.`,
		Example: `  openpayments-pp-cli download --year 2024 --type research --dry-run
  openpayments-pp-cli download --year 2024 --type ownership --out ./ownership-2024.csv`,
		// Writes a local file, so it is not advertised as read-only and is
		// hidden from the MCP surface.
		Annotations: map[string]string{"pp:data-source": "live", "mcp:hidden": "true"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if helpOnly(cmd, args) {
				return cmd.Help()
			}
			if year == 0 || typ == "" {
				if dryRunOK(flags) {
					return writeDryRun(cmd.OutOrStdout(), flags, "download")
				}
				return usageErr(fmt.Errorf("--year and --type are required (e.g. --year 2024 --type research)"))
			}
			ctx, cancel := boundSyncCtx(cmd, flags)
			defer cancel()
			_, db, err := openOPStore(ctx)
			if err != nil {
				return err
			}
			d, err := resolveDataset(ctx, flags, db, year, typ)
			if err != nil {
				return err
			}
			if out == "" {
				out = filepath.Base(d.DownloadURL)
			}
			info := map[string]any{"title": d.Title, "url": d.DownloadURL, "out": out, "dataset_modified": d.Modified}
			if flags.dryRun || cliutil.IsAnyHarness() {
				info["dry_run"] = true
				return flags.printJSON(cmd, info)
			}
			if _, statErr := os.Stat(out); statErr == nil && !force {
				return usageErr(fmt.Errorf("%s already exists; pass --force to overwrite", out))
			}
			n, err := downloadTo(ctx, d.DownloadURL, out)
			if err != nil {
				return apiErr(err)
			}
			info["bytes"] = n
			return flags.printJSON(cmd, info)
		},
	}
	cmd.Flags().IntVar(&year, "year", 0, "Program year (2019-2025)")
	cmd.Flags().StringVar(&typ, "type", "", "general, research or ownership")
	cmd.Flags().StringVar(&out, "out", "", "Output file (default: the CMS file name)")
	cmd.Flags().BoolVar(&force, "force", false, "Overwrite --out if it exists")
	return cmd
}

func downloadTo(ctx context.Context, url, out string) (int64, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("User-Agent", op.UserAgent)
	limiter := cliutil.NewAdaptiveLimiter(1)
	if err := limiter.Wait(ctx); err != nil {
		return 0, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusTooManyRequests {
		return 0, &cliutil.RateLimitError{URL: url, RetryAfter: cliutil.RetryAfter(resp)}
	}
	if resp.StatusCode != http.StatusOK {
		return 0, fmt.Errorf("download %s: HTTP %d", url, resp.StatusCode)
	}
	f, err := os.Create(out)
	if err != nil {
		return 0, err
	}
	defer f.Close()
	n, err := io.Copy(f, resp.Body)
	if err == nil {
		fmt.Fprintf(os.Stderr, "wrote %s (%s bytes)\n", out, strconv.FormatInt(n, 10))
	}
	return n, err
}

// runBulkSync loads a bulk CSV into the local store for --full syncs.
func runBulkSync(cmd *cobra.Command, flags *rootFlags, scope op.Scope) error {
	if len(scope.Years) != 1 || len(scope.Types) != 1 {
		return usageErr(fmt.Errorf("--full bulk loads take exactly one --year and one --type (e.g. --full --year 2024 --type research)"))
	}
	if dryRunOK(flags) {
		return writeDryRun(cmd.OutOrStdout(), flags, "sync --full")
	}
	ctx, cancel := boundSyncCtx(cmd, flags)
	defer cancel()
	st, db, err := openOPStore(ctx)
	if err != nil {
		return err
	}
	d, err := resolveDataset(ctx, flags, db, scope.Years[0], scope.Types[0])
	if err != nil {
		return err
	}
	opts := op.BulkOptions{Progress: os.Stderr}
	if cliutil.IsDogfoodEnv() {
		opts.MaxRows = 500
	}
	report, err := op.BulkSync(ctx, db, d, scope.Types[0], scope, opts)
	if err != nil {
		return apiErr(err)
	}
	recordFrameworkSyncState(st, report)
	return flags.printJSON(cmd, report)
}

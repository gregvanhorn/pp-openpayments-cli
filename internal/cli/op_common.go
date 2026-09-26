package cli

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"openpayments-pp-cli/internal/op"
	"openpayments-pp-cli/internal/store"
)

const opCLIName = "openpayments-pp-cli"

// registryMaxAge bounds how long cached dataset IDs are trusted. CMS swaps
// identifiers on every June publish and January refresh.
const registryMaxAge = 24 * time.Hour

// openOPStore opens the local store read-write and ensures the domain schema.
func openOPStore(ctx context.Context) (*store.Store, *sql.DB, error) {
	s, err := store.OpenWithContext(ctx, defaultDBPath(opCLIName))
	if err != nil {
		return nil, nil, fmt.Errorf("opening local store: %w", err)
	}
	if err := op.EnsureSchema(s.DB()); err != nil {
		s.Close()
		return nil, nil, err
	}
	return s, s.DB(), nil
}

// openOPStoreRead opens the local store for a read command. It returns a
// friendly error when nothing has been synced yet.
func openOPStoreRead(ctx context.Context) (*store.Store, *sql.DB, error) {
	path := defaultDBPath(opCLIName)
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return nil, nil, notFoundErr(fmt.Errorf("no local data yet; run '%s sync --years 2024 --states PA' first", opCLIName))
	}
	// Read-write open so schema creation is idempotent on older stores; the
	// commands themselves only issue SELECTs.
	return openOPStore(ctx)
}

// resolveRegistry returns the dataset registry, refreshing from the live
// metastore when the cache is empty, stale, or refresh is requested.
func resolveRegistry(ctx context.Context, flags *rootFlags, db *sql.DB, refresh bool) ([]op.Dataset, error) {
	cached, err := op.LoadRegistry(db)
	if err != nil {
		return nil, err
	}
	fresh := len(cached) > 0 && !refresh
	if fresh {
		if t, perr := time.Parse(time.RFC3339, cached[0].ResolvedAt); perr != nil || time.Since(t) > registryMaxAge {
			fresh = false
		}
	}
	if fresh || flags.dataSource == "local" {
		if len(cached) == 0 {
			return nil, notFoundErr(errors.New("dataset registry is empty; run without --data-source local to resolve dataset IDs"))
		}
		return cached, nil
	}
	c, err := flags.newClient()
	if err != nil {
		return nil, err
	}
	live, err := op.FetchCatalog(ctx, c)
	if err != nil {
		if len(cached) > 0 {
			fmt.Fprintf(os.Stderr, "warning: could not refresh dataset registry (%v); using cached IDs\n", err)
			return cached, nil
		}
		return nil, apiErr(err)
	}
	if err := op.SaveRegistry(db, live); err != nil {
		return nil, err
	}
	return live, nil
}

// resolveDataset finds one dataset by year and type ("general:2024" style
// references are accepted by callers via parseDatasetRef).
func resolveDataset(ctx context.Context, flags *rootFlags, db *sql.DB, year int, typ string) (op.Dataset, error) {
	reg, err := resolveRegistry(ctx, flags, db, false)
	if err != nil {
		return op.Dataset{}, err
	}
	if d, ok := op.Find(reg, year, typ); ok {
		return d, nil
	}
	reg, err = resolveRegistry(ctx, flags, db, true)
	if err != nil {
		return op.Dataset{}, err
	}
	if d, ok := op.Find(reg, year, typ); ok {
		return d, nil
	}
	return op.Dataset{}, notFoundErr(fmt.Errorf("no %s dataset for program year %d (CMS publishes 2019-2025; see 'datasets list')", typ, year))
}

// helpOnly reports a bare invocation that should print help.
func helpOnly(cmd *cobra.Command, args []string) bool {
	return len(args) == 0 && cmd.Flags().NFlag() == 0
}

// splitCSVFlag splits a comma list flag value.
func splitCSVFlag(s string) []string { return op.SplitList(s) }

// upperList upper-cases each entry (state codes).
func upperList(xs []string) []string {
	out := make([]string, len(xs))
	for i, x := range xs {
		out[i] = strings.ToUpper(x)
	}
	return out
}

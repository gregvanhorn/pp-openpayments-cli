// Copyright 2026 Claude and contributors. Licensed under Apache-2.0. See LICENSE.

package cli

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"openpayments-pp-cli/internal/op"
)

const trialCacheAge = 7 * 24 * time.Hour

// newCTGov builds the ClinicalTrials.gov client honoring --timeout.
func newCTGov(flags *rootFlags) *op.CTGov {
	t := flags.timeout
	if t <= 0 {
		t = time.Minute
	}
	return op.NewCTGov(t, 3)
}

// researchCompanies lists distinct research sponsors in the local store.
func researchCompanies(ctx context.Context, db *sql.DB) ([]string, error) {
	rows, err := queryArgs(ctx, db, `SELECT DISTINCT company FROM payments_research WHERE company IS NOT NULL`)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(rows))
	for _, r := range rows {
		out = append(out, fmt.Sprint(r["company"]))
	}
	return out, nil
}

// parseAliases turns repeated --sponsor-alias "CT name=OP name" into a map.
func parseAliases(xs []string) map[string]string {
	m := map[string]string{}
	for _, x := range xs {
		if a, b, ok := strings.Cut(x, "="); ok {
			m[strings.ToLower(strings.TrimSpace(a))] = strings.TrimSpace(b)
		}
	}
	return m
}

// trialsCtx opens the store for a trials command (CT.gov live + OP local).
func trialsCtx(cmd *cobra.Command, flags *rootFlags) (context.Context, context.CancelFunc, *sql.DB, error) {
	ctx, cancel := boundCtx(cmd.Context(), flags)
	st, db, err := openOPStore(ctx)
	if err != nil {
		cancel()
		return nil, nil, nil, err
	}
	emitOPHints(cmd, flags, st, "payments_research")
	return ctx, cancel, db, nil
}

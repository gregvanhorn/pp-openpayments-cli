// Copyright 2026 Greg Van Horn and contributors. Licensed under Apache-2.0. See LICENSE.
// cli-printing-press: novel-scaffold-test
// Novel command scaffold tests. Keep the wiring smoke test and add behavior cases as needed.

package cli

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"

	"openpayments-pp-cli/internal/cliutil/testenv"
)

// TestNovelTrialsSponsorHelpWires smoke-tests that the trials sponsor command
// resolves at runtime and renders useful --help output. Catches wiring
// regressions (missing AddCommand, panicking RunE on --help, etc.) before
// review. Keep this smoke test when adding behavior-specific cases.
func TestNovelTrialsSponsorHelpWires(t *testing.T) {
	testenv.Isolate(t)
	cmd := RootCmd()
	cmd.SetArgs([]string{"trials", "sponsor", "--help"})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("trials sponsor --help error = %v (novel command not wired correctly?)", err)
	}
	help := out.String()
	for _, want := range []string{"Usage:", "sponsor"} {
		if !strings.Contains(help, want) {
			t.Fatalf("trials sponsor --help missing %q in output:\n%s", want, help)
		}
	}
}

// TestSponsorTrialPaymentsSQLParses runs the per-trial join against an empty
// schema so a malformed statement fails here instead of at runtime.
func TestSponsorTrialPaymentsSQLParses(t *testing.T) {
	testenv.Isolate(t)
	ctx := context.Background()
	st, db, err := openOPStore(ctx)
	if err != nil {
		t.Fatalf("openOPStore: %v", err)
	}
	defer st.Close()
	rows, err := queryArgs(ctx, db, sponsorTrialPaymentsSQL, "NCT00000000", "NCT00000000", "NCT00000000")
	if err != nil {
		t.Fatalf("sponsorTrialPaymentsSQL: %v", err)
	}
	if len(rows) != 1 || fmt.Sprint(rows[0]["payments"]) != "0" || fmt.Sprint(rows[0]["pi_names"]) != "" {
		t.Fatalf("unexpected empty-store row: %v", rows)
	}
}

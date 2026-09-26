// Copyright 2026 Claude and contributors. Licensed under Apache-2.0. See LICENSE.
// Novel command scaffold. Implement the RunE body before shipping.
// generate --force preserves implemented bodies; untouched TODO scaffolds may refresh.
// pp:data-source auto
// Supported strategies: auto, local, live, or computed. Change this default deliberately.

package cli

import (
	"github.com/spf13/cobra"
)

func newNovelTrialsCmd(flags *rootFlags) *cobra.Command {

	cmd := &cobra.Command{
		Use:         "trials",
		Short:       "Trial-site intelligence",
		Example:     "  openpayments-pp-cli trials gaps --condition 'low back pain' --state PA,NJ --agent",
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "auto"},
		RunE:        parentNoSubcommandRunE(flags),
	}
	addNovelCommandIfAbsent(cmd, newNovelTrialsGapsCmd(flags))
	addNovelCommandIfAbsent(cmd, newNovelTrialsSponsorCmd(flags))
	return cmd
}

// Copyright 2026 Claude and contributors. Licensed under Apache-2.0. See LICENSE.
// pp:data-source local

package cli

import (
	"database/sql"
	"fmt"
	"sort"

	"github.com/spf13/cobra"

	"openpayments-pp-cli/internal/op"
)

func newNovelNearCmd(flags *rootFlags) *cobra.Command {
	var f opFilter
	var zip, typ string
	var miles float64
	cmd := &cobra.Command{
		Use:   "near",
		Short: "Find paid clinicians within N miles of a ZIP code",
		Long: `Radius search over synced recipients using bundled US Census ZIP (ZCTA)
centroids: distance is ZIP-centroid to ZIP-centroid, not street address.
--type research matches principal investigators by their own ZIP.

Use this command for radius queries around a ZIP. Do NOT use it for
whole-state leaderboards; use 'top' instead. Do NOT use it for ranking
research sites by specialty and state; use 'research-sites' instead.`,
		Example: `  openpayments-pp-cli near --zip 19002 --miles 25 --type research --agent
  openpayments-pp-cli near --zip 19002 --miles 10 --specialty "Orthopaedic Surgery" --company stryker`,
		Annotations: map[string]string{"mcp:read-only": "true", "pp:data-source": "local"},
		RunE: func(cmd *cobra.Command, args []string) error {
			if helpOnly(cmd, args) {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "near")
			}
			if len(zip) < 5 || miles <= 0 {
				return usageErr(fmt.Errorf("--zip (5 digits) and --miles > 0 are required"))
			}
			ctx, cancel, db, err := localCtx(cmd, flags, "payments_general")
			if err != nil {
				return err
			}
			defer cancel()
			if err := op.EnsureZipCentroids(db); err != nil {
				return err
			}
			var lat, lon float64
			if err := db.QueryRowContext(ctx, `SELECT lat, lon FROM zip_centroids WHERE zip5 = ?`, zip[:5]).Scan(&lat, &lon); err != nil {
				if err == sql.ErrNoRows {
					return notFoundErr(fmt.Errorf("unknown ZIP %s (not a Census ZCTA)", zip))
				}
				return err
			}
			minLat, maxLat, minLon, maxLon := op.BoundingBox(lat, lon, miles)
			zips, err := queryArgs(ctx, db, `SELECT zip5, lat, lon FROM zip_centroids WHERE lat BETWEEN ? AND ? AND lon BETWEEN ? AND ?`, minLat, maxLat, minLon, maxLon)
			if err != nil {
				return err
			}
			dist := map[string]float64{}
			var inList []any
			for _, z := range zips {
				d := op.Haversine(lat, lon, toFloat(z["lat"]), toFloat(z["lon"]))
				if d <= miles {
					k := fmt.Sprint(z["zip5"])
					dist[k] = d
					inList = append(inList, k)
				}
			}
			if len(inList) == 0 {
				return printRows(cmd, flags, nil)
			}
			var q string
			var a []any
			switch typ {
			case "general":
				w, fa, err := f.where("p", "specialties", "amount")
				if err != nil {
					return err
				}
				q = `SELECT p.npi, MAX(p.recipient_name) name, MAX(p.specialty) specialty, MAX(p.city) city, MAX(p.state) state, p.zip5, COUNT(*) payments, ROUND(SUM(p.amount),2) total, COUNT(DISTINCT p.company) companies
					FROM payments_general p WHERE p.zip5 IN (` + qmarks(len(inList)) + `) AND ` + w + ` AND p.npi IS NOT NULL GROUP BY p.npi`
				a = append(append(a, inList...), fa...)
			case "research":
				pf := f
				pf.states, pf.specialties = "", ""
				w, fa, err := pf.where("p", "specialties", "amount")
				if err != nil {
					return err
				}
				iw, ia, err := opFilter{states: f.states, specialties: f.specialties}.where("i", "specialty", "")
				if err != nil {
					return err
				}
				q = `SELECT i.npi, MAX(i.name) name, MAX(i.specialty) specialty, MAX(i.city) city, MAX(i.state) state, i.zip5, COUNT(*) payments, ROUND(SUM(p.amount),2) total,
					COUNT(DISTINCT p.nct_id) trials, GROUP_CONCAT(DISTINCT p.company) sponsors
					FROM research_investigators i JOIN payments_research p ON p.record_id = i.record_id AND p.program_year = i.program_year
					WHERE i.zip5 IN (` + qmarks(len(inList)) + `) AND ` + w + ` AND ` + iw + ` GROUP BY COALESCE(i.npi, i.name)`
				a = append(append(append(a, inList...), fa...), ia...)
			default:
				return usageErr(fmt.Errorf("--type must be general or research"))
			}
			rows, err := queryArgs(ctx, db, q, a...)
			if err != nil {
				return err
			}
			for _, r := range rows {
				r["miles"] = roundCents(dist[fmt.Sprint(r["zip5"])])
			}
			sort.SliceStable(rows, func(i, j int) bool { return toFloat(rows[i]["total"]) > toFloat(rows[j]["total"]) })
			if len(rows) > f.limit {
				rows = rows[:f.limit]
			}
			return printRows(cmd, flags, rows)
		},
	}
	addFilterFlags(cmd, &f, 50)
	cmd.Flags().StringVar(&zip, "zip", "", "Center ZIP code (5 digits)")
	cmd.Flags().Float64Var(&miles, "miles", 25, "Radius in miles")
	cmd.Flags().StringVar(&typ, "type", "general", "general (covered recipients) or research (principal investigators)")
	return cmd
}

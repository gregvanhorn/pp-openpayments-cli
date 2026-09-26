package cli

// pp:data-source local

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

func init() {
	registerNovelCommand(func(root *cobra.Command, flags *rootFlags) {
		addNovelCommandIfAbsent(root, newDossierCmd(flags))
		addNovelCommandIfAbsent(root, newCompanyCmd(flags))
		addNovelCommandIfAbsent(root, newTopCmd(flags))
		addNovelCommandIfAbsent(root, newResearchCmd(flags))
		addNovelCommandIfAbsent(root, newHospitalCmd(flags))
		addNovelCommandIfAbsent(root, newOwnershipCmd(flags))
		addNovelCommandIfAbsent(root, newNatureCmd(flags))
		addNovelCommandIfAbsent(root, newProductCmd(flags))
	})
}

var localAnn = map[string]string{"mcp:read-only": "true", "pp:data-source": "local"}

func withAnn(k, v string) map[string]string {
	m := ann()
	m[k] = v
	return m
}

func ann() map[string]string {
	m := make(map[string]string, len(localAnn))
	for k, v := range localAnn {
		m[k] = v
	}
	return m
}

func newDossierCmd(flags *rootFlags) *cobra.Command {
	var f opFilter
	var payments int
	cmd := &cobra.Command{
		Use:   "dossier <npi|name>",
		Short: "Build a full payment dossier for one clinician: by year, company, nature and product",
		Long: `Every synced payment for one recipient: totals by type, by year, by paying
company, by nature of payment and by product, research studies with NCT IDs,
ownership interests, and disputed-payment counts. Reports published facts only.

Use this command for one recipient's full history. Do NOT use it to put
recipients side by side; use 'compare' instead. Do NOT use it for the
per-company tenure timeline; use 'relationships' instead.`,
		Example: `  openpayments-pp-cli dossier 1234567890
  openpayments-pp-cli dossier "Smith, Jane" --state PA --payments 20 --json`,
		Annotations: withAnn("pp:happy-args", "<npi>=1234567890"),
		RunE: func(cmd *cobra.Command, args []string) error {
			if helpOnly(cmd, args) {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "dossier")
			}
			if len(args) == 0 {
				return usageErr(fmt.Errorf("an NPI or name is required"))
			}
			ctx, cancel, db, err := localCtx(cmd, flags, "payments_general")
			if err != nil {
				return err
			}
			defer cancel()
			rec, err := pickRecipient(ctx, db, strings.Join(args, " "))
			if err != nil {
				return err
			}
			pred, pargs := recipientPredicate("p", rec)
			fw, fargs, err := f.where("p", "specialties", "amount")
			if err != nil {
				return err
			}
			w := pred + " AND " + fw
			a := append(append([]any{}, pargs...), fargs...)
			out := map[string]any{"recipient": rec}
			sections := []struct{ key, q string }{
				{"totals", `SELECT 'general' type, COUNT(*) payments, SUM(amount) total, MIN(program_year) first_year, MAX(program_year) last_year, SUM(CASE WHEN dispute='Yes' THEN 1 ELSE 0 END) disputed FROM payments_general p WHERE ` + w + `
					UNION ALL SELECT 'research', COUNT(*), SUM(amount), MIN(program_year), MAX(program_year), SUM(CASE WHEN dispute='Yes' THEN 1 ELSE 0 END) FROM payments_research p WHERE ` + w},
				{"by_year", `SELECT program_year, COUNT(*) payments, SUM(amount) total, COUNT(DISTINCT company) companies, MAX(dataset_modified) dataset_modified FROM payments_general p WHERE ` + w + ` GROUP BY program_year ORDER BY program_year`},
				{"by_company", `SELECT company, COUNT(*) payments, SUM(amount) total, MIN(program_year) first_year, MAX(program_year) last_year FROM payments_general p WHERE ` + w + ` GROUP BY company ORDER BY total DESC LIMIT 25`},
				{"by_nature", `SELECT nature, COUNT(*) payments, SUM(amount) total FROM payments_general p WHERE ` + w + ` GROUP BY nature ORDER BY total DESC`},
				{"by_product", `SELECT MAX(name) product, MAX(kind) kind, MAX(category) category, COUNT(*) payments, SUM(amount) total FROM (SELECT p.record_id, p.program_year, MAX(p.amount) amount, LOWER(pr.name) pkey, MAX(pr.name) name, MAX(pr.kind) kind, MAX(pr.category) category FROM payments_general p JOIN products pr ON pr.payment_type='general' AND pr.record_id=p.record_id AND pr.program_year=p.program_year WHERE ` + w + ` AND pr.name IS NOT NULL GROUP BY p.record_id, p.program_year, pkey) GROUP BY pkey ORDER BY total DESC LIMIT 25`},
				{"research_studies", `SELECT nct_id, MAX(name_of_study) name_of_study, company, COUNT(*) payments, SUM(amount) total, MIN(program_year) first_year, MAX(program_year) last_year FROM payments_research p WHERE ` + w + ` GROUP BY nct_id, company ORDER BY total DESC LIMIT 50`},
			}
			for _, s := range sections {
				rows, err := queryArgs(ctx, db, s.q, repeatArgs(s.q, a)...)
				if err != nil {
					return err
				}
				out[s.key] = rows
			}
			pi, err := queryArgs(ctx, db, `SELECT p.nct_id, MAX(p.name_of_study) name_of_study, p.company, COUNT(*) payments, SUM(p.amount) total, MIN(p.program_year) first_year, MAX(p.program_year) last_year
				FROM research_investigators i JOIN payments_research p ON p.record_id=i.record_id AND p.program_year=i.program_year
				WHERE `+strings.Replace(pred, "p.", "i.", 1)+` GROUP BY p.nct_id, p.company ORDER BY total DESC LIMIT 50`, pargs...)
			if err != nil {
				return err
			}
			out["as_principal_investigator"] = pi
			ownPred := strings.Replace(pred, "p.", "o.", 1)
			own, err := queryArgs(ctx, db, `SELECT program_year, company, amount_invested, value_of_interest, terms, held_by, dispute, record_id, dataset_modified FROM payments_ownership o WHERE `+ownPred+` ORDER BY program_year`, pargs...)
			if err != nil {
				return err
			}
			out["ownership"] = own
			if payments > 0 {
				rows, err := queryArgs(ctx, db, `SELECT record_id, program_year, payment_date, company, nature, amount, form, dispute, dataset_modified FROM payments_general p WHERE `+w+` ORDER BY payment_date DESC LIMIT ?`, append(a, payments)...)
				if err != nil {
					return err
				}
				out["payments"] = rows
			}
			return flags.printJSON(cmd, out)
		},
	}
	addFilterFlags(cmd, &f, 25)
	cmd.Flags().IntVar(&payments, "payments", 0, "Also list this many most recent individual payments with record_id")
	return cmd
}

// repeatArgs duplicates args for UNION queries that reuse the predicate.
func repeatArgs(q string, a []any) []any {
	n := strings.Count(q, "WHERE ")
	out := make([]any, 0, len(a)*n)
	for i := 0; i < n; i++ {
		out = append(out, a...)
	}
	return out
}

func newCompanyCmd(flags *rootFlags) *cobra.Command {
	var f opFilter
	cmd := &cobra.Command{
		Use:   "company <name>",
		Short: "Show who a company pays: top recipients, specialties, states, products and natures by year",
		Long: `A paying company's footprint in the synced scope. The name is a
case-insensitive substring ("stryker" matches "Stryker Corporation").

Use this command to list who a company pays. Do NOT use it to measure how
concentrated the spend is; use 'concentration' instead. Do NOT use it to
compare two companies' recipients; use 'overlap' instead.`,
		Example: `  openpayments-pp-cli company stryker --state PA
  openpayments-pp-cli company "Stryker Corporation" --specialty "Orthopaedic Surgery" --state PA --json --select by_year`,
		Annotations: ann(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if helpOnly(cmd, args) {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "company")
			}
			if len(args) == 0 {
				return usageErr(fmt.Errorf("a company name is required"))
			}
			ctx, cancel, db, err := localCtx(cmd, flags, "payments_general")
			if err != nil {
				return err
			}
			defer cancel()
			f.companies = strings.Join(args, " ")
			w, a, err := f.where("p", "specialties", "amount")
			if err != nil {
				return err
			}
			out := map[string]any{"company": f.companies}
			sections := []struct{ key, q string }{
				{"matched_names", `SELECT company, company_id, COUNT(*) payments FROM payments_general p WHERE ` + w + ` GROUP BY company, company_id ORDER BY payments DESC`},
				{"by_year", `SELECT program_year, COUNT(*) payments, COUNT(DISTINCT COALESCE(npi, teaching_hospital_ccn)) recipients, SUM(amount) total, MAX(dataset_modified) dataset_modified FROM payments_general p WHERE ` + w + ` GROUP BY program_year ORDER BY program_year`},
				{"top_recipients", `SELECT npi, MAX(recipient_name) name, MAX(specialty) specialty, MAX(city) city, MAX(state) state, COUNT(*) payments, SUM(amount) total, MIN(program_year) first_year, MAX(program_year) last_year FROM payments_general p WHERE ` + w + ` GROUP BY COALESCE(npi, teaching_hospital_ccn) ORDER BY total DESC LIMIT ` + fmt.Sprint(f.limit)},
				{"by_specialty", `SELECT specialty, COUNT(DISTINCT npi) recipients, SUM(amount) total FROM payments_general p WHERE ` + w + ` AND specialty IS NOT NULL GROUP BY specialty ORDER BY total DESC LIMIT 15`},
				{"by_state", `SELECT state, COUNT(DISTINCT npi) recipients, SUM(amount) total FROM payments_general p WHERE ` + w + ` GROUP BY state ORDER BY total DESC LIMIT 15`},
				{"by_nature", `SELECT nature, COUNT(*) payments, SUM(amount) total FROM payments_general p WHERE ` + w + ` GROUP BY nature ORDER BY total DESC`},
				{"by_product", `SELECT MAX(name) product, MAX(kind) kind, COUNT(*) payments, SUM(amount) total FROM (SELECT p.record_id, p.program_year, MAX(p.amount) amount, LOWER(pr.name) pkey, MAX(pr.name) name, MAX(pr.kind) kind FROM payments_general p JOIN products pr ON pr.payment_type='general' AND pr.record_id=p.record_id AND pr.program_year=p.program_year WHERE ` + w + ` AND pr.name IS NOT NULL GROUP BY p.record_id, p.program_year, pkey) GROUP BY pkey ORDER BY total DESC LIMIT 15`},
				{"research_by_year", `SELECT program_year, COUNT(*) payments, COUNT(DISTINCT nct_id) studies, SUM(amount) total FROM payments_research p WHERE ` + w + ` GROUP BY program_year ORDER BY program_year`},
			}
			for _, s := range sections {
				rows, err := queryArgs(ctx, db, s.q, a...)
				if err != nil {
					return err
				}
				out[s.key] = rows
				if s.key == "matched_names" && len(rows) == 0 {
					return notFoundErr(fmt.Errorf("no match for %q in synced payments (check the name or sync that company's scope)", f.companies))
				}
			}
			return flags.printJSON(cmd, out)
		},
	}
	addFilterFlags(cmd, &f, 20)
	return cmd
}

func newTopCmd(flags *rootFlags) *cobra.Command {
	var f opFilter
	var by, typ string
	cmd := &cobra.Command{
		Use:   "top",
		Short: "Rank top recipients, companies, products, states, specialties or hospitals by dollars",
		Long: `Ranks the synced scope by total dollars.

Use this command for all-nature dollar leaderboards. Do NOT use it to rank
physicians by speaking and consulting activity; use 'kol' instead. Do NOT
use it for radius questions around a ZIP; use 'near' instead.`,
		Example: `  openpayments-pp-cli top --by recipient --state PA --year 2024 --limit 20
  openpayments-pp-cli top --by company --state PA --specialty "Pain Medicine" --year 2019-2025 --limit 10
  openpayments-pp-cli top --by product --state PA --specialty "Neurological Surgery"`,
		Annotations: ann(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "top")
			}
			table := "payments_" + typ
			amount, spec := "amount", "specialties"
			switch typ {
			case "general", "research":
			case "ownership":
				amount, spec = "amount_invested", "specialty"
			default:
				return usageErr(fmt.Errorf("--type must be general, research or ownership"))
			}
			ctx, cancel, db, err := localCtx(cmd, flags, table)
			if err != nil {
				return err
			}
			defer cancel()
			w, a, err := f.where("p", spec, amount)
			if err != nil {
				return err
			}
			var q string
			switch by {
			case "recipient":
				q = `SELECT npi, MAX(recipient_name) name, MAX(specialty) specialty, MAX(city) city, MAX(state) state, COUNT(*) payments, COUNT(DISTINCT company) companies, SUM(` + amount + `) total, MIN(program_year) first_year, MAX(program_year) last_year, MAX(dataset_modified) dataset_modified
					FROM ` + table + ` p WHERE ` + w + ` AND npi IS NOT NULL GROUP BY npi ORDER BY total DESC LIMIT ?`
			case "company":
				q = `SELECT company, COUNT(*) payments, COUNT(DISTINCT npi) recipients, SUM(` + amount + `) total, MIN(program_year) first_year, MAX(program_year) last_year, MAX(dataset_modified) dataset_modified
					FROM ` + table + ` p WHERE ` + w + ` GROUP BY company ORDER BY total DESC LIMIT ?`
			case "product":
				if typ == "ownership" {
					return usageErr(fmt.Errorf("--by product needs --type general or research"))
				}
				// DISTINCT payment × product name, so a product repeated across
				// slots of one payment is counted once.
				q = `SELECT MAX(name) product, MAX(kind) kind, MAX(category) category, COUNT(*) payments, COUNT(DISTINCT npi) recipients, SUM(amount) total, MAX(dataset_modified) dataset_modified FROM (
					SELECT p.record_id, p.program_year, MAX(p.npi) npi, MAX(p.amount) amount, MAX(p.dataset_modified) dataset_modified, LOWER(pr.name) pkey, MAX(pr.name) name, MAX(pr.kind) kind, MAX(pr.category) category
					FROM ` + table + ` p JOIN products pr ON pr.payment_type='` + typ + `' AND pr.record_id=p.record_id AND pr.program_year=p.program_year
					WHERE ` + w + ` AND pr.name IS NOT NULL GROUP BY p.record_id, p.program_year, pkey) GROUP BY pkey ORDER BY total DESC LIMIT ?`
			case "state":
				q = `SELECT state, COUNT(*) payments, COUNT(DISTINCT npi) recipients, SUM(` + amount + `) total FROM ` + table + ` p WHERE ` + w + ` GROUP BY state ORDER BY total DESC LIMIT ?`
			case "specialty":
				q = `SELECT specialty, COUNT(*) payments, COUNT(DISTINCT npi) recipients, SUM(` + amount + `) total FROM ` + table + ` p WHERE ` + w + ` AND specialty IS NOT NULL GROUP BY specialty ORDER BY total DESC LIMIT ?`
			case "hospital":
				if typ == "ownership" {
					return usageErr(fmt.Errorf("--by hospital needs --type general or research"))
				}
				q = `SELECT teaching_hospital_ccn ccn, MAX(teaching_hospital_name) hospital, MAX(city) city, MAX(state) state, COUNT(*) payments, SUM(amount) total FROM ` + table + ` p WHERE ` + w + ` AND teaching_hospital_ccn IS NOT NULL GROUP BY teaching_hospital_ccn ORDER BY total DESC LIMIT ?`
			default:
				return usageErr(fmt.Errorf("--by must be recipient, company, product, state, specialty or hospital"))
			}
			return runLocal(ctx, cmd, flags, db, q, append(a, f.limit)...)
		},
	}
	addFilterFlags(cmd, &f, 20)
	cmd.Flags().StringVar(&by, "by", "recipient", "recipient, company, product, state, specialty or hospital")
	cmd.Flags().StringVar(&typ, "type", "general", "Payment type: general, research or ownership")
	return cmd
}

func newResearchCmd(flags *rootFlags) *cobra.Command {
	var f opFilter
	var nct string
	var hasNCT bool
	cmd := &cobra.Command{
		Use:   "research",
		Short: "List research payments with principal investigators, study name, NCT ID and sponsor",
		Long: `Research payment rows from the local store. --state and --specialty match
the covered recipient OR any of the five principal investigators, so trials
paid to a hospital still surface under the PI's specialty and state.

Use this command for raw research payment rows. Do NOT use it to rank sites
or PIs; use 'research-sites' instead. Do NOT use it for the PI roll-up of one
trial; use 'investigators' instead.`,
		Example: `  openpayments-pp-cli research --state NJ --year 2024 --has-nct
  openpayments-pp-cli research --state PA,NJ --specialty "Pain Medicine" --json --select nct_id,name_of_study,company,amount`,
		Annotations: ann(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "research")
			}
			ctx, cancel, db, err := localCtx(cmd, flags, "payments_research")
			if err != nil {
				return err
			}
			defer cancel()
			pf := f
			pf.states, pf.specialties = "", ""
			w, a, err := pf.where("p", "specialties", "amount")
			if err != nil {
				return err
			}
			if st := upperList(splitCSVFlag(f.states)); len(st) > 0 {
				w += ` AND (p.state IN (` + qmarks(len(st)) + `) OR EXISTS (SELECT 1 FROM research_investigators i WHERE i.record_id=p.record_id AND i.program_year=p.program_year AND i.state IN (` + qmarks(len(st)) + `)))`
				for i := 0; i < 2; i++ {
					for _, s := range st {
						a = append(a, s)
					}
				}
			}
			if sp := splitCSVFlag(f.specialties); len(sp) > 0 {
				var ors []string
				for _, s := range sp {
					ors = append(ors, `LOWER(p.specialties) LIKE ? OR EXISTS (SELECT 1 FROM research_investigators i WHERE i.record_id=p.record_id AND i.program_year=p.program_year AND LOWER(i.specialty) LIKE ?)`)
					a = append(a, "%"+strings.ToLower(s)+"%", "%"+strings.ToLower(s)+"%")
				}
				w += " AND (" + strings.Join(ors, " OR ") + ")"
			}
			if nct != "" {
				w += " AND p.nct_id = ?"
				a = append(a, strings.ToUpper(nct))
			}
			if hasNCT {
				w += " AND p.nct_id IS NOT NULL"
			}
			q := `SELECT p.record_id, p.program_year, p.payment_date, p.nct_id, p.name_of_study, p.company sponsor, p.amount,
				COALESCE(p.recipient_name, p.noncovered_entity) recipient, p.npi, p.teaching_hospital_name, p.city, p.state,
				(SELECT GROUP_CONCAT(i.name || ' (' || COALESCE(i.npi,'') || ', ' || COALESCE(i.city,'') || ' ' || COALESCE(i.state,'') || ')', '; ') FROM research_investigators i WHERE i.record_id=p.record_id AND i.program_year=p.program_year) investigators,
				p.context_of_research, p.preclinical, p.dispute, p.dataset_modified
				FROM payments_research p WHERE ` + w + ` ORDER BY p.amount DESC LIMIT ?`
			return runLocal(ctx, cmd, flags, db, q, append(a, f.limit)...)
		},
	}
	addFilterFlags(cmd, &f, 100)
	cmd.Flags().StringVar(&nct, "nct", "", "Only this ClinicalTrials.gov ID (NCT########)")
	cmd.Flags().BoolVar(&hasNCT, "has-nct", false, "Only payments that cite a ClinicalTrials.gov ID")
	return cmd
}

func newHospitalCmd(flags *rootFlags) *cobra.Command {
	var f opFilter
	var metric string
	cmd := &cobra.Command{
		Use:   "hospital [ccn|name]",
		Short: "Show payments to a teaching hospital, or rank teaching hospitals by research or general dollars",
		Example: `  openpayments-pp-cli hospital --state PA --metric research --limit 20
  openpayments-pp-cli hospital 390111 --json
  openpayments-pp-cli hospital "Hospital of the University of Pennsylvania"`,
		Annotations: ann(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "hospital")
			}
			ctx, cancel, db, err := localCtx(cmd, flags, "payments_general")
			if err != nil {
				return err
			}
			defer cancel()
			w, a, err := f.where("p", "specialties", "amount")
			if err != nil {
				return err
			}
			if len(args) == 0 {
				table := "payments_research"
				if metric == "general" {
					table = "payments_general"
				}
				q := `SELECT p.teaching_hospital_ccn ccn, MAX(p.teaching_hospital_name) hospital, MAX(p.city) city, MAX(p.state) state, COUNT(*) payments, SUM(p.amount) total, MIN(p.program_year) first_year, MAX(p.program_year) last_year, MAX(p.dataset_modified) dataset_modified
					FROM ` + table + ` p WHERE ` + w + ` AND p.teaching_hospital_ccn IS NOT NULL GROUP BY p.teaching_hospital_ccn ORDER BY total DESC LIMIT ?`
				return runLocal(ctx, cmd, flags, db, q, append(a, f.limit)...)
			}
			arg := strings.Join(args, " ")
			hw := "p.teaching_hospital_ccn = ?"
			harg := any(arg)
			if !isDigits(arg) {
				hw = "LOWER(p.teaching_hospital_name) LIKE ?"
				harg = "%" + strings.ToLower(arg) + "%"
			}
			out := map[string]any{"hospital": arg}
			found := false
			for _, s := range []struct{ key, table string }{{"general", "payments_general"}, {"research", "payments_research"}} {
				rows, err := queryArgs(ctx, db, `SELECT p.teaching_hospital_ccn ccn, MAX(p.teaching_hospital_name) hospital, p.program_year, COUNT(*) payments, SUM(p.amount) total, COUNT(DISTINCT p.company) companies
					FROM `+s.table+` p WHERE `+hw+` AND `+w+` GROUP BY p.teaching_hospital_ccn, p.program_year ORDER BY p.program_year`, append([]any{harg}, a...)...)
				if err != nil {
					return err
				}
				out[s.key+"_by_year"] = rows
				top, err := queryArgs(ctx, db, `SELECT p.company, COUNT(*) payments, SUM(p.amount) total FROM `+s.table+` p WHERE `+hw+` AND `+w+` GROUP BY p.company ORDER BY total DESC LIMIT ?`, append(append([]any{harg}, a...), f.limit)...)
				if err != nil {
					return err
				}
				out[s.key+"_top_companies"] = top
				found = found || len(rows) > 0
			}
			if !found {
				return notFoundErr(fmt.Errorf("no match for %q among synced teaching hospitals (use a CCN or part of the name)", arg))
			}
			return flags.printJSON(cmd, out)
		},
	}
	addFilterFlags(cmd, &f, 20)
	cmd.Flags().StringVar(&metric, "metric", "research", "Ranking metric without an argument: research or general")
	return cmd
}

func newOwnershipCmd(flags *rootFlags) *cobra.Command {
	var f opFilter
	var groupBy string
	cmd := &cobra.Command{
		Use:   "ownership",
		Short: "List physician ownership and investment interests, individually or grouped by company",
		Example: `  openpayments-pp-cli ownership --state NJ --group-by company
  openpayments-pp-cli ownership --state PA --group-by none --json`,
		Annotations: ann(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "ownership")
			}
			ctx, cancel, db, err := localCtx(cmd, flags, "payments_ownership")
			if err != nil {
				return err
			}
			defer cancel()
			w, a, err := f.where("p", "specialty", "amount_invested")
			if err != nil {
				return err
			}
			var q string
			switch groupBy {
			case "company":
				q = `SELECT company, COUNT(*) interests, COUNT(DISTINCT npi) physicians, SUM(amount_invested) amount_invested, SUM(value_of_interest) value_of_interest, MIN(program_year) first_year, MAX(program_year) last_year, MAX(dataset_modified) dataset_modified FROM payments_ownership p WHERE ` + w + ` GROUP BY company ORDER BY value_of_interest DESC LIMIT ?`
			case "physician":
				q = `SELECT npi, MAX(recipient_name) name, MAX(specialty) specialty, MAX(city) city, MAX(state) state, COUNT(*) interests, COUNT(DISTINCT company) companies, SUM(amount_invested) amount_invested, SUM(value_of_interest) value_of_interest FROM payments_ownership p WHERE ` + w + ` GROUP BY npi ORDER BY value_of_interest DESC LIMIT ?`
			case "none", "":
				q = `SELECT record_id, program_year, npi, recipient_name, specialty, city, state, company, amount_invested, value_of_interest, terms, held_by, dispute, dataset_modified FROM payments_ownership p WHERE ` + w + ` ORDER BY value_of_interest DESC LIMIT ?`
			default:
				return usageErr(fmt.Errorf("--group-by must be company, physician or none"))
			}
			return runLocal(ctx, cmd, flags, db, q, append(a, f.limit)...)
		},
	}
	addFilterFlags(cmd, &f, 50)
	cmd.Flags().StringVar(&groupBy, "group-by", "company", "company, physician or none")
	return cmd
}

func newNatureCmd(flags *rootFlags) *cobra.Command {
	var f opFilter
	var npi string
	cmd := &cobra.Command{
		Use:   "nature",
		Short: "Break down general payments by nature (consulting, food, travel, royalties...)",
		Example: `  openpayments-pp-cli nature --state PA --year 2024
  openpayments-pp-cli nature --npi 1234567890 --json`,
		Annotations: ann(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "nature")
			}
			ctx, cancel, db, err := localCtx(cmd, flags, "payments_general")
			if err != nil {
				return err
			}
			defer cancel()
			w, a, err := f.where("p", "specialties", "amount")
			if err != nil {
				return err
			}
			if npi != "" {
				w += " AND p.npi = ?"
				a = append(a, npi)
			}
			q := `SELECT nature, COUNT(*) payments, COUNT(DISTINCT npi) recipients, COUNT(DISTINCT company) companies, SUM(amount) total,
				ROUND(100.0 * SUM(amount) / (SELECT SUM(amount) FROM payments_general p WHERE ` + w + `), 2) pct_of_total, MIN(program_year) first_year, MAX(program_year) last_year
				FROM payments_general p WHERE ` + w + ` GROUP BY nature ORDER BY total DESC LIMIT ?`
			return runLocal(ctx, cmd, flags, db, q, append(append(append([]any{}, a...), a...), f.limit)...)
		},
	}
	addFilterFlags(cmd, &f, 50)
	cmd.Flags().StringVar(&npi, "npi", "", "Only this recipient NPI")
	return cmd
}

func newProductCmd(flags *rootFlags) *cobra.Command {
	var f opFilter
	var kind string
	cmd := &cobra.Command{
		Use:   "product <name>",
		Short: "List every clinician paid in connection with a drug, biologic or device",
		Example: `  openpayments-pp-cli product Mako --state PA
  openpayments-pp-cli product "SPINAL CORD STIMULATOR" --kind Device --json`,
		Annotations: ann(),
		RunE: func(cmd *cobra.Command, args []string) error {
			if helpOnly(cmd, args) {
				return cmd.Help()
			}
			if dryRunOK(flags) {
				return writeDryRun(cmd.OutOrStdout(), flags, "product")
			}
			if len(args) == 0 {
				return usageErr(fmt.Errorf("a product name is required"))
			}
			ctx, cancel, db, err := localCtx(cmd, flags, "payments_general")
			if err != nil {
				return err
			}
			defer cancel()
			w, a, err := f.where("p", "specialties", "amount")
			if err != nil {
				return err
			}
			// Each payment counts once even when several product slots match.
			match := "pr.payment_type='general' AND pr.record_id=p.record_id AND pr.program_year=p.program_year AND (LOWER(pr.name) LIKE ? OR LOWER(pr.category) LIKE ?)"
			term := "%" + strings.ToLower(strings.Join(args, " ")) + "%"
			margs := []any{term, term}
			if kind != "" {
				match += " AND LOWER(pr.kind) = ?"
				margs = append(margs, strings.ToLower(kind))
			}
			q := `SELECT p.npi, MAX(p.recipient_name) name, MAX(p.specialty) specialty, MAX(p.city) city, MAX(p.state) state,
				GROUP_CONCAT(DISTINCT (SELECT MIN(pr.name) FROM products pr WHERE ` + match + `)) products, GROUP_CONCAT(DISTINCT p.company) companies,
				COUNT(*) payments, SUM(p.amount) total, MIN(p.program_year) first_year, MAX(p.program_year) last_year
				FROM payments_general p WHERE ` + w + ` AND EXISTS (SELECT 1 FROM products pr WHERE ` + match + `)
				GROUP BY COALESCE(p.npi, p.teaching_hospital_ccn) ORDER BY total DESC LIMIT ?`
			a = append(append(append([]any{}, margs...), a...), margs...)
			rows, err := queryArgs(ctx, db, q, append(a, f.limit)...)
			if err != nil {
				return err
			}
			if len(rows) == 0 {
				return notFoundErr(fmt.Errorf("no match for %q among synced product names or categories", strings.Join(args, " ")))
			}
			return printRowsDB(cmd, flags, db, rows)
		},
	}
	addFilterFlags(cmd, &f, 50)
	cmd.Flags().StringVar(&kind, "kind", "", "Drug, Biological, Device or Medical Supply")
	return cmd
}

package op

import (
	"context"
	"database/sql"
	"encoding/json"
	"math"
	"net/url"
	"strings"
	"testing"

	_ "modernc.org/sqlite"
)

func TestClassifyTitle(t *testing.T) {
	cases := []struct {
		title string
		year  int
		typ   string
	}{
		{"2024 General Payment Data", 2024, TypeGeneral},
		{"2019 Research Payment Data", 2019, TypeResearch},
		{"2025 Ownership Payment Data", 2025, TypeOwnership},
		{"2024 payments grouped by covered recipient and nature of payments", 2024, "payments-grouped-by-covered-recipient-and-nature-of-payments"},
		{"Reporting entity profile information", 0, "reporting-entity-profile-information"},
	}
	for _, c := range cases {
		y, typ := ClassifyTitle(c.title)
		if y != c.year || typ != c.typ {
			t.Errorf("ClassifyTitle(%q) = %d,%q want %d,%q", c.title, y, typ, c.year, c.typ)
		}
	}
}

func TestParseYears(t *testing.T) {
	cases := map[string][]int{"2024": {2024}, "2019-2021": {2019, 2020, 2021}, "2023,2025": {2023, 2025}}
	for in, want := range cases {
		got, err := ParseYears(in)
		if err != nil || len(got) != len(want) {
			t.Fatalf("ParseYears(%q) = %v, %v", in, got, err)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Errorf("ParseYears(%q)[%d] = %d want %d", in, i, got[i], want[i])
			}
		}
	}
	if _, err := ParseYears("2025-2019"); err == nil {
		t.Error("reversed range should fail")
	}
	if _, err := ParseYears("abc"); err == nil {
		t.Error("non-numeric year should fail")
	}
}

func TestParseTypes(t *testing.T) {
	got, err := ParseTypes("general, Research")
	if err != nil || len(got) != 2 || got[1] != TypeResearch {
		t.Fatalf("ParseTypes = %v, %v", got, err)
	}
	if all, _ := ParseTypes(""); len(all) != 3 {
		t.Errorf("empty types should default to all three, got %v", all)
	}
	if _, err := ParseTypes("gifts"); err == nil {
		t.Error("unknown type should fail")
	}
}

func TestQueryValues(t *testing.T) {
	q := Query{
		Conditions: []Condition{{Property: "recipient_state", Value: "PA"}, {Property: "date_of_payment", Operator: "between", Value: []string{"01/00/2024", "01/99/2024"}}},
		Properties: []string{"record_id", "program_year"},
		Limit:      2000,
	}
	v := q.Values()
	if v.Get("limit") != "500" {
		t.Errorf("limit must clamp to 500, got %s", v.Get("limit"))
	}
	if v.Get("conditions[0][operator]") != "=" || v.Get("conditions[1][value][1]") != "01/99/2024" {
		t.Errorf("conditions encoded wrong: %v", v)
	}
	if got := v["properties[]"]; len(got) != 2 {
		t.Errorf("plain properties should use append syntax, got %v", v)
	}
	agg := Query{Properties: []string{"company"}, Aggregates: []Aggregate{{"sum", "total_amount_of_payment_usdollars", "total"}}, Groupings: []string{"company"}, Sorts: []Sort{{"total", true}}}
	av := agg.Values()
	if av.Get("properties[1][expression][operator]") != "sum" || av.Get("sorts[0][order]") != "desc" {
		t.Errorf("aggregate encoding wrong: %v", av)
	}
	if (Query{NoResults: true}).Values().Get("results") != "false" {
		t.Error("NoResults should send results=false")
	}
	big := Query{Aggregates: []Aggregate{{"sum", "x", "t"}}, Properties: strings.Split("a,b,c,d,e,f,g,h,i,j", ",")}
	if big.Validate() == nil {
		t.Error("11 indexed properties must be rejected")
	}
}

func TestParseCondition(t *testing.T) {
	cases := []struct {
		in, prop, op string
		val          any
	}{
		{"recipient_state=PA", "recipient_state", "=", "PA"},
		{"total_amount_of_payment_usdollars>=1000", "total_amount_of_payment_usdollars", ">=", "1000"},
		{"company~stryker", "company", "like", "%stryker%"},
		{"recipient_state!=PA", "recipient_state", "<>", "PA"},
	}
	for _, c := range cases {
		got, err := ParseCondition(c.in)
		if err != nil || got.Property != c.prop || got.Operator != c.op || got.Value != c.val {
			t.Errorf("ParseCondition(%q) = %+v, %v", c.in, got, err)
		}
	}
	in, _ := ParseCondition("recipient_state=PA,NJ")
	if in.Operator != "in" {
		t.Errorf("comma list should become IN, got %+v", in)
	}
	if _, err := ParseCondition("nonsense"); err == nil {
		t.Error("missing operator should fail")
	}
}

func TestNormalizers(t *testing.T) {
	if ISODate("10/23/2024") != "2024-10-23" {
		t.Errorf("ISODate = %v", ISODate("10/23/2024"))
	}
	if ISODate("") != nil {
		t.Error("empty date should be nil")
	}
	if Zip5("19002-1234") != "19002" {
		t.Errorf("Zip5 = %v", Zip5("19002-1234"))
	}
	if PersonName("Jane", "Q", "Smith", "Jr.") != "Smith Jr., Jane Q" {
		t.Errorf("PersonName = %q", PersonName("Jane", "Q", "Smith", "Jr."))
	}
	if NormalizeNCT(" nct04280705 ") != "NCT04280705" {
		t.Errorf("NormalizeNCT = %v", NormalizeNCT(" nct04280705 "))
	}
	if NormalizeNCT("see https://clinicaltrials.gov/NCT05665088 for details") != "NCT05665088" {
		t.Error("NormalizeNCT should extract embedded id")
	}
	r := Row{"total_amount_of_payment_usdollars": "1,234.50", "program_year": "2024"}
	if r.F("total_amount_of_payment_usdollars") != 1234.5 || r.I("program_year") != 2024 {
		t.Errorf("Row numeric parsing wrong: %v %v", r.F("total_amount_of_payment_usdollars"), r.I("program_year"))
	}
}

func TestProductsAndInvestigators(t *testing.T) {
	r := Row{
		"name_of_drug_or_biological_or_device_or_medical_supply_1":  "MAKO",
		"indicate_drug_or_biological_or_device_or_medical_supply_1": "Device",
		"product_category_or_therapeutic_area_3":                    "Orthopedics",
		"principal_investigator_1_npi":                              "1234567890",
		"principal_investigator_1_first_name":                       "Ann",
		"principal_investigator_1_last_name":                        "Lee",
		"principal_investigator_1_zip_code":                         "19104-3333",
		"principal_investigator_4_last_name":                        "Park",
	}
	ps := Products(r)
	if len(ps) != 2 || ps[0].Name != "MAKO" || ps[1].Slot != 3 {
		t.Fatalf("Products = %+v", ps)
	}
	inv := Investigators(r)
	if len(inv) != 2 || inv[0].Name != "Lee, Ann" || inv[0].Zip5 != "19104" || inv[1].Slot != 4 {
		t.Fatalf("Investigators = %+v", inv)
	}
}

func TestPropertiesCoverColumns(t *testing.T) {
	for _, typ := range PaymentTypes {
		props := map[string]bool{}
		for _, p := range Properties(typ) {
			props[p] = true
		}
		for _, c := range Columns(typ) {
			for _, s := range c.Sources {
				if !props[s] {
					t.Errorf("%s: column %s source %s missing from Properties", typ, c.Name, s)
				}
			}
		}
		if !props["record_id"] || !props["program_year"] {
			t.Errorf("%s: provenance keys missing", typ)
		}
	}
}

func TestSponsorMatching(t *testing.T) {
	yes := [][2]string{{"Medtronic", "Medtronic USA, Inc."}, {"Boston Scientific Corporation", "Boston Scientific Corp"}, {"Eli Lilly and Company", "Eli Lilly and Company"}, {"Stryker", "STRYKER CORPORATION"}}
	no := [][2]string{{"Medtronic", "Abbott Laboratories"}, {"Pfizer", "Pfizer-BioNTech Collaboration Partner LLC Other"}, {"University of Pennsylvania", "Penn Medical Devices"}}
	for _, p := range yes {
		if !SponsorMatches(p[0], p[1]) {
			t.Errorf("expected %q ~ %q", p[0], p[1])
		}
	}
	for _, p := range no[:1] {
		if SponsorMatches(p[0], p[1]) {
			t.Errorf("did not expect %q ~ %q", p[0], p[1])
		}
	}
	got := MatchCompanies("DePuy Synthes", []string{"DePuy Synthes Products, Inc", "Stryker Corporation"}, nil)
	if len(got) != 1 || got[0] != "DePuy Synthes Products, Inc" {
		t.Errorf("MatchCompanies = %v", got)
	}
	aliased := MatchCompanies("J&J MedTech", []string{"Johnson & Johnson Health Care Systems Inc."}, map[string]string{"j&j medtech": "Johnson & Johnson"})
	if len(aliased) != 1 {
		t.Errorf("alias should match, got %v", aliased)
	}
}

func TestGeo(t *testing.T) {
	// Philadelphia City Hall to Ambler (19002) is roughly 15 miles.
	d := Haversine(39.9524, -75.1636, 40.18775, -75.21585)
	if d < 14 || d > 18 {
		t.Errorf("Haversine = %.1f", d)
	}
	minLat, maxLat, minLon, maxLon := BoundingBox(40, -75, 25)
	if !(minLat < 40 && maxLat > 40 && minLon < -75 && maxLon > -75) || math.Abs(maxLat-minLat-50.0/69) > 0.01 {
		t.Errorf("BoundingBox wrong: %v %v %v %v", minLat, maxLat, minLon, maxLon)
	}
	if StateCodes["new jersey"] != "NJ" || StateName("pa") != "pennsylvania" {
		t.Error("state mapping wrong")
	}
}

func TestScopePredicate(t *testing.T) {
	keep := scopePredicate(TypeResearch, Scope{States: []string{"pa"}, NPIs: []string{"111"}})
	if !keep(Row{"recipient_state": "PA", "covered_recipient_npi": "111"}) {
		t.Error("recipient NPI match should keep")
	}
	if !keep(Row{"recipient_state": "PA", "principal_investigator_3_npi": "111"}) {
		t.Error("PI slot NPI match should keep for research")
	}
	if keep(Row{"recipient_state": "NJ", "covered_recipient_npi": "111"}) {
		t.Error("wrong state should drop")
	}
	spec := scopePredicate(TypeGeneral, Scope{Specialties: []string{"Pain Medicine"}})
	if !spec(Row{"covered_recipient_specialty_1": "Allopathic & Osteopathic Physicians|Pain Medicine|Interventional"}) {
		t.Error("specialty substring should keep")
	}
}

// fakeGetter serves canned DKAN pages for sync tests.
type fakeGetter struct {
	pages map[int][]map[string]any
	calls int
}

func (f *fakeGetter) Get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error) {
	return json.RawMessage(`[]`), nil
}

func (f *fakeGetter) GetWithHeadersNoCacheValues(ctx context.Context, path string, params url.Values, headers map[string]string) (json.RawMessage, error) {
	f.calls++
	if params.Get("schema") == "true" {
		fields := map[string]any{}
		for _, p := range Properties(TypeOwnership) {
			fields[p] = map[string]string{"type": "text"}
		}
		b, _ := json.Marshal(map[string]any{"schema": map[string]any{"x": map[string]any{"fields": fields}}})
		return b, nil
	}
	off := 0
	if o := params.Get("offset"); o != "" {
		off = atoi(o)
	}
	b, _ := json.Marshal(map[string]any{"results": f.pages[off]})
	return b, nil
}

func atoi(s string) int {
	n := 0
	for _, r := range s {
		n = n*10 + int(r-'0')
	}
	return n
}

func TestSyncUpsertAndDeletes(t *testing.T) {
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatal(err)
	}
	db.SetMaxOpenConns(1)
	if err := EnsureSchema(db); err != nil {
		t.Fatal(err)
	}
	reg := []Dataset{{Year: 2024, Type: TypeOwnership, DatasetID: "ds1", Modified: "2026-06-30"}}
	row := func(id, amt string) map[string]any {
		return map[string]any{"record_id": id, "program_year": "2024", "physician_npi": "111", "recipient_state": "NJ", "total_amount_invested_usdollars": amt, "applicable_manufacturer_or_applicable_gpo_making_payment_name": "Acme"}
	}
	g := &fakeGetter{pages: map[int][]map[string]any{0: {row("1", "10"), row("2", "20")}}}
	scope := Scope{Years: []int{2024}, Types: []string{TypeOwnership}, States: []string{"NJ"}}
	rep, err := Sync(context.Background(), g, db, reg, scope, SyncOptions{Concurrency: 2})
	if err != nil {
		t.Fatal(err)
	}
	if rep.RowsSeen != 2 || rep.Added != 2 {
		t.Fatalf("first sync report = %+v", rep)
	}
	// Unchanged modified date: skipped.
	rep, _ = Sync(context.Background(), g, db, reg, scope, SyncOptions{})
	if rep.Skipped != 1 {
		t.Fatalf("expected skip on unchanged dataset, got %+v", rep)
	}
	// CMS refresh: record 1 amended, record 2 deleted, record 3 added.
	reg[0].Modified = "2027-01-15"
	g.pages[0] = []map[string]any{row("1", "15"), row("3", "30")}
	rep, err = Sync(context.Background(), g, db, reg, scope, SyncOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if rep.Amended != 1 || rep.Deleted != 1 || rep.Added != 1 {
		t.Fatalf("refresh report = %+v", rep)
	}
	var n int
	_ = db.QueryRow(`SELECT COUNT(*) FROM sync_changes WHERE sync_run = ?`, rep.SyncRun).Scan(&n)
	if n != 3 {
		t.Errorf("expected 3 logged changes, got %d", n)
	}
	var amt float64
	_ = db.QueryRow(`SELECT amount_invested FROM payments_ownership WHERE record_id='1'`).Scan(&amt)
	if amt != 15 {
		t.Errorf("amended amount = %v", amt)
	}
	_ = db.QueryRow(`SELECT COUNT(*) FROM recipients`).Scan(&n)
	if n != 1 {
		t.Errorf("recipients rebuilt = %d", n)
	}
}

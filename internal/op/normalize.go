package op

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Row is one raw DKAN/CSV record with snake_case keys and string values.
type Row map[string]any

// S returns a trimmed string field ("" when absent).
func (r Row) S(key string) string {
	v, ok := r[key]
	if !ok || v == nil {
		return ""
	}
	switch t := v.(type) {
	case string:
		return strings.TrimSpace(t)
	default:
		return strings.TrimSpace(fmt.Sprint(t))
	}
}

// F parses a numeric field; CMS publishes every value as a string.
func (r Row) F(key string) any {
	s := r.S(key)
	if s == "" {
		return nil
	}
	f, err := strconv.ParseFloat(strings.ReplaceAll(s, ",", ""), 64)
	if err != nil {
		return nil
	}
	return f
}

// I parses an integer field.
func (r Row) I(key string) any {
	s := r.S(key)
	if s == "" {
		return nil
	}
	i, err := strconv.Atoi(s)
	if err != nil {
		return nil
	}
	return i
}

// NullS returns nil for empty strings so SQL NULL marks absent data.
func (r Row) NullS(key string) any {
	if s := r.S(key); s != "" {
		return s
	}
	return nil
}

// ISODate converts CMS MM/DD/YYYY dates to YYYY-MM-DD.
func ISODate(s string) any {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	for _, layout := range []string{"01/02/2006", "1/2/2006", "2006-01-02", "01/02/2006 15:04:05"} {
		if t, err := time.Parse(layout, s); err == nil {
			return t.Format("2006-01-02")
		}
	}
	return s
}

// Zip5 trims ZIP+4 to five digits.
func Zip5(s string) any {
	s = strings.TrimSpace(s)
	if len(s) >= 5 {
		return s[:5]
	}
	if s == "" {
		return nil
	}
	return s
}

// PersonName formats "Last, First Middle" for display and search.
func PersonName(first, middle, last, suffix string) string {
	given := strings.TrimSpace(strings.Join(nonEmpty(first, middle), " "))
	name := strings.TrimSpace(last)
	if suffix != "" {
		name += " " + suffix
	}
	if given != "" {
		if name != "" {
			return name + ", " + given
		}
		return given
	}
	return name
}

func nonEmpty(xs ...string) []string {
	var out []string
	for _, x := range xs {
		if strings.TrimSpace(x) != "" {
			out = append(out, strings.TrimSpace(x))
		}
	}
	return out
}

// Column maps a destination column to a value extractor.
type Column struct {
	Name    string
	Sources []string // raw keys read (used to build DKAN properties lists)
	Value   func(Row) any
}

func str(col, key string) Column {
	return Column{col, []string{key}, func(r Row) any { return r.NullS(key) }}
}
func num(col, key string) Column {
	return Column{col, []string{key}, func(r Row) any { return r.F(key) }}
}

func joinSlots(r Row, prefix string, n int) any {
	var parts []string
	for i := 1; i <= n; i++ {
		if s := r.S(fmt.Sprintf("%s%d", prefix, i)); s != "" {
			parts = append(parts, s)
		}
	}
	if len(parts) == 0 {
		return nil
	}
	return strings.Join(parts, "; ")
}

func slotKeys(prefix string, n int) []string {
	out := make([]string, n)
	for i := range out {
		out[i] = fmt.Sprintf("%s%d", prefix, i+1)
	}
	return out
}

// recipientColumns are shared by general and research.
func recipientColumns() []Column {
	return []Column{
		str("change_type", "change_type"),
		str("recipient_type", "covered_recipient_type"),
		str("npi", "covered_recipient_npi"),
		str("profile_id", "covered_recipient_profile_id"),
		str("first_name", "covered_recipient_first_name"),
		str("middle_name", "covered_recipient_middle_name"),
		str("last_name", "covered_recipient_last_name"),
		{"recipient_name", []string{"covered_recipient_first_name", "covered_recipient_middle_name", "covered_recipient_last_name", "covered_recipient_name_suffix", "teaching_hospital_name"}, func(r Row) any {
			if n := PersonName(r.S("covered_recipient_first_name"), r.S("covered_recipient_middle_name"), r.S("covered_recipient_last_name"), r.S("covered_recipient_name_suffix")); n != "" {
				return n
			}
			if h := r.S("teaching_hospital_name"); h != "" {
				return h
			}
			return r.NullS("noncovered_recipient_entity_name")
		}},
		str("street", "recipient_primary_business_street_address_line1"),
		str("city", "recipient_city"),
		str("state", "recipient_state"),
		{"zip5", []string{"recipient_zip_code"}, func(r Row) any { return Zip5(r.S("recipient_zip_code")) }},
		str("zip", "recipient_zip_code"),
		str("country", "recipient_country"),
		str("primary_type", "covered_recipient_primary_type_1"),
		str("specialty", "covered_recipient_specialty_1"),
		{"specialties", slotKeys("covered_recipient_specialty_", 6), func(r Row) any { return joinSlots(r, "covered_recipient_specialty_", 6) }},
		str("teaching_hospital_ccn", "teaching_hospital_ccn"),
		str("teaching_hospital_id", "teaching_hospital_id"),
		str("teaching_hospital_name", "teaching_hospital_name"),
		str("company", "applicable_manufacturer_or_applicable_gpo_making_payment_name"),
		str("company_id", "applicable_manufacturer_or_applicable_gpo_making_payment_id"),
		str("company_state", "applicable_manufacturer_or_applicable_gpo_making_payment_state"),
		str("company_country", "applicable_manufacturer_or_applicable_gpo_making_payment_country"),
		str("submitting_company", "submitting_applicable_manufacturer_or_applicable_gpo_name"),
		num("amount", "total_amount_of_payment_usdollars"),
		{"payment_date", []string{"date_of_payment"}, func(r Row) any { return ISODate(r.S("date_of_payment")) }},
		str("form", "form_of_payment_or_transfer_of_value"),
		str("related_product", "related_product_indicator"),
		str("dispute", "dispute_status_for_publication"),
		str("delay_publication", "delay_in_publication_indicator"),
		{"publication_date", []string{"payment_publication_date"}, func(r Row) any { return ISODate(r.S("payment_publication_date")) }},
	}
}

// Columns returns the typed column mapping for a payment type.
func Columns(typ string) []Column {
	base := []Column{str("record_id", "record_id"), Column{"program_year", []string{"program_year"}, func(r Row) any { return r.I("program_year") }}}
	switch typ {
	case TypeGeneral:
		cols := append(base, recipientColumns()...)
		return append(cols,
			Column{"n_payments", []string{"number_of_payments_included_in_total_amount"}, func(r Row) any { return r.I("number_of_payments_included_in_total_amount") }},
			str("nature", "nature_of_payment_or_transfer_of_value"),
			str("travel_city", "city_of_travel"),
			str("travel_state", "state_of_travel"),
			str("travel_country", "country_of_travel"),
			str("physician_ownership", "physician_ownership_indicator"),
			str("third_party", "third_party_payment_recipient_indicator"),
			str("charity", "charity_indicator"),
			str("context", "contextual_information"),
		)
	case TypeResearch:
		cols := append(base, recipientColumns()...)
		return append(cols,
			str("noncovered_entity", "noncovered_recipient_entity_name"),
			str("name_of_study", "name_of_study"),
			Column{"nct_id", []string{"clinicaltrials_gov_identifier"}, func(r Row) any { return NormalizeNCT(r.S("clinicaltrials_gov_identifier")) }},
			str("context_of_research", "context_of_research"),
			str("preclinical", "preclinical_research_indicator"),
			str("research_link", "research_information_link"),
			Column{"expenditure", slotKeys("expenditure_category", 6), func(r Row) any { return joinSlots(r, "expenditure_category", 6) }},
		)
	case TypeOwnership:
		return append(base,
			str("change_type", "change_type"),
			str("npi", "physician_npi"),
			str("profile_id", "physician_profile_id"),
			str("first_name", "physician_first_name"),
			str("middle_name", "physician_middle_name"),
			str("last_name", "physician_last_name"),
			Column{"recipient_name", []string{"physician_first_name", "physician_middle_name", "physician_last_name", "physician_name_suffix"}, func(r Row) any {
				return PersonName(r.S("physician_first_name"), r.S("physician_middle_name"), r.S("physician_last_name"), r.S("physician_name_suffix"))
			}},
			str("street", "recipient_primary_business_street_address_line1"),
			str("city", "recipient_city"),
			str("state", "recipient_state"),
			Column{"zip5", []string{"recipient_zip_code"}, func(r Row) any { return Zip5(r.S("recipient_zip_code")) }},
			str("zip", "recipient_zip_code"),
			str("country", "recipient_country"),
			str("primary_type", "physician_primary_type"),
			str("specialty", "physician_specialty"),
			num("amount_invested", "total_amount_invested_usdollars"),
			num("value_of_interest", "value_of_interest"),
			str("terms", "terms_of_interest"),
			str("held_by", "interest_held_by_physician_or_an_immediate_family_member"),
			str("company", "applicable_manufacturer_or_applicable_gpo_making_payment_name"),
			str("company_id", "applicable_manufacturer_or_applicable_gpo_making_payment_id"),
			str("company_state", "applicable_manufacturer_or_applicable_gpo_making_payment_state"),
			str("company_country", "applicable_manufacturer_or_applicable_gpo_making_payment_country"),
			str("submitting_company", "submitting_applicable_manufacturer_or_applicable_gpo_name"),
			str("dispute", "dispute_status_for_publication"),
			Column{"publication_date", []string{"payment_publication_date"}, func(r Row) any { return ISODate(r.S("payment_publication_date")) }},
		)
	}
	return nil
}

// Product is one drug/device slot on a payment.
type Product struct {
	Slot                                    int
	Covered, Kind, Category, Name, NDC, PDI string
}

// Products extracts the 1-5 product slots on a general or research row.
func Products(r Row) []Product {
	var out []Product
	for i := 1; i <= 5; i++ {
		p := Product{
			Slot:     i,
			Covered:  r.S(fmt.Sprintf("covered_or_noncovered_indicator_%d", i)),
			Kind:     r.S(fmt.Sprintf("indicate_drug_or_biological_or_device_or_medical_supply_%d", i)),
			Category: r.S(fmt.Sprintf("product_category_or_therapeutic_area_%d", i)),
			Name:     r.S(fmt.Sprintf("name_of_drug_or_biological_or_device_or_medical_supply_%d", i)),
			NDC:      r.S(fmt.Sprintf("associated_drug_or_biological_ndc_%d", i)),
			PDI:      r.S(fmt.Sprintf("associated_device_or_medical_supply_pdi_%d", i)),
		}
		if p.Name != "" || p.Category != "" || p.NDC != "" || p.PDI != "" {
			out = append(out, p)
		}
	}
	return out
}

func productKeys() []string {
	var out []string
	for i := 1; i <= 5; i++ {
		for _, p := range []string{"covered_or_noncovered_indicator_", "indicate_drug_or_biological_or_device_or_medical_supply_", "product_category_or_therapeutic_area_", "name_of_drug_or_biological_or_device_or_medical_supply_", "associated_drug_or_biological_ndc_", "associated_device_or_medical_supply_pdi_"} {
			out = append(out, fmt.Sprintf("%s%d", p, i))
		}
	}
	return out
}

// Investigator is one PI slot on a research payment.
type Investigator struct {
	Slot                                                                                        int
	NPI, ProfileID, First, Last, Name, Type, City, State, Zip5, Country, PrimaryType, Specialty string
}

// Investigators extracts principal investigators 1-5.
func Investigators(r Row) []Investigator {
	var out []Investigator
	for i := 1; i <= 5; i++ {
		p := fmt.Sprintf("principal_investigator_%d_", i)
		inv := Investigator{
			Slot:        i,
			NPI:         r.S(p + "npi"),
			ProfileID:   r.S(p + "profile_id"),
			First:       r.S(p + "first_name"),
			Last:        r.S(p + "last_name"),
			Type:        r.S(p + "covered_recipient_type"),
			City:        r.S(p + "city"),
			State:       r.S(p + "state"),
			Country:     r.S(p + "country"),
			PrimaryType: r.S(p + "primary_type_1"),
			Specialty:   r.S(p + "specialty_1"),
		}
		if z, ok := Zip5(r.S(p + "zip_code")).(string); ok {
			inv.Zip5 = z
		}
		inv.Name = PersonName(inv.First, r.S(p+"middle_name"), inv.Last, r.S(p+"name_suffix"))
		if inv.NPI != "" || inv.ProfileID != "" || inv.Last != "" {
			out = append(out, inv)
		}
	}
	return out
}

func investigatorKeys() []string {
	var out []string
	for i := 1; i <= 5; i++ {
		p := fmt.Sprintf("principal_investigator_%d_", i)
		for _, k := range []string{"npi", "profile_id", "first_name", "middle_name", "last_name", "name_suffix", "covered_recipient_type", "city", "state", "zip_code", "country", "primary_type_1", "specialty_1"} {
			out = append(out, p+k)
		}
	}
	return out
}

// Properties lists every raw key the sync must request for a type, so pages
// carry only mapped columns (~40 KB instead of ~2 MB per 500 rows).
func Properties(typ string) []string {
	seen := map[string]bool{}
	var out []string
	add := func(keys ...string) {
		for _, k := range keys {
			if !seen[k] {
				seen[k] = true
				out = append(out, k)
			}
		}
	}
	for _, c := range Columns(typ) {
		add(c.Sources...)
	}
	if typ != TypeOwnership {
		add("covered_recipient_name_suffix")
		add(productKeys()...)
	}
	if typ == TypeResearch {
		add("noncovered_recipient_entity_name")
		add(investigatorKeys()...)
	}
	sort.Strings(out)
	return out
}

// NormalizeNCT upper-cases and validates a ClinicalTrials.gov identifier.
func NormalizeNCT(s string) any {
	s = strings.ToUpper(strings.TrimSpace(s))
	if s == "" {
		return nil
	}
	// CMS occasionally publishes lists or URLs; keep the first NCT token.
	if i := strings.Index(s, "NCT"); i >= 0 && len(s) >= i+11 {
		cand := s[i : i+11]
		if _, err := strconv.Atoi(cand[3:]); err == nil {
			return cand
		}
	}
	return s
}

// TableFor maps a payment type to its table.
func TableFor(typ string) string {
	return "payments_" + typ
}

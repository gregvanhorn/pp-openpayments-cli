// Package op holds the CMS Open Payments domain layer: dataset resolution,
// DKAN query building, the typed SQLite schema, and scoped sync.
package op

import (
	"context"
	"encoding/json"
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

// MaxPageSize is the DKAN hard cap. limit=501 returns HTTP 400
// "JSON Schema validation failed", so every request is clamped here.
const MaxPageSize = 500

// Getter is the slice of the generated HTTP client the domain layer needs.
// It keeps this package free of the generated client's import graph and lets
// tests substitute a fake.
type Getter interface {
	Get(ctx context.Context, path string, params map[string]string) (json.RawMessage, error)
	GetWithHeadersNoCacheValues(ctx context.Context, path string, params url.Values, headers map[string]string) (json.RawMessage, error)
}

// Condition is one DKAN query condition. Operator is one of
// =, <>, <, <=, >, >=, like, between, in, not in.
type Condition struct {
	Property string
	Operator string
	Value    any // string, or []string for between/in
}

// Sort orders results by a property or alias.
type Sort struct {
	Property string
	Desc     bool
}

// Aggregate is a server-side expression, e.g. sum(total_amount_of_payment_usdollars) AS total.
type Aggregate struct {
	Operator string // sum, count, avg, min, max
	Property string
	Alias    string
}

// Query describes a GET /datastore/query/{datasetId}/0 request.
type Query struct {
	Conditions []Condition
	Properties []string
	Aggregates []Aggregate
	Groupings  []string
	Sorts      []Sort
	Limit      int
	Offset     int
	Count      bool
	NoResults  bool // results=false: count/schema only
}

// Values encodes the query in DKAN's bracketed deepObject form.
func (q Query) Values() url.Values {
	v := url.Values{}
	limit := q.Limit
	if limit <= 0 || limit > MaxPageSize {
		limit = MaxPageSize
	}
	v.Set("limit", strconv.Itoa(limit))
	if q.Offset > 0 {
		v.Set("offset", strconv.Itoa(q.Offset))
	}
	v.Set("count", strconv.FormatBool(q.Count))
	if q.NoResults {
		v.Set("results", "false")
	}
	for i, c := range q.Conditions {
		p := fmt.Sprintf("conditions[%d]", i)
		v.Set(p+"[property]", c.Property)
		op := c.Operator
		if op == "" {
			op = "="
		}
		v.Set(p+"[operator]", op)
		switch val := c.Value.(type) {
		case []string:
			// Append syntax keeps list order under url.Values' key sorting
			// (indexed [10] would sort before [2] and decode as an object).
			for _, s := range val {
				v.Add(p+"[value][]", s)
			}
		default:
			v.Set(p+"[value]", fmt.Sprint(val))
		}
	}
	// url.Values encodes keys sorted, so properties[10] would precede
	// properties[2] and PHP would decode an object, which DKAN rejects.
	// Plain column lists use append syntax; indexed keys are only used with
	// aggregates and are kept below 10 (see Validate).
	n := 0
	if len(q.Aggregates) == 0 {
		for _, prop := range q.Properties {
			v.Add("properties[]", prop)
		}
	} else {
		for _, prop := range q.Properties {
			v.Set(fmt.Sprintf("properties[%d]", n), prop)
			n++
		}
	}
	for _, a := range q.Aggregates {
		p := fmt.Sprintf("properties[%d]", n)
		v.Set(p+"[alias]", a.Alias)
		v.Set(p+"[expression][operator]", a.Operator)
		v.Set(p+"[expression][operands][0]", a.Property)
		n++
	}
	for i, g := range q.Groupings {
		v.Set(fmt.Sprintf("groupings[%d]", i), g)
	}
	for i, s := range q.Sorts {
		v.Set(fmt.Sprintf("sorts[%d][property]", i), s.Property)
		order := "asc"
		if s.Desc {
			order = "desc"
		}
		v.Set(fmt.Sprintf("sorts[%d][order]", i), order)
	}
	return v
}

// Validate rejects shapes DKAN cannot decode after key sorting.
func (q Query) Validate() error {
	if len(q.Aggregates) > 0 && len(q.Properties)+len(q.Aggregates) > 10 {
		return fmt.Errorf("aggregate queries support at most 10 grouped columns plus aggregates")
	}
	if len(q.Conditions) > 10 || len(q.Groupings) > 10 || len(q.Sorts) > 10 {
		return fmt.Errorf("at most 10 conditions, groupings and sorts per query")
	}
	return nil
}

// QueryResponse is the DKAN datastore query envelope.
type QueryResponse struct {
	Results []map[string]any `json:"results"`
	Count   *int             `json:"count"`
	Message string           `json:"message"`
}

// RunQuery executes one page of a DKAN query against a dataset.
func RunQuery(ctx context.Context, g Getter, datasetID string, q Query) (*QueryResponse, error) {
	if err := q.Validate(); err != nil {
		return nil, err
	}
	path := "/api/1/datastore/query/" + url.PathEscape(datasetID) + "/0"
	raw, err := g.GetWithHeadersNoCacheValues(ctx, path, q.Values(), nil)
	if err != nil {
		return nil, err
	}
	var resp QueryResponse
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decoding DKAN query response: %w", err)
	}
	if resp.Message != "" && len(resp.Results) == 0 && resp.Count == nil {
		return nil, fmt.Errorf("DKAN query error: %s", resp.Message)
	}
	return &resp, nil
}

// RunSQL executes DKAN bracket SQL and normalizes Title_Case keys to snake_case.
func RunSQL(ctx context.Context, g Getter, query string) ([]map[string]any, error) {
	v := url.Values{}
	v.Set("query", query)
	v.Set("show_db_columns", "true")
	raw, err := g.GetWithHeadersNoCacheValues(ctx, "/api/1/datastore/sql", v, nil)
	if err != nil {
		return nil, err
	}
	var rows []map[string]any
	if err := json.Unmarshal(raw, &rows); err != nil {
		var envelope struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(raw, &envelope) == nil && envelope.Message != "" {
			return nil, fmt.Errorf("DKAN SQL error: %s", envelope.Message)
		}
		return nil, fmt.Errorf("decoding DKAN SQL response: %w", err)
	}
	for i, r := range rows {
		rows[i] = SnakeKeys(r)
	}
	return rows, nil
}

// SnakeKeys lower-cases keys so SQL (Title_Case) and query (snake_case)
// results share one shape.
func SnakeKeys(in map[string]any) map[string]any {
	out := make(map[string]any, len(in))
	for k, v := range in {
		out[strings.ToLower(strings.ReplaceAll(k, " ", "_"))] = v
	}
	return out
}

// ParseCondition parses a CLI filter like "recipient_state=PA",
// "total_amount_of_payment_usdollars>=1000" or "name~Stryker" (like).
func ParseCondition(s string) (Condition, error) {
	for _, op := range []string{">=", "<=", "!=", "<>", "=", ">", "<", "~"} {
		if i := strings.Index(s, op); i > 0 {
			prop := strings.TrimSpace(s[:i])
			val := strings.TrimSpace(s[i+len(op):])
			switch op {
			case "!=":
				op = "<>"
			case "~":
				op = "like"
				if !strings.Contains(val, "%") {
					val = "%" + val + "%"
				}
			}
			if op == "=" && strings.Contains(val, ",") {
				return Condition{Property: prop, Operator: "in", Value: strings.Split(val, ",")}, nil
			}
			return Condition{Property: prop, Operator: op, Value: val}, nil
		}
	}
	return Condition{}, fmt.Errorf("cannot parse filter %q: expected property<op>value with op one of = != > >= < <= ~", s)
}

// SchemaFields returns the column names a dataset actually exposes.
func SchemaFields(ctx context.Context, g Getter, datasetID string) (map[string]bool, error) {
	v := url.Values{}
	v.Set("results", "false")
	v.Set("count", "false")
	v.Set("schema", "true")
	v.Set("limit", "1")
	raw, err := g.GetWithHeadersNoCacheValues(ctx, "/api/1/datastore/query/"+url.PathEscape(datasetID)+"/0", v, nil)
	if err != nil {
		return nil, err
	}
	var resp struct {
		Schema map[string]struct {
			Fields map[string]json.RawMessage `json:"fields"`
		} `json:"schema"`
	}
	if err := json.Unmarshal(raw, &resp); err != nil {
		return nil, fmt.Errorf("decoding dataset schema: %w", err)
	}
	out := map[string]bool{}
	for _, s := range resp.Schema {
		for k := range s.Fields {
			out[k] = true
		}
	}
	return out, nil
}

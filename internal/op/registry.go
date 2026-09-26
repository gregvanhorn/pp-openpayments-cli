package op

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Payment dataset types.
const (
	TypeGeneral   = "general"
	TypeResearch  = "research"
	TypeOwnership = "ownership"
)

// PaymentTypes lists the three detail-payment families in sync order.
var PaymentTypes = []string{TypeGeneral, TypeResearch, TypeOwnership}

// Dataset is one entry in the CMS metastore, resolved to the identifiers the
// two query surfaces need: DatasetID for /datastore/query, DistributionID for
// /datastore/sql.
type Dataset struct {
	Year           int    `json:"year,omitempty"`
	Type           string `json:"type"`
	Title          string `json:"title"`
	DatasetID      string `json:"dataset_id"`
	DistributionID string `json:"distribution_id"`
	Modified       string `json:"modified"`
	DownloadURL    string `json:"download_url"`
	RowCount       int64  `json:"row_count,omitempty"`
	ResolvedAt     string `json:"resolved_at,omitempty"`
}

var paymentTitleRE = regexp.MustCompile(`^(\d{4}) (General|Research|Ownership) Payment Data$`)
var yearPrefixRE = regexp.MustCompile(`^(\d{4}) `)
var slugRE = regexp.MustCompile(`[^a-z0-9]+`)

// ClassifyTitle maps a metastore title to (year, type). Detail payment
// datasets get general/research/ownership; everything else gets a slug of its
// title so summaries and profile tables stay addressable.
func ClassifyTitle(title string) (int, string) {
	if m := paymentTitleRE.FindStringSubmatch(title); m != nil {
		y, _ := strconv.Atoi(m[1])
		return y, strings.ToLower(m[2])
	}
	year := 0
	rest := title
	if m := yearPrefixRE.FindStringSubmatch(title); m != nil {
		year, _ = strconv.Atoi(m[1])
		rest = title[len(m[0]):]
	}
	slug := strings.Trim(slugRE.ReplaceAllString(strings.ToLower(rest), "-"), "-")
	return year, slug
}

type metastoreItem struct {
	Identifier   string `json:"identifier"`
	Title        string `json:"title"`
	Modified     string `json:"modified"`
	Distribution []struct {
		Identifier string `json:"identifier"`
		Data       struct {
			DownloadURL string `json:"downloadURL"`
		} `json:"data"`
	} `json:"distribution"`
}

// FetchCatalog lists every dataset from the live metastore.
func FetchCatalog(ctx context.Context, g Getter) ([]Dataset, error) {
	raw, err := g.Get(ctx, "/api/1/metastore/schemas/dataset/items", map[string]string{"show-reference-ids": "true"})
	if err != nil {
		return nil, err
	}
	var items []metastoreItem
	if err := json.Unmarshal(raw, &items); err != nil {
		return nil, fmt.Errorf("decoding metastore catalog: %w", err)
	}
	now := time.Now().UTC().Format(time.RFC3339)
	out := make([]Dataset, 0, len(items))
	for _, it := range items {
		year, typ := ClassifyTitle(it.Title)
		d := Dataset{Year: year, Type: typ, Title: it.Title, DatasetID: it.Identifier, Modified: it.Modified, ResolvedAt: now}
		if len(it.Distribution) > 0 {
			d.DistributionID = it.Distribution[0].Identifier
			d.DownloadURL = it.Distribution[0].Data.DownloadURL
		}
		out = append(out, d)
	}
	SortDatasets(out)
	return out, nil
}

// SortDatasets orders payment families first by year, then everything else by title.
func SortDatasets(ds []Dataset) {
	rank := func(t string) int {
		switch t {
		case TypeGeneral:
			return 0
		case TypeResearch:
			return 1
		case TypeOwnership:
			return 2
		}
		return 3
	}
	sort.SliceStable(ds, func(i, j int) bool {
		ri, rj := rank(ds[i].Type), rank(ds[j].Type)
		if (ri < 3) != (rj < 3) {
			return ri < 3
		}
		if ds[i].Year != ds[j].Year {
			return ds[i].Year > ds[j].Year
		}
		if ri != rj {
			return ri < rj
		}
		return ds[i].Title < ds[j].Title
	})
}

// SaveRegistry replaces the cached registry.
func SaveRegistry(db *sql.DB, ds []Dataset) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT INTO dataset_registry (dataset_id, year, type, title, distribution_id, modified, download_url, resolved_at)
		VALUES (?,?,?,?,?,?,?,?)
		ON CONFLICT(dataset_id) DO UPDATE SET year=excluded.year, type=excluded.type, title=excluded.title,
		distribution_id=excluded.distribution_id, modified=excluded.modified, download_url=excluded.download_url, resolved_at=excluded.resolved_at`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	for _, d := range ds {
		if _, err := stmt.Exec(d.DatasetID, d.Year, d.Type, d.Title, d.DistributionID, d.Modified, d.DownloadURL, d.ResolvedAt); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// LoadRegistry reads the cached registry; empty when never resolved.
func LoadRegistry(db *sql.DB) ([]Dataset, error) {
	rows, err := db.Query(`SELECT dataset_id, year, type, title, distribution_id, modified, download_url, COALESCE(row_count,0), resolved_at FROM dataset_registry`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Dataset
	for rows.Next() {
		var d Dataset
		if err := rows.Scan(&d.DatasetID, &d.Year, &d.Type, &d.Title, &d.DistributionID, &d.Modified, &d.DownloadURL, &d.RowCount, &d.ResolvedAt); err != nil {
			return nil, err
		}
		out = append(out, d)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	SortDatasets(out)
	return out, nil
}

// Find returns the dataset for a year and type from a resolved list.
func Find(ds []Dataset, year int, typ string) (Dataset, bool) {
	for _, d := range ds {
		if d.Type == typ && (d.Year == year || year == 0) {
			return d, true
		}
	}
	return Dataset{}, false
}

// ParseYears parses "2024", "2019-2025" or "2023,2025".
func ParseYears(s string) ([]int, error) {
	var out []int
	for _, part := range strings.Split(s, ",") {
		part = strings.TrimSpace(part)
		if part == "" {
			continue
		}
		if a, b, ok := strings.Cut(part, "-"); ok {
			lo, err1 := strconv.Atoi(strings.TrimSpace(a))
			hi, err2 := strconv.Atoi(strings.TrimSpace(b))
			if err1 != nil || err2 != nil || lo > hi {
				return nil, fmt.Errorf("invalid year range %q (want e.g. 2019-2025)", part)
			}
			for y := lo; y <= hi; y++ {
				out = append(out, y)
			}
			continue
		}
		y, err := strconv.Atoi(part)
		if err != nil {
			return nil, fmt.Errorf("invalid year %q", part)
		}
		out = append(out, y)
	}
	return out, nil
}

// ParseTypes validates a comma list of payment types.
func ParseTypes(s string) ([]string, error) {
	if strings.TrimSpace(s) == "" {
		return PaymentTypes, nil
	}
	var out []string
	for _, t := range strings.Split(s, ",") {
		t = strings.ToLower(strings.TrimSpace(t))
		switch t {
		case TypeGeneral, TypeResearch, TypeOwnership:
			out = append(out, t)
		case "":
		default:
			return nil, fmt.Errorf("unknown payment type %q (want general, research, ownership)", t)
		}
	}
	return out, nil
}

// SplitList splits a comma list, trimming blanks.
func SplitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

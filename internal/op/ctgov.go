package op

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strings"
	"time"

	"openpayments-pp-cli/internal/cliutil"
)

// CTGovBase is the ClinicalTrials.gov API v2 root (free, no auth).
const CTGovBase = "https://clinicaltrials.gov/api/v2"

// CTGov is a small ClinicalTrials.gov v2 client with adaptive pacing.
type CTGov struct {
	HTTP    *http.Client
	Base    string
	limiter *cliutil.AdaptiveLimiter
}

// NewCTGov returns a client paced at ratePerSec (ClinicalTrials.gov asks
// for polite use; 3 req/s is well under its limits).
func NewCTGov(timeout time.Duration, ratePerSec float64) *CTGov {
	if ratePerSec <= 0 {
		ratePerSec = 3
	}
	return &CTGov{HTTP: &http.Client{Timeout: timeout, Transport: http.DefaultTransport}, Base: CTGovBase, limiter: cliutil.NewAdaptiveLimiter(ratePerSec)}
}

// Trial is the subset of a study the join needs.
type Trial struct {
	NCTID        string          `json:"nct_id"`
	Title        string          `json:"title"`
	Status       string          `json:"status"`
	Phase        string          `json:"phase"`
	Sponsor      string          `json:"sponsor"`
	SponsorClass string          `json:"sponsor_class"`
	Conditions   string          `json:"conditions"`
	StartDate    string          `json:"start_date"`
	Locations    []TrialLocation `json:"locations,omitempty"`
}

// TrialLocation is one site; State is a USPS code when mappable.
type TrialLocation struct {
	Facility string `json:"facility"`
	City     string `json:"city"`
	State    string `json:"state"`
	Zip5     string `json:"zip5"`
	Country  string `json:"country"`
	Status   string `json:"status"`
}

type ctStudy struct {
	ProtocolSection struct {
		IdentificationModule struct {
			NCTID      string `json:"nctId"`
			BriefTitle string `json:"briefTitle"`
		} `json:"identificationModule"`
		StatusModule struct {
			OverallStatus   string `json:"overallStatus"`
			StartDateStruct struct {
				Date string `json:"date"`
			} `json:"startDateStruct"`
		} `json:"statusModule"`
		SponsorCollaboratorsModule struct {
			LeadSponsor struct {
				Name  string `json:"name"`
				Class string `json:"class"`
			} `json:"leadSponsor"`
		} `json:"sponsorCollaboratorsModule"`
		ConditionsModule struct {
			Conditions []string `json:"conditions"`
		} `json:"conditionsModule"`
		DesignModule struct {
			Phases []string `json:"phases"`
		} `json:"designModule"`
		ContactsLocationsModule struct {
			Locations []struct {
				Facility string `json:"facility"`
				Status   string `json:"status"`
				City     string `json:"city"`
				State    string `json:"state"`
				Zip      string `json:"zip"`
				Country  string `json:"country"`
			} `json:"locations"`
		} `json:"contactsLocationsModule"`
	} `json:"protocolSection"`
}

const ctFields = "NCTId,BriefTitle,OverallStatus,Phase,LeadSponsorName,LeadSponsorClass,Condition,LocationFacility,LocationCity,LocationState,LocationZip,LocationCountry,LocationStatus,StartDate"

func (s ctStudy) trial() Trial {
	p := s.ProtocolSection
	t := Trial{
		NCTID:        p.IdentificationModule.NCTID,
		Title:        p.IdentificationModule.BriefTitle,
		Status:       p.StatusModule.OverallStatus,
		Phase:        strings.Join(p.DesignModule.Phases, "/"),
		Sponsor:      p.SponsorCollaboratorsModule.LeadSponsor.Name,
		SponsorClass: p.SponsorCollaboratorsModule.LeadSponsor.Class,
		Conditions:   strings.Join(p.ConditionsModule.Conditions, "; "),
		StartDate:    p.StatusModule.StartDateStruct.Date,
	}
	for _, l := range p.ContactsLocationsModule.Locations {
		st := l.State
		if code, ok := StateCodes[strings.ToLower(strings.TrimSpace(l.State))]; ok {
			st = code
		}
		z := l.Zip
		if len(z) > 5 {
			z = z[:5]
		}
		t.Locations = append(t.Locations, TrialLocation{Facility: l.Facility, City: l.City, State: st, Zip5: z, Country: l.Country, Status: l.Status})
	}
	return t
}

func (c *CTGov) get(ctx context.Context, path string, q url.Values) ([]byte, error) {
	u := c.Base + path
	if len(q) > 0 {
		u += "?" + q.Encode()
	}
	for attempt := 0; ; attempt++ {
		if err := c.limiter.Wait(ctx); err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("User-Agent", UserAgent)
		req.Header.Set("Accept", "application/json")
		resp, err := c.HTTP.Do(req)
		if err != nil {
			return nil, err
		}
		body, rerr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if resp.StatusCode == http.StatusTooManyRequests || resp.StatusCode >= 500 {
			c.limiter.OnRateLimit()
			if attempt < 3 {
				wait := cliutil.RetryAfter(resp)
				if wait <= 0 {
					wait = cliutil.Backoff(attempt)
				}
				select {
				case <-time.After(wait):
					continue
				case <-ctx.Done():
					return nil, ctx.Err()
				}
			}
			if resp.StatusCode == http.StatusTooManyRequests {
				return nil, &cliutil.RateLimitError{URL: u, RetryAfter: cliutil.RetryAfter(resp), Body: string(body)}
			}
			return nil, fmt.Errorf("ClinicalTrials.gov %s: HTTP %d", path, resp.StatusCode)
		}
		if rerr != nil {
			return nil, rerr
		}
		if resp.StatusCode == http.StatusNotFound {
			return nil, fmt.Errorf("ClinicalTrials.gov: %s not found", path)
		}
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("ClinicalTrials.gov %s: HTTP %d: %s", path, resp.StatusCode, truncate(string(body), 200))
		}
		c.limiter.OnSuccess()
		return body, nil
	}
}

func truncate(s string, n int) string {
	if len(s) > n {
		return s[:n] + "..."
	}
	return s
}

// Study fetches one study by NCT ID.
func (c *CTGov) Study(ctx context.Context, nct string) (Trial, error) {
	body, err := c.get(ctx, "/studies/"+url.PathEscape(nct), url.Values{"fields": {ctFields}})
	if err != nil {
		return Trial{}, err
	}
	var s ctStudy
	if err := json.Unmarshal(body, &s); err != nil {
		return Trial{}, fmt.Errorf("decoding study %s: %w", nct, err)
	}
	return s.trial(), nil
}

// SearchParams are ClinicalTrials.gov v2 query parameters.
type SearchParams struct {
	Condition string
	Location  string // free text, e.g. "Pennsylvania"
	Sponsor   string
	Statuses  []string
	MaxPages  int
}

// Search pages through matching studies (100 per page).
func (c *CTGov) Search(ctx context.Context, p SearchParams) ([]Trial, error) {
	q := url.Values{"fields": {ctFields}, "pageSize": {"100"}}
	if p.Condition != "" {
		q.Set("query.cond", p.Condition)
	}
	if p.Location != "" {
		q.Set("query.locn", p.Location)
	}
	if p.Sponsor != "" {
		q.Set("query.spons", p.Sponsor)
	}
	if len(p.Statuses) > 0 {
		q.Set("filter.overallStatus", strings.Join(p.Statuses, ","))
	}
	maxPages := p.MaxPages
	if maxPages <= 0 {
		maxPages = 10
	}
	var out []Trial
	for page := 0; page < maxPages; page++ {
		body, err := c.get(ctx, "/studies", q)
		if err != nil {
			return nil, err
		}
		var resp struct {
			Studies       []ctStudy `json:"studies"`
			NextPageToken string    `json:"nextPageToken"`
		}
		if err := json.Unmarshal(body, &resp); err != nil {
			return nil, fmt.Errorf("decoding study search: %w", err)
		}
		for _, s := range resp.Studies {
			out = append(out, s.trial())
		}
		if resp.NextPageToken == "" {
			break
		}
		q.Set("pageToken", resp.NextPageToken)
	}
	return out, nil
}

// CacheTrials upserts trials and their locations.
func CacheTrials(db *sql.DB, trials []Trial) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	now := time.Now().UTC().Format(time.RFC3339)
	for _, t := range trials {
		raw, _ := json.Marshal(t)
		if _, err := tx.Exec(`INSERT OR REPLACE INTO trials (nct_id, title, status, phase, sponsor, sponsor_class, conditions, start_date, fetched_at, raw) VALUES (?,?,?,?,?,?,?,?,?,?)`,
			t.NCTID, t.Title, t.Status, t.Phase, t.Sponsor, t.SponsorClass, t.Conditions, t.StartDate, now, string(raw)); err != nil {
			return err
		}
		if _, err := tx.Exec(`DELETE FROM trial_locations WHERE nct_id = ?`, t.NCTID); err != nil {
			return err
		}
		for _, l := range t.Locations {
			if _, err := tx.Exec(`INSERT INTO trial_locations (nct_id, facility, city, state, zip5, country, status) VALUES (?,?,?,?,?,?,?)`,
				t.NCTID, l.Facility, l.City, l.State, l.Zip5, l.Country, l.Status); err != nil {
				return err
			}
		}
	}
	return tx.Commit()
}

// CachedTrial reads a cached study younger than maxAge.
func CachedTrial(db *sql.DB, nct string, maxAge time.Duration) (Trial, bool) {
	var raw, fetched string
	if err := db.QueryRow(`SELECT raw, fetched_at FROM trials WHERE nct_id = ?`, nct).Scan(&raw, &fetched); err != nil {
		return Trial{}, false
	}
	t, err := time.Parse(time.RFC3339, fetched)
	if err != nil || time.Since(t) > maxAge {
		return Trial{}, false
	}
	var tr Trial
	if json.Unmarshal([]byte(raw), &tr) != nil {
		return Trial{}, false
	}
	return tr, true
}

var nonAlnum = regexp.MustCompile(`[^a-z0-9 ]+`)

var corpWords = map[string]bool{
	"inc": true, "incorporated": true, "llc": true, "l": true, "c": true, "corp": true, "corporation": true, "co": true, "company": true,
	"ltd": true, "limited": true, "plc": true, "sa": true, "se": true, "ag": true, "gmbh": true, "bv": true, "nv": true, "lp": true,
	"usa": true, "us": true, "the": true, "holdings": true, "group": true, "international": true, "of": true, "and": true, "america": true,
	"north": true, "global": true,
}

// SponsorKey normalizes a company or sponsor name to its distinctive tokens.
func SponsorKey(name string) []string {
	s := nonAlnum.ReplaceAllString(strings.ToLower(strings.ReplaceAll(name, "&", " and ")), " ")
	var toks []string
	for _, t := range strings.Fields(s) {
		if !corpWords[t] {
			toks = append(toks, t)
		}
	}
	return toks
}

// SponsorMatches reports whether a ClinicalTrials.gov lead sponsor and an
// Open Payments manufacturer name plausibly name the same company: the
// leading distinctive token must agree and the shorter token list must be a
// prefix of the longer one ("Medtronic" ~ "Medtronic USA, Inc."; "Boston
// Scientific Corporation" ~ "Boston Scientific"). Conservative on purpose:
// every command prints the matched pairs so users can verify or alias.
func SponsorMatches(sponsor, company string) bool {
	a, b := SponsorKey(sponsor), SponsorKey(company)
	if len(a) == 0 || len(b) == 0 {
		return false
	}
	if len(a) > len(b) {
		a, b = b, a
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// MatchCompanies returns the local Open Payments company names matching a sponsor.
func MatchCompanies(sponsor string, companies []string, aliases map[string]string) []string {
	if alias, ok := aliases[strings.ToLower(sponsor)]; ok {
		sponsor = alias
	}
	var out []string
	for _, c := range companies {
		if SponsorMatches(sponsor, c) {
			out = append(out, c)
		}
	}
	sort.Strings(out)
	return out
}

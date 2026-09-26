package op

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"database/sql"
	_ "embed"
	"math"
	"strconv"
	"strings"
)

// zctaCSV is the US Census Bureau 2024 ZCTA Gazetteer (public domain),
// reduced to zip5,lat,lon internal points.
//
//go:embed data/zcta.csv.gz
var zctaCSV []byte

// EnsureZipCentroids loads the bundled ZIP centroids once.
func EnsureZipCentroids(db *sql.DB) error {
	var n int
	if err := db.QueryRow(`SELECT COUNT(*) FROM zip_centroids`).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return nil
	}
	zr, err := gzip.NewReader(bytes.NewReader(zctaCSV))
	if err != nil {
		return err
	}
	defer zr.Close()
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	stmt, err := tx.Prepare(`INSERT OR REPLACE INTO zip_centroids (zip5, lat, lon) VALUES (?,?,?)`)
	if err != nil {
		return err
	}
	defer stmt.Close()
	sc := bufio.NewScanner(zr)
	first := true
	for sc.Scan() {
		if first {
			first = false
			continue
		}
		parts := strings.Split(sc.Text(), ",")
		if len(parts) != 3 {
			continue
		}
		lat, err1 := strconv.ParseFloat(parts[1], 64)
		lon, err2 := strconv.ParseFloat(parts[2], 64)
		if err1 != nil || err2 != nil {
			continue
		}
		if _, err := stmt.Exec(parts[0], lat, lon); err != nil {
			return err
		}
	}
	if err := sc.Err(); err != nil {
		return err
	}
	return tx.Commit()
}

// Haversine returns the great-circle distance in miles.
func Haversine(lat1, lon1, lat2, lon2 float64) float64 {
	const r = 3958.8
	rad := math.Pi / 180
	dlat := (lat2 - lat1) * rad
	dlon := (lon2 - lon1) * rad
	a := math.Sin(dlat/2)*math.Sin(dlat/2) + math.Cos(lat1*rad)*math.Cos(lat2*rad)*math.Sin(dlon/2)*math.Sin(dlon/2)
	return 2 * r * math.Asin(math.Sqrt(a))
}

// BoundingBox returns lat/lon bounds that contain a radius, for index-friendly prefiltering.
func BoundingBox(lat, lon, miles float64) (minLat, maxLat, minLon, maxLon float64) {
	dLat := miles / 69.0
	dLon := miles / (69.0 * math.Cos(lat*math.Pi/180))
	return lat - dLat, lat + dLat, lon - dLon, lon + dLon
}

// StateCodes maps full US state and territory names (ClinicalTrials.gov
// locations) to USPS codes (Open Payments).
var StateCodes = map[string]string{
	"alabama": "AL", "alaska": "AK", "arizona": "AZ", "arkansas": "AR", "california": "CA", "colorado": "CO",
	"connecticut": "CT", "delaware": "DE", "district of columbia": "DC", "florida": "FL", "georgia": "GA",
	"hawaii": "HI", "idaho": "ID", "illinois": "IL", "indiana": "IN", "iowa": "IA", "kansas": "KS",
	"kentucky": "KY", "louisiana": "LA", "maine": "ME", "maryland": "MD", "massachusetts": "MA",
	"michigan": "MI", "minnesota": "MN", "mississippi": "MS", "missouri": "MO", "montana": "MT",
	"nebraska": "NE", "nevada": "NV", "new hampshire": "NH", "new jersey": "NJ", "new mexico": "NM",
	"new york": "NY", "north carolina": "NC", "north dakota": "ND", "ohio": "OH", "oklahoma": "OK",
	"oregon": "OR", "pennsylvania": "PA", "rhode island": "RI", "south carolina": "SC", "south dakota": "SD",
	"tennessee": "TN", "texas": "TX", "utah": "UT", "vermont": "VT", "virginia": "VA", "washington": "WA",
	"west virginia": "WV", "wisconsin": "WI", "wyoming": "WY", "puerto rico": "PR", "guam": "GU",
	"virgin islands": "VI", "u.s. virgin islands": "VI", "american samoa": "AS", "northern mariana islands": "MP",
}

// StateName returns the full name for a USPS code ("" when unknown).
func StateName(code string) string {
	code = strings.ToUpper(code)
	for name, c := range StateCodes {
		if c == code {
			return name
		}
	}
	return ""
}

package students

import (
	"bytes"
	"encoding/csv"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"gopkg.in/yaml.v3"
)

// courseSep separates course slugs inside the single CSV `courses` cell (comma is
// the CSV delimiter, so ';' is used instead).
const courseSep = ";"

// FormatFromExt infers a Format from a filename extension; empty if unknown.
func FormatFromExt(name string) Format {
	switch {
	case strings.HasSuffix(name, ".csv"):
		return FormatCSV
	case strings.HasSuffix(name, ".yaml"), strings.HasSuffix(name, ".yml"):
		return FormatYAML
	case strings.HasSuffix(name, ".json"):
		return FormatJSON
	}
	return ""
}

// FormatFromContentType infers a Format from an HTTP Content-Type; empty if unknown.
func FormatFromContentType(ct string) Format {
	ct = strings.ToLower(ct)
	switch {
	case strings.Contains(ct, "csv"):
		return FormatCSV
	case strings.Contains(ct, "yaml"), strings.Contains(ct, "yml"):
		return FormatYAML
	case strings.Contains(ct, "json"):
		return FormatJSON
	}
	return ""
}

// rosterFile is the YAML/JSON envelope.
type rosterFile struct {
	Users []UserRecord `json:"users" yaml:"users"`
}

// foldLegacyClasses moves any deprecated `classes` entries into Courses, so the
// rest of the package only ever sees the canonical field.
func foldLegacyClasses(recs []UserRecord) []UserRecord {
	for i := range recs {
		if len(recs[i].LegacyClasses) > 0 {
			recs[i].Courses = append(recs[i].Courses, recs[i].LegacyClasses...)
			recs[i].LegacyClasses = nil
		}
	}
	return recs
}

// DecodeRoster parses roster data in the given format into canonical records.
func DecodeRoster(data []byte, f Format) ([]UserRecord, error) {
	switch f {
	case FormatCSV:
		return decodeCSV(data)
	case FormatYAML:
		var rf rosterFile
		if err := yaml.Unmarshal(data, &rf); err != nil {
			return nil, fmt.Errorf("parse yaml: %w", err)
		}
		return foldLegacyClasses(rf.Users), nil
	case FormatJSON:
		var rf rosterFile
		if err := json.Unmarshal(data, &rf); err != nil {
			return nil, fmt.Errorf("parse json: %w", err)
		}
		return foldLegacyClasses(rf.Users), nil
	default:
		return nil, fmt.Errorf("unknown format %q", f)
	}
}

// EncodeRoster serialises records to the given format. Passwords are dropped
// (import-only). CSV omits positions (use YAML/JSON for positions).
func EncodeRoster(recs []UserRecord, f Format) ([]byte, error) {
	for i := range recs {
		recs[i].Password = ""
	}
	switch f {
	case FormatCSV:
		return encodeCSV(recs)
	case FormatYAML:
		return yaml.Marshal(rosterFile{Users: recs})
	case FormatJSON:
		return json.MarshalIndent(rosterFile{Users: recs}, "", "  ")
	default:
		return nil, fmt.Errorf("unknown format %q", f)
	}
}

var csvHeader = []string{"uid", "email", "name", "role", "courses", "password", "verified"}

func decodeCSV(data []byte) ([]UserRecord, error) {
	r := csv.NewReader(bytes.NewReader(data))
	r.TrimLeadingSpace = true
	rows, err := r.ReadAll()
	if err != nil {
		return nil, fmt.Errorf("parse csv: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil
	}
	// map header names to column indices (order-independent).
	idx := map[string]int{}
	for i, h := range rows[0] {
		idx[strings.TrimSpace(strings.ToLower(h))] = i
	}
	col := func(row []string, name string) string {
		if i, ok := idx[name]; ok && i < len(row) {
			return strings.TrimSpace(row[i])
		}
		return ""
	}
	var out []UserRecord
	for _, row := range rows[1:] {
		if len(row) == 0 || strings.TrimSpace(strings.Join(row, "")) == "" {
			continue // skip blank lines
		}
		rec := UserRecord{
			Uid:      col(row, "uid"),
			Email:    col(row, "email"),
			Name:     col(row, "name"),
			Role:     col(row, "role"),
			Password: col(row, "password"),
		}
		cs := col(row, "courses")
		if cs == "" {
			cs = col(row, "classes") // deprecated pre-rename column name
		}
		if cs != "" {
			for _, c := range strings.Split(cs, courseSep) {
				if c = strings.TrimSpace(c); c != "" {
					rec.Courses = append(rec.Courses, c)
				}
			}
		}
		if v := col(row, "verified"); v != "" {
			b, err := strconv.ParseBool(v)
			if err != nil {
				return nil, fmt.Errorf("row %q: bad verified %q", rec.Email, v)
			}
			rec.Verified = &b
		}
		out = append(out, rec)
	}
	return out, nil
}

func encodeCSV(recs []UserRecord) ([]byte, error) {
	var buf bytes.Buffer
	w := csv.NewWriter(&buf)
	if err := w.Write(csvHeader); err != nil {
		return nil, err
	}
	for _, r := range recs {
		verified := ""
		if r.Verified != nil {
			verified = strconv.FormatBool(*r.Verified)
		}
		row := []string{r.Uid, r.Email, r.Name, r.Role, strings.Join(r.Courses, courseSep), r.Password, verified}
		if err := w.Write(row); err != nil {
			return nil, err
		}
	}
	w.Flush()
	if err := w.Error(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

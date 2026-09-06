// Package vtodo implements the small, interoperable VTODO contract used by
// Momentum. It deliberately does not attempt to parse arbitrary iCalendar
// components: Parse accepts one VTODO, optionally wrapped in VCALENDAR.
package vtodo

import (
	"bytes"
	"errors"
	"fmt"
	"net/url"
	"strconv"
	"strings"
	"time"
)

var (
	ErrInvalid = errors.New("invalid VTODO")
)

// Property is an unrecognised VTODO property. Unknown properties are retained
// so an import/export round trip does not discard provider extensions.
type Property struct {
	Name   string
	Params map[string]string
	Value  string
}

// Todo is Momentum's transport-independent VTODO contract. DateOnly is used
// for DTSTART and Due when their wire representation was a DATE value.
type Todo struct {
	UID, Summary, Description, Status string
	Due, Start, Completed             *time.Time
	DueDateOnly, StartDateOnly        bool
	DTStamp                           time.Time
	LastModified                      *time.Time
	Priority                          *int
	PercentComplete                   *int
	Sequence                          int
	Categories, RelatedTo             []string
	URL, Location                     string
	Extra                             []Property
}

// Parse parses a single VTODO component or a VCALENDAR containing one VTODO.
func Parse(data []byte) (*Todo, error) {
	lines, err := unfold(data)
	if err != nil {
		return nil, err
	}
	var in, calendar bool
	t := &Todo{}
	seen := map[string]bool{}
	for _, line := range lines {
		name, params, value, err := property(line)
		if err != nil {
			return nil, err
		}
		switch name {
		case "BEGIN":
			switch strings.ToUpper(value) {
			case "VCALENDAR":
				calendar = true
			case "VTODO":
				if in {
					return nil, fmt.Errorf("%w: nested VTODO", ErrInvalid)
				}
				in = true
			default:
				if !in && !calendar {
					return nil, fmt.Errorf("%w: unexpected component", ErrInvalid)
				}
			}
		case "END":
			switch strings.ToUpper(value) {
			case "VTODO":
				if !in {
					return nil, fmt.Errorf("%w: END without VTODO", ErrInvalid)
				}
				in = false
			case "VCALENDAR":
				if in || !calendar {
					return nil, fmt.Errorf("%w: invalid VCALENDAR boundaries", ErrInvalid)
				}
				calendar = false
			default:
				return nil, fmt.Errorf("%w: unexpected END", ErrInvalid)
			}
		default:
			if !in {
				continue
			}
			if seen[name] && name != "CATEGORIES" && name != "RELATED-TO" {
				return nil, fmt.Errorf("%w: duplicate %s", ErrInvalid, name)
			}
			seen[name] = true
			if err := t.assign(name, params, value); err != nil {
				return nil, err
			}
		}
	}
	if in || calendar {
		return nil, fmt.Errorf("%w: unterminated component", ErrInvalid)
	}
	if err := t.Validate(); err != nil {
		return nil, err
	}
	return t, nil
}

func (t *Todo) assign(name string, params map[string]string, value string) error {
	switch name {
	case "UID":
		t.UID = value
	case "SUMMARY":
		t.Summary = unescape(value)
	case "DESCRIPTION":
		t.Description = unescape(value)
	case "STATUS":
		t.Status = strings.ToUpper(value)
	case "DTSTAMP":
		v, err := parseDate(value, params, false)
		if err != nil {
			return fieldErr(name, err)
		}
		t.DTStamp = v
	case "LAST-MODIFIED":
		v, err := parseDate(value, params, false)
		if err != nil {
			return fieldErr(name, err)
		}
		t.LastModified = &v
	case "DUE":
		v, dateOnly, err := parseDateValue(value, params)
		if err != nil {
			return fieldErr(name, err)
		}
		t.Due, t.DueDateOnly = &v, dateOnly
	case "DTSTART":
		v, dateOnly, err := parseDateValue(value, params)
		if err != nil {
			return fieldErr(name, err)
		}
		t.Start, t.StartDateOnly = &v, dateOnly
	case "COMPLETED":
		v, err := parseDate(value, params, false)
		if err != nil {
			return fieldErr(name, err)
		}
		t.Completed = &v
	case "SEQUENCE":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fieldErr(name, err)
		}
		t.Sequence = v
	case "PRIORITY":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fieldErr(name, err)
		}
		t.Priority = &v
	case "PERCENT-COMPLETE":
		v, err := strconv.Atoi(value)
		if err != nil {
			return fieldErr(name, err)
		}
		t.PercentComplete = &v
	case "CATEGORIES":
		for _, v := range strings.Split(unescape(value), ",") {
			if v = strings.TrimSpace(v); v != "" && !contains(t.Categories, v) {
				t.Categories = append(t.Categories, v)
			}
		}
	case "RELATED-TO":
		t.RelatedTo = append(t.RelatedTo, value)
	case "URL":
		t.URL = unescape(value)
	case "LOCATION":
		t.Location = unescape(value)
	default:
		t.Extra = append(t.Extra, Property{Name: name, Params: params, Value: value})
	}
	return nil
}

// Validate enforces the documented canonical contract and RFC value ranges.
func (t *Todo) Validate() error {
	if strings.TrimSpace(t.UID) == "" {
		return fmt.Errorf("%w: UID is required", ErrInvalid)
	}
	if strings.TrimSpace(t.Summary) == "" {
		return fmt.Errorf("%w: SUMMARY is required", ErrInvalid)
	}
	if len([]rune(t.Summary)) > 255 {
		return fmt.Errorf("%w: SUMMARY exceeds 255 characters", ErrInvalid)
	}
	if t.DTStamp.IsZero() {
		return fmt.Errorf("%w: DTSTAMP is required", ErrInvalid)
	}
	if t.Status == "" {
		t.Status = "NEEDS-ACTION"
	}
	switch t.Status {
	case "NEEDS-ACTION", "IN-PROCESS", "COMPLETED", "CANCELLED":
	default:
		return fmt.Errorf("%w: invalid STATUS", ErrInvalid)
	}
	if t.Sequence < 0 {
		return fmt.Errorf("%w: SEQUENCE must be non-negative", ErrInvalid)
	}
	if t.Priority != nil && (*t.Priority < 0 || *t.Priority > 9) {
		return fmt.Errorf("%w: PRIORITY must be 0..9", ErrInvalid)
	}
	if t.PercentComplete != nil && (*t.PercentComplete < 0 || *t.PercentComplete > 100) {
		return fmt.Errorf("%w: PERCENT-COMPLETE must be 0..100", ErrInvalid)
	}
	if t.Start != nil && t.Due != nil && t.Start.After(*t.Due) {
		return fmt.Errorf("%w: DTSTART is after DUE", ErrInvalid)
	}
	if t.Status == "COMPLETED" && t.Completed == nil {
		return fmt.Errorf("%w: COMPLETED is required for completed tasks", ErrInvalid)
	}
	if t.Status != "COMPLETED" && t.Completed != nil {
		return fmt.Errorf("%w: COMPLETED requires COMPLETED status", ErrInvalid)
	}
	if t.URL != "" {
		if u, err := url.ParseRequestURI(t.URL); err != nil || u.Scheme == "" {
			return fmt.Errorf("%w: invalid URL", ErrInvalid)
		}
	}
	if len([]rune(t.Description)) > 10000 {
		return fmt.Errorf("%w: DESCRIPTION exceeds 10000 characters", ErrInvalid)
	}
	if len(t.Categories) > 20 {
		return fmt.Errorf("%w: too many categories", ErrInvalid)
	}
	for _, c := range t.Categories {
		if len([]rune(c)) > 50 {
			return fmt.Errorf("%w: category exceeds 50 characters", ErrInvalid)
		}
	}
	return nil
}

// Marshal serializes a Todo as a VCALENDAR containing one VTODO, folding lines
// at 75 octets as required by RFC 5545.
func (t *Todo) Marshal() ([]byte, error) {
	if err := t.Validate(); err != nil {
		return nil, err
	}
	var b strings.Builder
	b.WriteString("BEGIN:VCALENDAR\r\nVERSION:2.0\r\nPRODID:-//Momentum//VTODO//EN\r\nBEGIN:VTODO\r\n")
	line := func(name, value string) { b.WriteString(fold(name + ":" + value)); b.WriteString("\r\n") }
	line("UID", t.UID)
	line("DTSTAMP", formatDate(t.DTStamp, false))
	if t.LastModified != nil {
		line("LAST-MODIFIED", formatDate(*t.LastModified, false))
	}
	line("SEQUENCE", strconv.Itoa(t.Sequence))
	line("SUMMARY", escape(t.Summary))
	line("STATUS", t.Status)
	if t.Description != "" {
		line("DESCRIPTION", escape(t.Description))
	}
	if t.Priority != nil {
		line("PRIORITY", strconv.Itoa(*t.Priority))
	}
	if len(t.Categories) > 0 {
		vals := make([]string, len(t.Categories))
		for i, v := range t.Categories {
			vals[i] = escape(v)
		}
		line("CATEGORIES", strings.Join(vals, ","))
	}
	if t.Start != nil {
		line(dateName("DTSTART", t.StartDateOnly), formatDate(*t.Start, t.StartDateOnly))
	}
	if t.Due != nil {
		line(dateName("DUE", t.DueDateOnly), formatDate(*t.Due, t.DueDateOnly))
	}
	if t.Completed != nil {
		line("COMPLETED", formatDate(*t.Completed, false))
	}
	if t.PercentComplete != nil {
		line("PERCENT-COMPLETE", strconv.Itoa(*t.PercentComplete))
	}
	if t.URL != "" {
		line("URL", escape(t.URL))
	}
	if t.Location != "" {
		line("LOCATION", escape(t.Location))
	}
	for _, p := range t.RelatedTo {
		line("RELATED-TO", escape(p))
	}
	for _, p := range t.Extra {
		n := p.Name
		for k, v := range p.Params {
			n += ";" + k + "=" + v
		}
		line(n, p.Value)
	}
	b.WriteString("END:VTODO\r\nEND:VCALENDAR\r\n")
	return []byte(b.String()), nil
}

func dateName(name string, dateOnly bool) string {
	if dateOnly {
		return name + ";VALUE=DATE"
	}
	return name
}
func formatDate(t time.Time, dateOnly bool) string {
	if dateOnly {
		return t.Format("20060102")
	}
	return t.UTC().Format("20060102T150405Z")
}
func parseDateValue(v string, p map[string]string) (time.Time, bool, error) {
	d := strings.EqualFold(p["VALUE"], "DATE") || len(v) == 8
	t, err := parseDate(v, p, d)
	return t, d, err
}
func parseDate(v string, p map[string]string, dateOnly bool) (time.Time, error) {
	if dateOnly {
		return time.ParseInLocation("20060102", v, time.UTC)
	}
	if strings.HasSuffix(v, "Z") {
		return time.Parse("20060102T150405Z", v)
	}
	loc := time.UTC
	if z := p["TZID"]; z != "" {
		var err error
		loc, err = time.LoadLocation(z)
		if err != nil {
			return time.Time{}, err
		}
	}
	return time.ParseInLocation("20060102T150405", v, loc)
}
func fieldErr(n string, e error) error { return fmt.Errorf("%w: invalid %s: %v", ErrInvalid, n, e) }
func contains(a []string, v string) bool {
	for _, x := range a {
		if x == v {
			return true
		}
	}
	return false
}

func property(line string) (string, map[string]string, string, error) {
	i := strings.IndexByte(line, ':')
	if i <= 0 {
		return "", nil, "", fmt.Errorf("%w: malformed content line", ErrInvalid)
	}
	left, value := line[:i], line[i+1:]
	bits := strings.Split(left, ";")
	name := strings.ToUpper(bits[0])
	p := map[string]string{}
	for _, bit := range bits[1:] {
		kv := strings.SplitN(bit, "=", 2)
		if len(kv) != 2 {
			return "", nil, "", fmt.Errorf("%w: malformed parameter", ErrInvalid)
		}
		p[strings.ToUpper(kv[0])] = strings.Trim(kv[1], "\"")
	}
	return name, p, value, nil
}
func unfold(data []byte) ([]string, error) {
	s := strings.ReplaceAll(string(data), "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	raw := strings.Split(s, "\n")
	var out []string
	for _, line := range raw {
		if line == "" {
			continue
		}
		if len(out) > 0 && (line[0] == ' ' || line[0] == '\t') {
			out[len(out)-1] += line[1:]
		} else {
			out = append(out, line)
		}
	}
	if len(out) == 0 {
		return nil, fmt.Errorf("%w: empty input", ErrInvalid)
	}
	return out, nil
}
func escape(s string) string {
	s = strings.ReplaceAll(s, "\\", "\\\\")
	s = strings.ReplaceAll(s, ";", "\\;")
	s = strings.ReplaceAll(s, ",", "\\,")
	s = strings.ReplaceAll(strings.ReplaceAll(s, "\r\n", "\n"), "\n", "\\n")
	return s
}
func unescape(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+1 < len(s) {
			i++
			switch s[i] {
			case 'n', 'N':
				b.WriteByte('\n')
			default:
				b.WriteByte(s[i])
			}
		} else {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}
func fold(s string) string {
	data := []byte(s)
	if len(data) <= 75 {
		return s
	}
	var b bytes.Buffer
	first := true
	for len(data) > 0 {
		n := 75
		if !first {
			n = 74
		}
		if n > len(data) {
			n = len(data)
		}
		// A content line is measured in octets, but never split a UTF-8
		// sequence when inserting a fold.
		for n > 0 && n < len(data) && data[n]&0xc0 == 0x80 {
			n--
		}
		b.Write(data[:n])
		data = data[n:]
		if len(data) > 0 {
			b.WriteString("\r\n ")
		}
		first = false
	}
	return b.String()
}

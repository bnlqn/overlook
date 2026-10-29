// Package ical reads a to-do list: the chores the owner keeps on it, from
// the iCalendar file a CalDAV server such as Radicale, Nextcloud or Baïkal
// gives for a GET (ADR 0010). On the page they are chores, never problems,
// and in no area.
package ical

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Config is one to-do list, under sources.ical in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is the list's iCalendar file: on Radicale the list itself, such as
	// https://dav.example.com/owner/chores/, and on Nextcloud or Baïkal the
	// list with ?export.
	URL      string `yaml:"url"`
	User     string `yaml:"user"`
	Password string `yaml:"password"`
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	if err := fetch.CheckURL(c.URL); err != nil {
		return fmt.Errorf("url %w", err)
	}
	c.Name = cmp.Or(c.Name, "Chores")
	return nil
}

// Read gives back the chores on c's list that aren't done.
func Read(ctx context.Context, c Config) (snapshot.Parts, error) {
	body, err := fetch.Text(ctx, c.URL, "text/calendar", fetch.Login(c.User, c.Password))
	if err != nil {
		return snapshot.Parts{}, err
	}
	chores, err := parse(body, time.Local)
	return snapshot.Parts{Chores: chores}, err
}

// text undoes iCalendar's escapes in a text value.
var text = strings.NewReplacer(`\\`, `\`, `\,`, `,`, `\;`, `;`, `\n`, "\n", `\N`, "\n")

// parse gives back the to-dos in ics that aren't done, each due from the day
// of its DUE, in loc for one given in UTC. A to-do with no DUE, or one that
// can't be read, is due now; a start date doesn't make it due. A to-do that
// comes back is the to-do app's to move on, so its rule is left alone.
func parse(ics []byte, loc *time.Location) ([]snapshot.Chore, error) {
	// A long line goes on in the next one, which starts with a space or a
	// tab; servers end lines in CRLF, or only LF.
	s := strings.ReplaceAll(string(ics), "\r\n", "\n")
	s = strings.NewReplacer("\n ", "", "\n\t", "").Replace(s)
	if !strings.HasPrefix(s, "BEGIN:VCALENDAR") {
		return nil, errors.New("an answer that isn't iCalendar")
	}
	chores := []snapshot.Chore{}
	var c snapshot.Chore
	// in is how deep the reading is within a to-do: 1 in its own lines, more
	// in a part of it, such as a reminder, whose lines aren't the to-do's.
	in, done := 0, false
	for line := range strings.SplitSeq(s, "\n") {
		name, value := split(line)
		switch {
		case name == "BEGIN" && strings.EqualFold(value, "VTODO"):
			in, done, c = 1, false, snapshot.Chore{}
		case in == 0:
		case name == "BEGIN":
			in++
		case name == "END":
			in--
			if in == 0 && !done && c.Name != "" {
				chores = append(chores, c)
			}
		case in > 1:
		case name == "SUMMARY":
			c.Name = text.Replace(value)
		case name == "DESCRIPTION":
			c.Note = text.Replace(value)
		case name == "DUE":
			c.From = day(value, loc)
		case name == "STATUS":
			done = strings.EqualFold(value, "COMPLETED") || strings.EqualFold(value, "CANCELLED")
		case name == "COMPLETED":
			done = true
		}
	}
	return chores, nil
}

// split cuts a line into its name, without its parameters, and its value,
// at the first colon outside a quoted parameter.
func split(line string) (name, value string) {
	quoted := false
	for i, r := range line {
		switch {
		case r == '"':
			quoted = !quoted
		case r == ':' && !quoted:
			name, _, _ = strings.Cut(line[:i], ";")
			return strings.ToUpper(name), line[i+1:]
		}
	}
	return "", ""
}

// day is the day of a DUE as YYYY-MM-DD: a date as it is, a time by its
// own zone's calendar, which its value already is, and a time in UTC by
// loc's. It's empty for a value it can't read.
func day(value string, loc *time.Location) string {
	if t, err := time.Parse("20060102T150405Z", value); err == nil {
		return t.In(loc).Format(time.DateOnly)
	}
	if len(value) < 8 {
		return ""
	}
	t, err := time.Parse("20060102", value[:8])
	if err != nil {
		return ""
	}
	return t.Format(time.DateOnly)
}

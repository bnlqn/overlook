package snapshot

import "time"

// Areas are the four fixed parts of a homelab, in the page's order (ADR 0001).
var Areas = []string{"servers", "network", "storage", "apps"}

// Tones are the colours a value may ask for; empty is the page's own.
var Tones = []string{"", "green", "orange", "red", "blue", "gray"}

// Text is a message key with its values (ADR 0006), which the page turns into
// words from its table, or, with no key, a name shown as written.
//
// A message's {x} writes Args["x"]: a number the viewer's way, anything else
// escaped. {x:format} formats it: span and ms take milliseconds, when, day
// and clock take a time, bytes takes bytes, area takes an area. A message
// with plurals picks its form by Args["n"].
type Text struct {
	Key  string         `json:"key,omitempty"`
	Args map[string]any `json:"args,omitempty"`
	Name string         `json:"name,omitempty"`
}

// Check is one check of a check engine and its recent results.
type Check struct {
	// Key is the check engine's own key for the check.
	Key  string `json:"key"`
	Name string `json:"name"`
	// Area is empty for a check in no area: it still counts, and its
	// problems still show.
	Area string `json:"area,omitempty"`
	// Source is the name of the check engine, as the config gives it.
	Source string `json:"source"`
	// URL is the check's own page in the check engine.
	URL string `json:"url"`
	// Threshold is how many failures in a row make a problem: the same
	// number as the check engine's alerts.
	Threshold int `json:"threshold"`
	// Run is the failures in a row, up to the newest result.
	Run     int  `json:"run"`
	Problem bool `json:"problem"`
	// Why is the newest result's failure, as the check engine words it.
	Why string `json:"why,omitempty"`
	// Since is when the run of failures started, if the check engine
	// remembers.
	Since *time.Time `json:"since,omitempty"`
	// Results are the recent results, oldest first.
	Results []Result `json:"results"`
	// Outages are the problems the check engine remembers, the one still
	// going without an end.
	Outages []Outage `json:"outages,omitempty"`
	// Watched is when the check engine started the check, if it remembers.
	Watched *time.Time `json:"watched,omitempty"`
}

// Result is one run of a check.
type Result struct {
	OK bool      `json:"ok"`
	At time.Time `json:"at"`
	// MS is how long the check took, in milliseconds, if the engine says.
	MS  *float64 `json:"ms"`
	Why string   `json:"why,omitempty"`
}

// Outage is a stretch a check was a problem.
type Outage struct {
	From time.Time  `json:"from"`
	To   *time.Time `json:"to"`
}

// Section is a titled list of rows an adapter adds to its area's page.
type Section struct {
	Area  string `json:"area"`
	Title Text   `json:"title"`
	Rows  []Row  `json:"rows"`
}

// Chart is what a source counts by the hour, over as many hours as it keeps,
// drawn on its area's page, such as a resolver's lookups over a week. Its
// first series is the whole and each next one a part within it, such as the
// blocked lookups, all as long. Charts that add up and share a title key are
// drawn as one, such as two resolvers': hour by hour over the hours they all
// have, each source's own count in the tooltip.
type Chart struct {
	Area   string   `json:"area"`
	Title  Text     `json:"title"`
	Series []Series `json:"series"`
	Adds   bool     `json:"adds,omitempty"`
	// Source is the name of the source it came from, which the reader fills
	// in, as the config gives it.
	Source string `json:"source"`
}

// Series is a count by the hour, oldest first, ending with this hour.
type Series struct {
	Label Text      `json:"label"`
	Hours []float64 `json:"hours"`
}

// Row is one line of a section: a title, a value, a line under them, and
// sometimes a bar for how full something is.
type Row struct {
	Title  Text   `json:"title"`
	Value  *Text  `json:"value,omitempty"`
	Detail *Text  `json:"detail,omitempty"`
	Tone   string `json:"tone,omitempty"`
	Meter  *Meter `json:"meter,omitempty"`
}

// Meter is how full something is. With no tone it's blue, turning orange
// past 80% and red past 90%, as a disk's does.
type Meter struct {
	// Label names a figure's meter, such as Battery; a row's meter has the
	// row's title.
	Label *Text   `json:"label,omitempty"`
	Used  float64 `json:"used"`
	Total float64 `json:"total"`
	Tone  string  `json:"tone,omitempty"`
}

// Figure is a number for today, sometimes with the hours that make it up or
// the shares it splits into.
type Figure struct {
	Area string `json:"area"`
	// Icon is one of the page's own icons, such as shield or photos.
	Icon   string  `json:"icon"`
	Label  Text    `json:"label"`
	Value  float64 `json:"value"`
	Unit   *Text   `json:"unit,omitempty"`
	Detail *Text   `json:"detail,omitempty"`
	Tone   string  `json:"tone,omitempty"`
	// Hours are the last 24 hours, oldest first, ending with this one: a
	// line under the figure, and a chart of their own on the overview.
	Hours []float64 `json:"hours,omitempty"`
	// Within is the part of each of the hours its label names, such as the
	// blocked lookups.
	Within *Series `json:"within,omitempty"`
	// Shares split the figure into its parts, such as each person's photos,
	// drawn as one bar under it.
	Shares []Share `json:"shares,omitempty"`
	// Meters are drawn under the figure, each with its label, such as a
	// battery's charge.
	Meters []Meter `json:"meters,omitempty"`
	// Adds says the page may add it up with the others that add up and share
	// its label key, such as two resolvers' lookups: their values, their
	// hours and the parts within them hour by hour, and the numbers in their
	// details, which must therefore be counts.
	Adds bool `json:"adds,omitempty"`
}

// Share is one part of a figure, such as one person's photos. Its width in
// the bar is its value's share of them all, and its colour its tone or one
// the page picks. Its tooltip is its label over its rows, such as the
// person's photos, videos and space, or over its value and share without.
type Share struct {
	Label Text    `json:"label"`
	Value float64 `json:"value"`
	Tone  string  `json:"tone,omitempty"`
	Rows  []Row   `json:"rows,omitempty"`
}

// Event is something the homelab did this week, as a source remembers it.
type Event struct {
	At   time.Time `json:"at"`
	Area string    `json:"area"`
	// Icon is one of the page's own icons, such as archive.
	Icon string `json:"icon"`
	Tone string `json:"tone,omitempty"`
	Text Text   `json:"text"`
}

// Update is one waiting for the owner's merge.
type Update struct {
	Name string    `json:"name"`
	URL  string    `json:"url"`
	At   time.Time `json:"at"`
}

// Parts are what one source gives back, in Overlook's shapes.
type Parts struct {
	Checks   []Check   `json:"checks"`
	Sections []Section `json:"sections"`
	Charts   []Chart   `json:"charts"`
	Figures  []Figure  `json:"figures"`
	Events   []Event   `json:"events"`
	Updates  []Update  `json:"updates"`
}

// Link is an app or a tool the page links to, from the config.
type Link struct {
	Name string `yaml:"name" json:"name"`
	URL  string `yaml:"url" json:"url"`
	// Icon is a file in icons/, the owner's or one Overlook ships, or a URL.
	Icon string `yaml:"icon" json:"icon,omitempty"`
	// Check is the name of the check that says whether it answers.
	Check       string `yaml:"check" json:"check,omitempty"`
	Description string `yaml:"description" json:"description,omitempty"`
	// Color tints its tile, as #rgb or #rrggbb.
	Color string `yaml:"color" json:"color,omitempty"`
}

// Chore is maintenance the owner wrote down in the config, due from a day.
// It is done when its lines leave the config, since Overlook remembers
// nothing (ADR 0004).
type Chore struct {
	Name string `yaml:"name" json:"name"`
	// From is the day it's due from, as YYYY-MM-DD: a day of the viewer's
	// calendar, not a moment.
	From string `yaml:"from" json:"from"`
	Note string `yaml:"note" json:"note,omitempty"`
}

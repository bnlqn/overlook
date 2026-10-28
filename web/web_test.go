package web

import (
	"strings"
	"testing"
	"testing/fstest"
)

func TestEnglish(t *testing.T) {
	t.Parallel()
	table, err := English()
	if err != nil {
		t.Fatal(err)
	}
	if table["title"] != "Homelab" {
		t.Errorf("the page's own words are in the table: %v", table["title"])
	}

	both := fstest.MapFS{
		"i18n/en/page.json":  {Data: []byte(`{"title": "Homelab"}`)},
		"i18n/en/gatus.json": {Data: []byte(`{"gatus.checks": "Checks", "title": "Overlook"}`)},
	}
	if _, err := english(both); err == nil || !strings.Contains(err.Error(), "title is in both") {
		t.Errorf("a key two files give is refused: %v", err)
	}
}

// Package web is the page, embedded in the binary (ADR 0003): plain HTML,
// CSS and JavaScript modules, drawn from the snapshot, with no build.
package web

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"path"
)

//go:embed *.html style.css js i18n icons fonts
var files embed.FS

// FS holds the page's files, as served.
var FS fs.FS = files

// English is the page's English words and every adapter's (ADR 0006): one
// table from the files in i18n/en/, each adapter's in its own file. A key in
// two files is an error.
func English() (map[string]any, error) {
	return english(files)
}

func english(fsys fs.FS) (map[string]any, error) {
	names, err := fs.Glob(fsys, "i18n/en/*.json")
	if err != nil {
		return nil, err
	}
	table := map[string]any{}
	from := map[string]string{}
	for _, name := range names {
		raw, err := fs.ReadFile(fsys, name)
		if err != nil {
			return nil, err
		}
		var words map[string]any
		if err := json.Unmarshal(raw, &words); err != nil {
			return nil, fmt.Errorf("%s: %w", name, err)
		}
		for key, entry := range words {
			if other, ok := from[key]; ok {
				return nil, fmt.Errorf("%s is in both %s and %s", key, path.Base(other), path.Base(name))
			}
			table[key], from[key] = entry, name
		}
	}
	return table, nil
}

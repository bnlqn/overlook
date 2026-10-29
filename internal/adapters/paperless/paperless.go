// Package paperless reads Paperless-ngx: how many documents it keeps and how
// many wait in the inbox, the room left on its disk, the health of the parts
// it runs on, and each document it failed to take in.
package paperless

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "apps"

// version is the version of Paperless-ngx's API whose answers the adapter
// reads, asked for by name so that a newer default doesn't change them.
const version = "10"

// Config is one Paperless-ngx, under sources.paperless in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads Paperless-ngx, such as
	// http://paperless:8000.
	URL string `yaml:"url"`
	// Token is the API token of a user of Overlook's own.
	Token string `yaml:"token"`
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	if err := fetch.CheckURL(c.URL); err != nil {
		return fmt.Errorf("url %w", err)
	}
	if c.Token == "" {
		return errors.New("token is missing")
	}
	c.Name = cmp.Or(c.Name, "Paperless-ngx")
	return nil
}

// get reads the answer at Paperless-ngx's API path into v, with the token,
// at the API's version.
func (c Config) get(ctx context.Context, path string, v any) error {
	body, err := fetch.Text(ctx, strings.TrimSuffix(c.URL, "/")+"/api"+path, "application/json; version="+version,
		http.Header{"Authorization": {"Token " + c.Token}})
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, v); err != nil {
		return fmt.Errorf("an answer that isn't the JSON expected: %w", err)
	}
	return nil
}

// status is /api/status/: the room on the disk, and how each part
// Paperless-ngx runs on is, as OK, WARNING or ERROR with why.
type status struct {
	Version string `json:"pngx_version"`
	Storage struct {
		Total     int64 `json:"total"`
		Available int64 `json:"available"`
	} `json:"storage"`
	Database struct {
		Status string `json:"status"`
		Error  string `json:"error"`
	} `json:"database"`
	Tasks map[string]any `json:"tasks"`
}

// parts are the parts /api/status/ names under tasks, each by the prefix
// of its status and error, with its words.
var parts = []struct{ prefix, key string }{
	{"redis", "paperless.redis"},
	{"celery", "paperless.celery"},
	{"index", "paperless.index"},
	{"classifier", "paperless.classifier"},
	{"sanity_check", "paperless.sanity_check"},
}

// task is one of /api/tasks/: a document it took in, or a job of its own.
type task struct {
	Created time.Time `json:"date_created"`
	Input   struct {
		Filename string `json:"filename"`
	} `json:"input_data"`
	Result struct {
		Message string `json:"error_message"`
	} `json:"result_data"`
	Type string `json:"task_type_display"`
}

// Read reads the Paperless-ngx at c, which Check has filled.
func Read(ctx context.Context, c Config) (snapshot.Parts, error) {
	var st status
	var stats struct {
		Total int  `json:"documents_total"`
		Inbox *int `json:"documents_inbox"`
	}
	var failed struct {
		Results []task `json:"results"`
	}
	for _, r := range []struct {
		path string
		v    any
	}{
		{"/status/", &st},
		{"/statistics/", &stats},
		// The failures not yet dismissed in Paperless-ngx, which shows them
		// until they are, the newest first.
		{"/tasks/?status=failure&acknowledged=false&ordering=-date_created&page_size=10", &failed},
	} {
		if err := c.get(ctx, r.path, r.v); err != nil {
			return snapshot.Parts{}, err
		}
	}

	documents := snapshot.Row{Title: snapshot.Text{Key: "paperless.documents"}, Value: snapshot.Number(stats.Total)}
	if stats.Inbox != nil {
		documents.Detail = snapshot.Msg("paperless.inbox", map[string]any{"n": *stats.Inbox})
	}
	rows := []snapshot.Row{documents}
	if total := st.Storage.Total; total > 0 {
		rows = append(rows, snapshot.Row{
			Title: snapshot.Text{Key: "paperless.space"},
			Value: snapshot.Msg("paperless.free", map[string]any{"n": st.Storage.Available}),
			Meter: &snapshot.Meter{Used: float64(total - st.Storage.Available), Total: float64(total)},
		})
	}

	// The parts it runs on that aren't well, as it words why; with all of
	// them well, one row says so.
	ill := 0
	add := func(key, state, why string) {
		tone := map[string]string{"WARNING": "orange", "ERROR": "red"}[state]
		if tone == "" {
			return
		}
		ill++
		row := snapshot.Row{Title: snapshot.Text{Key: key}, Tone: tone, Value: snapshot.Msg("paperless.warning", nil)}
		if tone == "red" {
			row.Value = snapshot.Msg("paperless.error", nil)
		}
		if why != "" {
			row.Detail = &snapshot.Text{Name: why}
		}
		rows = append(rows, row)
	}
	add("paperless.database", st.Database.Status, st.Database.Error)
	for _, p := range parts {
		state, _ := st.Tasks[p.prefix+"_status"].(string)
		why, _ := st.Tasks[p.prefix+"_error"].(string)
		add(p.key, state, why)
	}
	if ill == 0 {
		rows = append(rows, snapshot.Row{Title: snapshot.Text{Key: "paperless.health"}, Value: snapshot.Msg("paperless.healthy", nil), Tone: "green"})
	}

	// Each document it failed to take in, with why.
	for _, t := range failed.Results {
		rows = append(rows, snapshot.Row{
			Title: snapshot.Text{Name: cmp.Or(t.Input.Filename, t.Type)}, Tone: "red",
			Value:  snapshot.Msg("paperless.failed", map[string]any{"at": t.Created}),
			Detail: &snapshot.Text{Name: t.Result.Message},
		})
	}
	rows = append(rows, snapshot.Row{Title: snapshot.Text{Key: "paperless.version"}, Value: &snapshot.Text{Name: st.Version}})

	// Today: the documents it keeps, and those waiting in the inbox.
	f := snapshot.Figure{Area: Area, Icon: "document", Label: snapshot.Text{Key: "paperless.documents"}, Value: float64(stats.Total)}
	if stats.Inbox != nil {
		f.Detail = snapshot.Msg("paperless.inbox", map[string]any{"n": *stats.Inbox})
	}
	if len(failed.Results) > 0 || ill > 0 {
		f.Tone = "orange"
	}
	return snapshot.Parts{
		Sections: []snapshot.Section{{Area: Area, Title: snapshot.Text{Key: "paperless.documents"}, Rows: rows}},
		Figures:  []snapshot.Figure{f},
	}, nil
}

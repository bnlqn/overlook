// Package nextcloud reads Nextcloud, from its serverinfo app: its files and
// shares, who used it today, the room left, its database, whether it's in
// maintenance or a newer one is out, and, as chores, the apps waiting for an
// update.
package nextcloud

import (
	"cmp"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"net/http"
	"slices"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "apps"

// Config is one Nextcloud, under sources.nextcloud in overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads Nextcloud, such as http://nextcloud.
	URL string `yaml:"url"`
	// Token is serverinfo's own, which reads its answer and nothing else.
	Token string `yaml:"token"`
	// Link is where the page sends the owner to update an app, if not URL.
	Link string `yaml:"link"`

	source fetch.Source
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	c.source = fetch.Source{URL: c.URL, Header: http.Header{"NC-Token": {c.Token}}}
	if err := c.source.Check(); err != nil {
		return err
	}
	if c.Token == "" {
		return errors.New("token is missing: serverinfo's, as the README says")
	}
	c.Name = cmp.Or(c.Name, "Nextcloud")
	c.Link = cmp.Or(c.Link, c.URL)
	if err := fetch.CheckURL(c.Link); err != nil {
		return fmt.Errorf("link %w", err)
	}
	return nil
}

// info is serverinfo's answer, with the apps and the update it leaves out
// unless asked.
type info struct {
	OCS struct {
		Data struct {
			Nextcloud struct {
				System struct {
					Free int64 `json:"freespace"`
					Apps struct {
						// Updates are each app's newer version by its id, which
						// PHP writes as [] when there are none.
						Updates json.RawMessage `json:"app_updates"`
					} `json:"apps"`
					Update struct {
						Available bool   `json:"available"`
						Version   string `json:"available_version"`
					} `json:"update"`
				} `json:"system"`
				Storage struct {
					Users int `json:"num_users"`
					Files int `json:"num_files"`
				} `json:"storage"`
				Shares struct {
					All             int `json:"num_shares"`
					Links           int `json:"num_shares_link"`
					LinksNoPassword int `json:"num_shares_link_no_password"`
				} `json:"shares"`
			} `json:"nextcloud"`
			Server struct {
				Database struct {
					Type string `json:"type"`
					Size int64  `json:"size"`
				} `json:"database"`
			} `json:"server"`
			Active struct {
				Day int `json:"last24hours"`
			} `json:"activeUsers"`
		} `json:"data"`
	} `json:"ocs"`
}

// Read reads the Nextcloud at c, which Check has filled.
func Read(ctx context.Context, c Config, _ time.Time) (snapshot.Parts, error) {
	var status struct {
		Maintenance bool   `json:"maintenance"`
		Upgrade     bool   `json:"needsDbUpgrade"`
		Version     string `json:"versionstring"`
	}
	if err := c.source.JSON(ctx, "/status.php", &status); err != nil {
		return snapshot.Parts{}, err
	}
	version := snapshot.Row{Title: snapshot.Text{Key: "nextcloud.version"}, Value: &snapshot.Text{Name: status.Version}}
	// In maintenance, or waiting for its upgrade to be finished, Nextcloud
	// serves nobody, serverinfo included.
	if status.Maintenance || status.Upgrade {
		version.Value, version.Tone, version.Detail = snapshot.Msg("nextcloud.maintenance", nil), "red", &snapshot.Text{Name: status.Version}
		if status.Upgrade {
			version.Value = snapshot.Msg("nextcloud.upgrade", nil)
		}
		return snapshot.Parts{Sections: []snapshot.Section{{Area: Area, Title: snapshot.Text{Key: "nextcloud.files"}, Rows: []snapshot.Row{version}}}}, nil
	}

	var in info
	if err := c.source.JSON(ctx, "/ocs/v2.php/apps/serverinfo/api/v1/info?format=json&skipApps=false&skipUpdate=false", &in); err != nil {
		return snapshot.Parts{}, err
	}
	d := in.OCS.Data
	nc := d.Nextcloud
	rows := []snapshot.Row{
		{
			Title:  snapshot.Text{Key: "nextcloud.files"},
			Value:  snapshot.Number(nc.Storage.Files),
			Detail: snapshot.Msg("nextcloud.people", map[string]any{"n": nc.Storage.Users, "active": d.Active.Day}),
		},
		{
			Title:  snapshot.Text{Key: "nextcloud.shares"},
			Value:  snapshot.Number(nc.Shares.All),
			Detail: snapshot.Msg("nextcloud.links", map[string]any{"n": nc.Shares.Links, "open": nc.Shares.LinksNoPassword}),
		},
		{Title: snapshot.Text{Key: "nextcloud.free"}, Value: snapshot.Msg("nextcloud.bytes", map[string]any{"n": nc.System.Free})},
		{
			Title: snapshot.Text{Key: "nextcloud.database"}, Value: snapshot.Msg("nextcloud.bytes", map[string]any{"n": d.Server.Database.Size}),
			Detail: &snapshot.Text{Name: d.Server.Database.Type},
		},
	}
	if u := nc.System.Update; u.Available && u.Version != "" {
		version.Value, version.Tone = snapshot.Msg("nextcloud.newer", map[string]any{"version": u.Version}), "orange"
		version.Detail = &snapshot.Text{Name: status.Version}
	}
	rows = append(rows, version)

	// Each app waiting for an update, done on Nextcloud's page of updates,
	// with the version it would go to, as words of no language.
	updates := map[string]string{}
	if len(nc.System.Apps.Updates) > 0 && nc.System.Apps.Updates[0] == '{' {
		if err := json.Unmarshal(nc.System.Apps.Updates, &updates); err != nil {
			return snapshot.Parts{}, fmt.Errorf("an answer that isn't the JSON expected: %w", err)
		}
	}
	var chores []snapshot.Chore
	for _, app := range slices.Sorted(maps.Keys(updates)) {
		chores = append(chores, snapshot.Chore{Name: app, Note: "→ " + updates[app], URL: strings.TrimSuffix(c.Link, "/") + "/settings/apps/updates"})
	}

	// Today: the files, and the people who used Nextcloud since yesterday.
	f := snapshot.Figure{
		Area: Area, Icon: "cloud", Label: snapshot.Text{Key: "nextcloud.files"}, Value: float64(nc.Storage.Files),
		Detail: snapshot.Msg("nextcloud.active", map[string]any{"n": d.Active.Day}),
	}
	return snapshot.Parts{
		Sections: []snapshot.Section{{Area: Area, Title: snapshot.Text{Key: "nextcloud.files"}, Rows: rows}},
		Figures:  []snapshot.Figure{f},
		Chores:   chores,
	}, nil
}

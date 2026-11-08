// Package jellyfin reads Jellyfin, a media server: what mediabrowser reads of
// it and of Emby alike, and the room left on the disks its libraries are on.
package jellyfin

import (
	"context"
	"net/http"
	"slices"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/mediabrowser"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = mediabrowser.Area

// Adapter says what this adapter is, for the config and its test.
var Adapter = snapshot.Adapter[Config]{Key: "jellyfin", Area: Area, Kind: "mediaserver", Read: Read}

// Config is one Jellyfin, under sources.jellyfin in overlook.yaml.
type Config struct {
	mediabrowser.Config `yaml:",inline" example:"http://jellyfin.example.com:8096"`
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	return c.Config.Check("Jellyfin", http.Header{"Authorization": {`MediaBrowser Token="` + c.Key + `"`}})
}

// log is how Jellyfin types a play and a failed login in its activity log.
var log = mediabrowser.Log{Plays: []string{"VideoPlayback", "AudioPlayback"}, Failed: "AuthenticationFailed"}

// storage is where each library's folders are, and the room on their disks.
type storage struct {
	Libraries []struct {
		Folders []struct {
			FreeSpace int64  `json:"FreeSpace"`
			UsedSpace int64  `json:"UsedSpace"`
			DeviceID  string `json:"DeviceId"`
		} `json:"Folders"`
	} `json:"Libraries"`
}

// Read reads the Jellyfin at c, which Check has filled.
func Read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	var disks storage
	if err := c.Get(ctx, "/System/Info/Storage", &disks); err != nil {
		return snapshot.Parts{}, err
	}
	// The room on each disk the libraries are on, once each, by where it's
	// mounted.
	var rows []snapshot.Row
	var seen []string
	for _, l := range disks.Libraries {
		for _, f := range l.Folders {
			if slices.Contains(seen, f.DeviceID) || f.FreeSpace+f.UsedSpace <= 0 {
				continue
			}
			seen = append(seen, f.DeviceID)
			rows = append(rows, snapshot.Row{
				Title: snapshot.Text{Name: f.DeviceID},
				Value: snapshot.Msg("jellyfin.free", map[string]any{"n": f.FreeSpace}),
				Meter: &snapshot.Meter{Used: float64(f.UsedSpace), Total: float64(f.FreeSpace + f.UsedSpace)},
			})
		}
	}
	return mediabrowser.Read(ctx, c.Config, now, log, rows)
}

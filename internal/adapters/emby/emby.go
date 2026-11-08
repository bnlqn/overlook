// Package emby reads Emby, a media server: what mediabrowser reads of it and
// of Jellyfin alike. Emby doesn't say how much room is left on its disks, as
// Jellyfin does, so this adapter shows none.
package emby

import (
	"context"
	"net/http"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/mediabrowser"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = mediabrowser.Area

// Adapter says what this adapter is, for the config and its test.
var Adapter = snapshot.Adapter[Config]{Key: "emby", Area: Area, Kind: "mediaserver", Read: Read}

// Config is one Emby, under sources.emby in overlook.yaml.
type Config struct {
	mediabrowser.Config `yaml:",inline" example:"http://emby.example.com:8096"`
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	return c.Config.Check("Emby", http.Header{"X-Emby-Token": {c.Key}})
}

// log is how Emby types a play and a failed login in its activity log. Emby
// logs a play when it starts and again when it stops, and only the start is
// a play.
var log = mediabrowser.Log{Plays: []string{"playback.start"}, Failed: "user.authenticationfailed"}

// Read reads the Emby at c, which Check has filled.
func Read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	return mediabrowser.Read(ctx, c.Config, now, log, nil)
}

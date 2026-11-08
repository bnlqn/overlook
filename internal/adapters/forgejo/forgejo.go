// Package forgejo reads Forgejo, which was forked from Gitea and still
// answers alike, as the gitea adapter reads it: the updates waiting, the
// Actions runners and the runs that failed this week.
package forgejo

import (
	"context"
	"time"

	"github.com/bnlqn/overlook/internal/adapters/gitea"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = gitea.Area

// Adapter says what this adapter is, for the config and its test.
var Adapter = snapshot.Adapter[Config]{Key: "forgejo", Area: Area, Read: Read}

// Config is one Forgejo, under sources.forgejo in overlook.yaml.
type Config struct {
	gitea.Config `yaml:",inline" example:"http://forgejo.example.com:3000"`
}

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error { return c.CheckAs("Forgejo") }

// Read reads the Forgejo at c, which Check has filled.
func Read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	return gitea.Read(ctx, c.Config, now)
}

// Package pbs reads Proxmox Backup Server: one datastore's room, the nights
// that made its backups, and each guest's newest backup, with its size and
// whether it passed its check. Everything it gives goes in Storage.
package pbs

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/bnlqn/overlook/internal/fetch"
	"github.com/bnlqn/overlook/internal/snapshot"
)

// Area is the adapter's home area.
const Area = "storage"

// night is the longest gap between two backups of one run. Backups further
// apart are two runs.
// ponytail: a run whose one guest takes longer than this splits in two;
// read the backup jobs' tasks if that's ever seen.
const night = 6 * time.Hour

// Config is one datastore of a Proxmox Backup Server, under sources.pbs in
// overlook.yaml.
type Config struct {
	Name string `yaml:"name" setup:"text,optional"`
	// URL is where Overlook reads PBS, such as https://pbs.example.com:8007.
	URL string `yaml:"url" setup:"text" example:"https://pbs.example.com:8007"`
	// Token is an API token's ID, as user@realm!name, and Secret its secret.
	Token  string `yaml:"token" setup:"text" example:"overlook@pbs!overlook"`
	Secret string `yaml:"secret" setup:"key"`
	// Datastore is the datastore whose backups are read.
	Datastore string `yaml:"datastore" setup:"text" example:"backups"`
	// CA is a PEM file of the CA that signed PBS's certificate, or of the
	// certificate itself, such as the server's
	// /etc/proxmox-backup/proxy.pem, when the system doesn't trust it.
	CA string `yaml:"ca" setup:"text,optional" example:"/etc/overlook/pbs.pem"`
	// ServerName is the name PBS's certificate is for, when the URL reaches
	// it by another, such as its address.
	ServerName string `yaml:"server_name" setup:"text,optional" example:"pbs.example.com"`

	source fetch.Source
}

var tokenID = regexp.MustCompile(`^[^@!\s]+@[^@!\s]+![^@!\s]+$`)

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	c.source = fetch.Source{URL: c.URL, CA: c.CA, ServerName: c.ServerName, Header: http.Header{"Authorization": {"PBSAPIToken=" + c.Token + ":" + c.Secret}}}
	if err := c.source.Check(); err != nil {
		return err
	}
	if !tokenID.MatchString(c.Token) {
		return errors.New("token needs to be an API token's ID, such as overlook@pbs!overlook")
	}
	if c.Secret == "" {
		return errors.New("secret is missing")
	}
	if c.Datastore == "" {
		return errors.New("datastore is missing")
	}
	c.Name = cmp.Or(c.Name, "Backup Server")
	return nil
}

// backup is one snapshot in the datastore, from
// /admin/datastore/{store}/snapshots.
type backup struct {
	Type string `json:"backup-type"`
	ID   string `json:"backup-id"`
	Time int64  `json:"backup-time"`
	// Comment is the first line of its notes, which Proxmox VE fills with
	// the guest's name.
	Comment      string `json:"comment"`
	Size         int64  `json:"size"`
	Verification *struct {
		State string `json:"state"`
	} `json:"verification"`
}

// room is the datastore's, in bytes, from /admin/datastore/{store}/status:
// that of the file system it's on.
type room struct {
	Total int64 `json:"total"`
	Used  int64 `json:"used"`
}

// Read reads c's datastore: its room and its backups.
func Read(ctx context.Context, c Config, now time.Time) (snapshot.Parts, error) {
	path := "/api2/json/admin/datastore/" + url.PathEscape(c.Datastore)
	var status struct {
		Data room `json:"data"`
	}
	if err := c.source.JSON(ctx, path+"/status", &status); err != nil {
		return snapshot.Parts{}, err
	}
	var snapshots struct {
		Data []backup `json:"data"`
	}
	if err := c.source.JSON(ctx, path+"/snapshots", &snapshots); err != nil {
		return snapshot.Parts{}, err
	}
	return parts(c.Datastore, status.Data, snapshots.Data, now), nil
}

// parts are the datastore's room as a store, its nights as backup runs,
// which go in Storage as every source's do (ADR 0016), then its own rows
// after them: each guest's newest backup.
func parts(datastore string, r room, backups []backup, now time.Time) snapshot.Parts {
	// PBS says nothing of a datastore's health.
	p := snapshot.Stores{{Name: snapshot.Text{Name: datastore}, Used: r.Used, Size: r.Total, Healthy: true}}.Parts()
	if len(backups) == 0 {
		return p
	}
	slices.SortFunc(backups, func(a, b backup) int { return cmp.Compare(b.Time, a.Time) })

	// Each guest's newest backup, by its ID.
	var newest []backup
	seen := map[string]bool{}
	for _, b := range backups {
		if g := b.Type + "/" + b.ID; !seen[g] {
			seen[g] = true
			newest = append(newest, b)
		}
	}
	// Guests by number, as Proxmox VE lists them, then hosts by name.
	slices.SortFunc(newest, func(a, b backup) int {
		x, errX := strconv.Atoi(a.ID)
		y, errY := strconv.Atoi(b.ID)
		switch {
		case errX == nil && errY == nil:
			return cmp.Compare(x, y)
		case errX == nil:
			return -1
		case errY == nil:
			return 1
		}
		return strings.Compare(a.Type+"/"+a.ID, b.Type+"/"+b.ID)
	})
	rows := make([]snapshot.Row, len(newest))
	for i, b := range newest {
		args := map[string]any{"group": b.Type + "/" + b.ID, "size": b.Size, "at": at(b)}
		title := snapshot.Text{Name: b.Comment}
		if b.Comment == "" {
			title.Name = b.Type + "/" + b.ID
		}
		rows[i] = snapshot.Row{Title: title, Value: snapshot.Msg("pbs.when", args), Detail: snapshot.Msg("pbs.unverified", args)}
		switch {
		case b.Verification == nil:
		case b.Verification.State == "ok":
			rows[i].Detail = snapshot.Msg("pbs.verified", args)
		default:
			rows[i].Value, rows[i].Tone, rows[i].Detail = snapshot.Msg("pbs.failed", nil), "red", snapshot.Msg("pbs.failed_detail", args)
		}
	}

	// The nights: the backups made one after another, newest first. PBS
	// knows when each backup started, not when the night ended.
	var runs [][]backup
	var nights snapshot.BackupRuns
	for i, b := range backups {
		if i == 0 || at(backups[i-1]).Sub(at(b)) > night {
			runs = append(runs, nil)
		}
		runs[len(runs)-1] = append(runs[len(runs)-1], b)
	}
	for _, r := range runs {
		nights = append(nights, snapshot.BackupRun{Start: at(r[len(r)-1]), Backups: len(r)})
	}
	n := nights.Parts(now)
	p.Sections = append(p.Sections, n.Sections...)
	p.Sections = append(p.Sections, snapshot.Section{Area: Area, Title: snapshot.Text{Key: "pbs.backups"}, Rows: rows})
	p.Events = n.Events

	// Today: the guests the last night backed up, of all those backed up. The
	// night is the run with the most guests in the day to the newest backup,
	// so a guest backed up by hand after it neither hides it nor reads as a
	// night that missed the rest.
	// ponytail: a run by hand of more guests than the night's is taken for
	// it; read the backup jobs' tasks if that's ever seen.
	last := runs[0]
	for _, r := range runs[1:] {
		if at(runs[0][0]).Sub(at(r[0])) > 24*time.Hour {
			break
		}
		if guests(r) > guests(last) {
			last = r
		}
	}
	// Under it, each guest, blue where the night backed it up, with its
	// newest backup's time, size and check.
	tonight := map[string]bool{}
	for _, b := range last {
		tonight[b.Type+"/"+b.ID] = true
	}
	shares := make([]snapshot.Share, len(newest))
	for i, b := range newest {
		in, tone := "pbs.not_in_night", "gray"
		if tonight[b.Type+"/"+b.ID] {
			in, tone = "pbs.in_night", "blue"
		}
		check := "pbs.check_none"
		if b.Verification != nil {
			check = "pbs.check_failed"
			if b.Verification.State == "ok" {
				check = "pbs.check_ok"
			}
		}
		shares[i] = snapshot.Share{Label: rows[i].Title, Value: 1, Tone: tone, Rows: []snapshot.Row{
			{Title: snapshot.Text{Key: in}, Value: snapshot.Msg("pbs.when", map[string]any{"at": at(b)})},
			{Title: snapshot.Text{Key: "pbs.size"}, Value: snapshot.Msg("pbs.bytes", map[string]any{"n": b.Size})},
			{Title: snapshot.Text{Key: "pbs.check"}, Value: snapshot.Msg(check, nil)},
		}}
	}
	p.Figures = []snapshot.Figure{{
		Area: Area, Icon: "archive", Label: snapshot.Text{Key: "pbs.backups"}, Value: float64(guests(last)),
		Unit:   snapshot.Msg("pbs.of_guests", map[string]any{"n": len(newest)}),
		Detail: snapshot.Msg("pbs.when", map[string]any{"at": at(last[len(last)-1])}),
		Shares: shares,
	}}
	return p
}

// guests is how many guests a run backed up.
func guests(run []backup) int {
	seen := map[string]bool{}
	for _, b := range run {
		seen[b.Type+"/"+b.ID] = true
	}
	return len(seen)
}

func at(b backup) time.Time { return time.Unix(b.Time, 0).UTC() }

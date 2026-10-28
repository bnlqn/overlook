// Package pbs reads Proxmox Backup Server: each guest's newest backup in one
// datastore, with its size and whether it passed its check, and the nights
// that made them. Everything it gives goes in Storage.
package pbs

import (
	"cmp"
	"context"
	"errors"
	"fmt"
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

const area = "storage"

// night is the longest gap between two backups of one run. Backups further
// apart are two runs.
// ponytail: a run whose one guest takes longer than this splits in two;
// read the backup jobs' tasks if that's ever seen.
const night = 6 * time.Hour

// Config is one datastore of a Proxmox Backup Server, under sources.pbs in
// overlook.yaml.
type Config struct {
	Name string `yaml:"name"`
	// URL is where Overlook reads PBS, such as https://pbs.example.com:8007.
	URL string `yaml:"url"`
	// Token is an API token's ID, as user@realm!name, and Secret its secret.
	Token  string `yaml:"token"`
	Secret string `yaml:"secret"`
	// Datastore is the datastore whose backups are read.
	Datastore string `yaml:"datastore"`
	// CA is a PEM file of the CA that signed PBS's certificate, or of the
	// certificate itself, such as the server's
	// /etc/proxmox-backup/proxy.pem, when the system doesn't trust it.
	CA string `yaml:"ca"`
	// ServerName is the name PBS's certificate is for, when the URL reaches
	// it by another, such as its address.
	ServerName string `yaml:"server_name"`

	client fetch.Client
}

var tokenID = regexp.MustCompile(`^[^@!\s]+@[^@!\s]+![^@!\s]+$`)

// Check fills c's defaults and refuses what can't work.
func (c *Config) Check() error {
	if c.URL == "" {
		return errors.New("url is missing")
	}
	if err := fetch.CheckURL(c.URL); err != nil {
		return fmt.Errorf("url %w", err)
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
	if c.Name == "" {
		c.Name = "Backup Server"
	}
	if c.ServerName != "" && c.CA == "" {
		return errors.New("server_name needs the ca that signed the certificate")
	}
	if c.CA != "" {
		client, err := fetch.Trusting(c.CA, c.ServerName)
		if err != nil {
			return fmt.Errorf("ca: %w", err)
		}
		c.client = client
	}
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

// Read reads the backups in c's datastore.
func Read(ctx context.Context, c Config) (snapshot.Parts, error) {
	var answer struct {
		Data []backup `json:"data"`
	}
	header := http.Header{"Authorization": {"PBSAPIToken=" + c.Token + ":" + c.Secret}}
	u := strings.TrimSuffix(c.URL, "/") + "/api2/json/admin/datastore/" + url.PathEscape(c.Datastore) + "/snapshots"
	if err := c.client.JSON(ctx, u, header, &answer); err != nil {
		return snapshot.Parts{}, err
	}
	return parts(answer.Data, time.Now()), nil
}

func parts(backups []backup, now time.Time) snapshot.Parts {
	var p snapshot.Parts
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
		rows[i] = snapshot.Row{Title: title, Value: ptr(key("pbs.when", args)), Detail: ptr(key("pbs.unverified", args))}
		switch {
		case b.Verification == nil:
		case b.Verification.State == "ok":
			rows[i].Detail = ptr(key("pbs.verified", args))
		default:
			rows[i].Value, rows[i].Tone, rows[i].Detail = ptr(key("pbs.failed", nil)), "red", ptr(key("pbs.failed_detail", args))
		}
	}
	p.Sections = []snapshot.Section{{Area: area, Title: key("pbs.backups", nil), Rows: rows}}

	// The nights: the backups made one after another, newest first.
	var runs [][]backup
	for i, b := range backups {
		if i == 0 || at(backups[i-1]).Sub(at(b)) > night {
			runs = append(runs, nil)
		}
		runs[len(runs)-1] = append(runs[len(runs)-1], b)
	}
	for _, r := range runs {
		start := at(r[len(r)-1])
		if now.Sub(start) > 7*24*time.Hour {
			break
		}
		p.Events = append(p.Events, snapshot.Event{
			At: start, Area: area, Icon: "archive",
			Text: key("pbs.backed_up", map[string]any{"n": guests(r)}),
		})
	}

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
	// Under it, each guest, green where the night backed it up.
	tonight := map[string]bool{}
	for _, b := range last {
		tonight[b.Type+"/"+b.ID] = true
	}
	shares := make([]snapshot.Share, len(newest))
	for i, b := range newest {
		shares[i] = snapshot.Share{Label: rows[i].Title, Value: 1, Tone: "gray", Detail: ptr(key("pbs.when", map[string]any{"at": at(b)}))}
		if tonight[b.Type+"/"+b.ID] {
			shares[i].Tone = "green"
		}
	}
	p.Figures = []snapshot.Figure{{
		Area: area, Icon: "archive", Label: key("pbs.backups", nil), Value: float64(guests(last)),
		Unit:   ptr(key("pbs.of_guests", map[string]any{"n": len(newest)})),
		Detail: ptr(key("pbs.when", map[string]any{"at": at(last[len(last)-1])})),
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

func key(k string, args map[string]any) snapshot.Text { return snapshot.Text{Key: k, Args: args} }

func ptr[T any](v T) *T { return &v }

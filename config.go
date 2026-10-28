package main

import (
	"bytes"
	"fmt"
	"os"
	"regexp"
	"slices"

	"go.yaml.in/yaml/v3"
)

// Config is overlook.yaml: the sources to read, and the apps and tools to
// link to. A key it doesn't know is an error, so a typo never goes unnoticed.
type Config struct {
	Sources struct {
		Gatus []GatusConfig `yaml:"gatus"`
	} `yaml:"sources"`
	Apps  []Link `yaml:"apps"`
	Tools []Link `yaml:"tools"`
}

// GatusConfig is one check engine.
type GatusConfig struct {
	Name string `yaml:"name"`
	URL  string `yaml:"url"`
	// Where the page links to, when Overlook reads Gatus at another address.
	Link string `yaml:"link"`
	// Failures in a row that make a problem: the same as Gatus's alerts.
	Threshold int `yaml:"threshold"`
	// Gatus groups not named like an area, and the area each goes to.
	Areas map[string]string `yaml:"areas"`
}

// Link is an app or a tool on the page.
type Link struct {
	Name        string `yaml:"name" json:"name"`
	URL         string `yaml:"url" json:"url"`
	Icon        string `yaml:"icon" json:"icon,omitempty"`
	Check       string `yaml:"check" json:"check,omitempty"`
	Description string `yaml:"description" json:"description,omitempty"`
	Color       string `yaml:"color" json:"color,omitempty"`
}

var Areas = []string{"servers", "network", "storage", "apps"}

var envVar = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

func loadConfig(path string) (Config, error) {
	var c Config
	raw, err := os.ReadFile(path)
	if err != nil {
		return c, err
	}
	// ${VAR} is replaced in values only, after parsing, so a value can't
	// change the file's structure and a comment can't name a var.
	var doc yaml.Node
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	if err := interpolate(&doc); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	if len(doc.Content) == 0 {
		return c, fmt.Errorf("%s: empty", path)
	}
	out, err := yaml.Marshal(&doc)
	if err != nil {
		return c, err
	}
	dec := yaml.NewDecoder(bytes.NewReader(out))
	dec.KnownFields(true)
	if err := dec.Decode(&c); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	if err := c.check(); err != nil {
		return c, fmt.Errorf("%s: %w", path, err)
	}
	return c, nil
}

func interpolate(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		var unset error
		n.Value = envVar.ReplaceAllStringFunc(n.Value, func(m string) string {
			name := envVar.FindStringSubmatch(m)[1]
			v, ok := os.LookupEnv(name)
			if !ok && unset == nil {
				unset = fmt.Errorf("%s is not set", name)
			}
			return v
		})
		return unset
	}
	for _, c := range n.Content {
		if err := interpolate(c); err != nil {
			return err
		}
	}
	return nil
}

// check fills the defaults and refuses what can't work.
func (c *Config) check() error {
	for i := range c.Sources.Gatus {
		g := &c.Sources.Gatus[i]
		if g.URL == "" {
			return fmt.Errorf("sources.gatus[%d]: url is missing", i)
		}
		if g.Name == "" {
			g.Name = "Gatus"
		}
		if g.Link == "" {
			g.Link = g.URL
		}
		if g.Threshold == 0 {
			g.Threshold = 3
		}
		if g.Threshold < 1 {
			return fmt.Errorf("sources.gatus[%d]: threshold must be 1 or more", i)
		}
		for group, area := range g.Areas {
			if !slices.Contains(Areas, area) {
				return fmt.Errorf("sources.gatus[%d].areas: %s goes to %q, not one of %v", i, group, area, Areas)
			}
		}
	}
	for _, l := range slices.Concat(c.Apps, c.Tools) {
		if l.Name == "" || l.URL == "" {
			return fmt.Errorf("every app and tool needs a name and a url")
		}
	}
	return nil
}

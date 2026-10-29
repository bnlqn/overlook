package servarr

import (
	"strings"
	"testing"
)

func TestConfigCheck(t *testing.T) {
	t.Parallel()
	c := Config{URL: "http://radarr:7878", Key: "0123abcd"}
	if err := c.Check("Radarr"); err != nil || c.Name != "Radarr" {
		t.Errorf("defaults: %+v, %v", c, err)
	}
	for _, bad := range []struct {
		c    Config
		want string
	}{
		{Config{Key: "0123abcd"}, "url is missing"},
		{Config{URL: "radarr:7878", Key: "0123abcd"}, "url needs"},
		{Config{URL: "http://radarr:7878"}, "key is missing"},
	} {
		if err := bad.c.Check("Radarr"); err == nil || !strings.Contains(err.Error(), bad.want) {
			t.Errorf("%+v: want an error with %q, got %v", bad.c, bad.want, err)
		}
	}
}

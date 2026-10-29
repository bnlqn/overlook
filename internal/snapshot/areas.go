package snapshot

import (
	"fmt"
	"slices"
	"strings"
)

// AreaOf is the area a check engine's group of checks goes in: the one the
// config names for it in areas, or the area it's named like, such as
// Servers or network, or none.
func AreaOf(group string, areas map[string]string) string {
	if a, ok := areas[group]; ok {
		return a
	}
	if a := strings.ToLower(group); slices.Contains(Areas, a) {
		return a
	}
	return ""
}

// CheckAreas refuses a config's areas that name an area that isn't one.
func CheckAreas(areas map[string]string) error {
	for group, area := range areas {
		if !slices.Contains(Areas, area) {
			return fmt.Errorf("areas: %s goes to %q, not one of %s", group, area, strings.Join(Areas, ", "))
		}
	}
	return nil
}

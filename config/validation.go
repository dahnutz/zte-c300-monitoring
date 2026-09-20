package config

import (
	"fmt"
	"strconv"
	"strings"
)

// Reject explicit topology mistakes before the legacy parser can fall back to
// another slot. Empty input preserves the documented legacy slots 1 and 2.
func validateBoardSpecs(value string) error {
	if strings.TrimSpace(value) == "" {
		return nil
	}
	seen := map[int]bool{}
	for _, spec := range strings.Split(value, ",") {
		parts := strings.Split(strings.TrimSpace(spec), ":")
		slot, err := strconv.Atoi(strings.TrimSpace(parts[0]))
		if err != nil || slot < 1 || slot > MaxBoardID || len(parts) > 2 || seen[slot] {
			return fmt.Errorf("invalid or duplicate board specification %q", spec)
		}
		seen[slot] = true
		if len(parts) == 2 {
			pons, err := strconv.Atoi(strings.TrimSpace(parts[1]))
			if err != nil || pons < 1 || pons > MaxPonID {
				return fmt.Errorf("invalid PON count in %q", spec)
			}
		}
	}
	return nil
}

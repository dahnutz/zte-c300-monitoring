package config

import "testing"

func TestExplicitTopologyValidation(t *testing.T) {
	for _, value := range []string{"31:16", "0:8", "3:0", "3:17", "3:8,3:16", "3:x", "3:8:1", "3:8,"} {
		if err := validateBoardSpecs(value); err == nil {
			t.Errorf("accepted invalid topology %q", value)
		}
	}
	for _, value := range []string{"", "3:16,5:8", "3,5"} {
		if err := validateBoardSpecs(value); err != nil {
			t.Error(err)
		}
	}
}
func TestRegistryPortDoesNotWrap(t *testing.T) {
	for _, port := range []int{-1, 65536, 65537} {
		if _, err := oltFromJSON(oltJSON{ID: "test", Host: "127.0.0.1", Community: "fixture-only", Port: port}, 1); err == nil {
			t.Errorf("accepted port %d", port)
		}
	}
}

package store

import (
	"strconv"
	"strings"
)

// ParseOpticalPower converts the collector's formatted optical string (for
// example "-20.00" or "3.16") into a nullable float. Blank or unparsable
// values are unavailable, never zero.
func ParseOpticalPower(raw string) *float64 {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil
	}
	return &value
}

// ParseRxPower is the RX name for ParseOpticalPower.
func ParseRxPower(raw string) *float64 {
	return ParseOpticalPower(raw)
}

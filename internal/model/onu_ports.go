package model

import (
	"strings"
	"unicode"
)

// ExpectedEthernetPorts is the UNI count for a known ZTE ONU type.
// 0 means the model is unknown: do not invent ports.
func ExpectedEthernetPorts(onuType string) int {
	t := normalizeOnuType(onuType)
	switch {
	case strings.Contains(t, "F668"), strings.Contains(t, "F670"),
		strings.Contains(t, "F660"), strings.Contains(t, "F680"),
		strings.Contains(t, "F643"), strings.Contains(t, "F6600"):
		return 4
	case strings.Contains(t, "F612"), strings.Contains(t, "F4005"):
		return 2
	case strings.Contains(t, "F601"), strings.Contains(t, "F401"),
		strings.Contains(t, "F400"):
		return 1
	default:
		return 0
	}
}

// PadEthernetPorts fills missing UNI indexes 1..N for a known model.
// Extra observed indexes above N are kept. Unknown models are unchanged.
func PadEthernetPorts(onuType string, ports []ONUEthernetPort) []ONUEthernetPort {
	n := ExpectedEthernetPorts(onuType)
	if n <= 0 {
		return ports
	}
	by := make(map[int]ONUEthernetPort, len(ports))
	for _, port := range ports {
		by[port.PortIndex] = port
	}
	out := make([]ONUEthernetPort, 0, n+len(ports))
	for i := 1; i <= n; i++ {
		if port, ok := by[i]; ok {
			out = append(out, port)
			continue
		}
		out = append(out, ONUEthernetPort{
			PortIndex:  i,
			AdminState: "unknown",
			LinkState:  "unknown",
			Duplex:     "unknown",
		})
	}
	for _, port := range ports {
		if port.PortIndex > n {
			out = append(out, port)
		}
	}
	return out
}

func normalizeOnuType(onuType string) string {
	var b strings.Builder
	for _, r := range strings.ToUpper(strings.TrimSpace(onuType)) {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			b.WriteRune(r)
		}
	}
	return b.String()
}

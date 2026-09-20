package usecase

import (
	"context"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	"go.uber.org/zap"
	"zte-c300-monitoring/config"
	apperrors "zte-c300-monitoring/internal/errors"
	"zte-c300-monitoring/internal/model"
	"zte-c300-monitoring/pkg/logger"
	"zte-c300-monitoring/pkg/metrics"
)

// uplinkNameRegex parses an uplink ifName of the form "<prefix>_<shelf>/<slot>/<port>"
// (e.g. "xgei_1/19/1" or "gei_1/3/2"). The prefix has already been stripped by
// the caller before matching, so this only matches the trailing "shelf/slot/port".
var uplinkNameRegex = regexp.MustCompile(`^(\d+)/(\d+)/(\d+)`)

// smartGroupNameRegex matches ZTE LAG ifNames (SmartGroup1, smartgroup3, …).
var smartGroupNameRegex = regexp.MustCompile(`(?i)^smartgroup(\d+)$`)

const uplinkKindLAG = "lag"

// entPhysicalClassModule is the ENTITY-MIB entPhysicalClass value for a
// module/card. Only rows with this class are treated as cards.
const entPhysicalClassModule = 9

// isCardEntity accepts standard modules and a narrowly identified firmware quirk.
// Some firmware reports line/control cards as chassis(3). Requiring "card"
// in the description keeps real chassis rows out. Power cards use class 6.
func isCardEntity(class int, description string) bool {
	if class == entPhysicalClassModule {
		return true
	}
	return (class == 3 || class == 6) && strings.Contains(strings.ToLower(description), "card")
}

// toInt extracts an int from a gosnmp PDU value, tolerating the several integer
// representations gosnmp may return (int, uint, int64, uint64, etc.). Returns
// 0 if the value is not an integer kind.
func toInt(v interface{}) int {
	switch n := v.(type) {
	case int:
		return n
	case int8:
		return int(n)
	case int16:
		return int(n)
	case int32:
		return int(n)
	case int64:
		return int(n)
	case uint:
		return int(n)
	case uint8:
		return int(n)
	case uint16:
		return int(n)
	case uint32:
		return int(n)
	case uint64:
		return int(n)
	default:
		return 0
	}
}

// pduString safely extracts a string from a gosnmp OctetString PDU value.
func pduString(v interface{}) string {
	if b, ok := v.([]byte); ok {
		return string(b)
	}
	if s, ok := v.(string); ok {
		return s
	}
	return ""
}

// trailingIndex returns the trailing integer of an OID string (the last
// dot-separated element). Returns -1 if the last element is not an integer.
func trailingIndex(oid string) int {
	oid = strings.TrimSuffix(oid, ".")
	idx := strings.LastIndex(oid, ".")
	last := oid
	if idx >= 0 {
		last = oid[idx+1:]
	}
	n, err := strconv.Atoi(last)
	if err != nil {
		return -1
	}
	return n
}

// trailingIndices returns the last n dot-separated integers of an OID.
func trailingIndices(oid string, n int) []int {
	if n < 1 {
		return nil
	}
	parts := strings.Split(strings.Trim(oid, "."), ".")
	if len(parts) < n {
		return nil
	}
	out := make([]int, n)
	for i := 0; i < n; i++ {
		v, err := strconv.Atoi(parts[len(parts)-n+i])
		if err != nil {
			return nil
		}
		out[i] = v
	}
	return out
}

type portRow struct {
	name    string
	kind    string
	lag     string
	members []string
}

func classifyUplinkName(name string) (kind string, ok bool) {
	switch {
	case strings.HasPrefix(name, "xgei_"):
		return "10G", true
	case strings.HasPrefix(name, "gei_"):
		return "1G", true
	}
	if smartGroupNameRegex.MatchString(name) {
		return uplinkKindLAG, true
	}
	return "", false
}

func parseUplinkLocation(name, kind string) (shelf, slot, port int) {
	if kind == uplinkKindLAG {
		if m := smartGroupNameRegex.FindStringSubmatch(name); m != nil {
			port, _ = strconv.Atoi(m[1])
		}
		return
	}
	if u := strings.IndexByte(name, '_'); u >= 0 {
		if m := uplinkNameRegex.FindStringSubmatch(name[u+1:]); m != nil {
			shelf, _ = strconv.Atoi(m[1])
			slot, _ = strconv.Atoi(m[2])
			port, _ = strconv.Atoi(m[3])
		}
	}
	return
}

func appendUnique(dst []string, name string) []string {
	for _, existing := range dst {
		if existing == name {
			return dst
		}
	}
	return append(dst, name)
}

func attachLagMember(ports map[int]*portRow, higher, lower int) {
	h, hOK := ports[higher]
	l, lOK := ports[lower]
	if !hOK || !lOK {
		return
	}
	switch {
	case h.kind == uplinkKindLAG && l.kind != uplinkKindLAG:
		h.members = appendUnique(h.members, l.name)
		l.lag = h.name
	case l.kind == uplinkKindLAG && h.kind != uplinkKindLAG:
		l.members = appendUnique(l.members, h.name)
		h.lag = l.name
	}
}

func uplinkPortLess(a, b model.UplinkPort) bool {
	aLAG, bLAG := a.Kind == uplinkKindLAG, b.Kind == uplinkKindLAG
	if aLAG != bLAG {
		return !aLAG
	}
	if aLAG {
		return a.Port < b.Port
	}
	if a.Slot != b.Slot {
		return a.Slot < b.Slot
	}
	return a.Port < b.Port
}

// statusString maps an IF-MIB admin/oper status integer to a human label.
func statusString(v int) string {
	switch v {
	case 1:
		return "up"
	case 2:
		return "down"
	default:
		return strconv.Itoa(v)
	}
}

// cardRole classifies a card from its entPhysicalDescr (case-insensitive
// contains matching).
func cardRole(descr string) string {
	d := strings.ToLower(descr)
	switch {
	case strings.Contains(d, "gpon"):
		return "gpon"
	case strings.Contains(d, "control"):
		return "control"
	case strings.Contains(d, "ethernet interface"):
		return "uplink"
	case strings.Contains(d, "power"):
		return "power"
	default:
		return "other"
	}
}

// GetUplinkTopology auto-detects the OLT's cards and uplink ethernet ports by
// walking standard IF-MIB + ENTITY-MIB subtrees. It is read-only (Phase 1:
// detection only) and intentionally NOT cached — detection is infrequent and
// must reflect the live hardware. Singleflight still coalesces concurrent calls.
func (u *onuUsecase) GetUplinkTopology(ctx context.Context) (topo *model.UplinkTopology, err error) {
	defer metrics.RecordSNMPOperation("walk", time.Now(), &err)

	result, sgErr, _ := u.sg.Do(u.cacheKey("uplink_topology"), func() (interface{}, error) {
		logger.WithRequestID(ctx).Info("fetching_uplink_topology_snmp_walk")

		ports := make(map[int]*portRow)
		walkErr := u.snmpRepository.BulkWalk(config.OidIfName, func(pdu gosnmp.SnmpPDU) error {
			idx := trailingIndex(pdu.Name)
			if idx < 0 {
				return nil
			}
			name := pduString(pdu.Value)
			kind, ok := classifyUplinkName(name)
			if !ok {
				return nil
			}
			ports[idx] = &portRow{name: name, kind: kind}
			return nil
		})
		if walkErr != nil {
			return nil, apperrors.NewSNMPError("walk", walkErr)
		}

		admin := make(map[int]int)
		oper := make(map[int]int)
		speed := make(map[int]int)

		if walkErr = u.snmpRepository.BulkWalk(config.OidIfAdminStatus, func(pdu gosnmp.SnmpPDU) error {
			if idx := trailingIndex(pdu.Name); idx >= 0 {
				if _, ok := ports[idx]; ok {
					admin[idx] = toInt(pdu.Value)
				}
			}
			return nil
		}); walkErr != nil {
			return nil, apperrors.NewSNMPError("walk", walkErr)
		}

		if walkErr = u.snmpRepository.BulkWalk(config.OidIfOperStatus, func(pdu gosnmp.SnmpPDU) error {
			if idx := trailingIndex(pdu.Name); idx >= 0 {
				if _, ok := ports[idx]; ok {
					oper[idx] = toInt(pdu.Value)
				}
			}
			return nil
		}); walkErr != nil {
			return nil, apperrors.NewSNMPError("walk", walkErr)
		}

		if walkErr = u.snmpRepository.BulkWalk(config.OidIfHighSpeed, func(pdu gosnmp.SnmpPDU) error {
			if idx := trailingIndex(pdu.Name); idx >= 0 {
				if _, ok := ports[idx]; ok {
					speed[idx] = toInt(pdu.Value)
				}
			}
			return nil
		}); walkErr != nil {
			return nil, apperrors.NewSNMPError("walk", walkErr)
		}

		if stackErr := u.snmpRepository.BulkWalk(config.OidIfStackStatus, func(pdu gosnmp.SnmpPDU) error {
			pair := trailingIndices(pdu.Name, 2)
			if len(pair) != 2 || pair[0] == 0 || pair[1] == 0 {
				return nil
			}
			attachLagMember(ports, pair[0], pair[1])
			return nil
		}); stackErr != nil {
			logger.WithRequestID(ctx).Warn("ifstack_unavailable", zap.Error(stackErr))
		}

		classByIdx := make(map[int]int)
		if walkErr = u.snmpRepository.BulkWalk(config.OidEntPhysicalClass, func(pdu gosnmp.SnmpPDU) error {
			if idx := trailingIndex(pdu.Name); idx >= 0 {
				classByIdx[idx] = toInt(pdu.Value)
			}
			return nil
		}); walkErr != nil {
			return nil, apperrors.NewSNMPError("walk", walkErr)
		}

		descrByIdx := make(map[int]string)
		if walkErr = u.snmpRepository.BulkWalk(config.OidEntPhysicalDescr, func(pdu gosnmp.SnmpPDU) error {
			if idx := trailingIndex(pdu.Name); idx >= 0 {
				descrByIdx[idx] = pduString(pdu.Value)
			}
			return nil
		}); walkErr != nil {
			return nil, apperrors.NewSNMPError("walk", walkErr)
		}

		positions := make(map[int]int)
		if posErr := u.snmpRepository.BulkWalk(config.OidEntPhysicalParentRelPos, func(pdu gosnmp.SnmpPDU) error {
			if idx := trailingIndex(pdu.Name); idx >= 0 && pdu.Type == gosnmp.Integer {
				positions[idx] = toInt(pdu.Value)
			}
			return nil
		}); posErr != nil {
			logger.WithRequestID(ctx).Warn("card_position_unavailable", zap.Error(posErr))
		}

		out := &model.UplinkTopology{
			Cards: make([]model.UplinkCard, 0),
			Ports: make([]model.UplinkPort, 0, len(ports)),
		}
		for idx, p := range ports {
			shelf, slot, portID := parseUplinkLocation(p.name, p.kind)
			members := append([]string(nil), p.members...)
			sort.Strings(members)
			out.Ports = append(out.Ports, model.UplinkPort{
				Name:        p.name,
				Kind:        p.kind,
				Shelf:       shelf,
				Slot:        slot,
				Port:        portID,
				AdminStatus: statusString(admin[idx]),
				OperStatus:  statusString(oper[idx]),
				SpeedMbps:   speed[idx],
				Lag:         p.lag,
				Members:     members,
			})
		}

		for idx, class := range classByIdx {
			if !isCardEntity(class, descrByIdx[idx]) {
				continue
			}
			descr := descrByIdx[idx]
			slot, source := idx/10-1, "index_heuristic"
			if pos, ok := positions[idx]; ok && pos >= 0 {
				slot, source = pos, "parent_relative_position"
			}
			out.Cards = append(out.Cards, model.UplinkCard{
				EntIndex:   idx,
				Slot:       slot,
				SlotSource: source,
				Type:       descr,
				Role:       cardRole(descr),
			})
		}

		sort.Slice(out.Ports, func(i, j int) bool {
			return uplinkPortLess(out.Ports[i], out.Ports[j])
		})
		sort.Slice(out.Cards, func(i, j int) bool {
			return out.Cards[i].Slot < out.Cards[j].Slot
		})

		logger.WithRequestID(ctx).Info("uplink_topology_detected",
			zap.Int("card_count", len(out.Cards)),
			zap.Int("uplink_port_count", len(out.Ports)),
		)
		return out, nil
	})

	if sgErr != nil {
		err = sgErr
		logger.WithRequestID(ctx).Error("get_uplink_topology_failed", zap.Error(err))
		return nil, err
	}

	return result.(*model.UplinkTopology), nil
}

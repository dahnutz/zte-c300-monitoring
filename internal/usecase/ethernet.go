package usecase

import (
	"context"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	"zte-c300-monitoring/config"
	"zte-c300-monitoring/internal/model"
)

func ponIfIndex(board, pon int) int {
	return config.OnuIDIfIndexBase + board*config.OnuIDSlotStride + pon*config.OnuIDIncrement
}

func emptyEthernet(onuState string) model.ONUEthernetStatus {
	out := model.ONUEthernetStatus{
		Status:     "unavailable",
		ObservedAt: time.Now().UTC().Format(time.RFC3339),
		Ports:      []model.ONUEthernetPort{},
	}
	if onuState != "Online" {
		out.Status = "onu_not_online"
		if onuState == "Unknown" || onuState == "" {
			out.Status = "onu_state_unknown"
		}
	}
	return out
}

func newEthernetPort(port int) *model.ONUEthernetPort {
	return &model.ONUEthernetPort{
		PortIndex:  port,
		AdminState: "unknown",
		LinkState:  "unknown",
		Duplex:     "unknown",
	}
}

func applyEthernetColumn(row *model.ONUEthernetPort, column, value int) {
	switch column {
	case 0:
		row.AdminRaw = &value
	case 1:
		row.OperRaw = &value
	case 2:
		row.SpeedRaw = &value
	}
}

func decodeEthernetPorts(rows map[int]*model.ONUEthernetPort) ([]model.ONUEthernetPort, bool) {
	failed := false
	out := make([]model.ONUEthernetPort, 0, len(rows))
	for _, row := range rows {
		if row.AdminRaw != nil {
			switch *row.AdminRaw {
			case 1:
				row.AdminState = "enabled"
			case 2:
				row.AdminState = "disabled"
			}
		}
		if row.OperRaw != nil {
			switch *row.OperRaw {
			case 1:
				row.LinkState = "up"
			case 2:
				row.LinkState = "down"
			}
		}
		if row.LinkState == "up" && row.SpeedRaw != nil {
			speed, duplex := ethernetSpeed(*row.SpeedRaw)
			if speed > 0 {
				row.SpeedMbps = &speed
				row.Duplex = duplex
			}
		}
		// Auto(1), 65535 and unknown values do not establish a negotiated rate.
		if row.AdminState == "unknown" || row.LinkState == "unknown" || (row.LinkState == "up" && row.SpeedMbps == nil) {
			failed = true
		}
		out = append(out, *row)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].PortIndex < out[j].PortIndex })
	return out, failed
}

func finishEthernet(out model.ONUEthernetStatus, ports []model.ONUEthernetPort, failed bool) model.ONUEthernetStatus {
	out.Ports = ports
	if len(ports) > 0 {
		out.Status = "ok"
		if failed {
			out.Status = "partial"
		}
	}
	return out
}

func padEthernet(onuType string, eth model.ONUEthernetStatus) model.ONUEthernetStatus {
	eth.Ports = model.PadEthernetPorts(onuType, eth.Ports)
	return eth
}

func parseEthernetSuffix(name, root string) []int {
	trimmed := strings.TrimPrefix(name, ".")
	expected := strings.TrimPrefix(root, ".") + "."
	if !strings.HasPrefix(trimmed, expected) {
		return nil
	}
	rest := strings.TrimPrefix(trimmed, expected)
	if rest == "" {
		return nil
	}
	parts := strings.Split(rest, ".")
	out := make([]int, 0, len(parts))
	for _, part := range parts {
		n, err := strconv.Atoi(part)
		if err != nil {
			return nil
		}
		out = append(out, n)
	}
	return out
}

func pduInt(pdu gosnmp.SnmpPDU) (int, bool) {
	if pdu.Type == gosnmp.NoSuchObject || pdu.Type == gosnmp.NoSuchInstance || pdu.Type == gosnmp.EndOfMibView {
		return 0, false
	}
	if pdu.Type != gosnmp.Integer {
		return 0, false
	}
	value, ok := pdu.Value.(int)
	return value, ok
}

func ethColumnPrefixes() []string {
	return []string{config.OnuEthAdminPrefix, config.OnuEthOperPrefix, config.OnuEthSpeedPrefix}
}

// getEthernetPorts only walks three columns underneath ONE ONU. It does not
// enumerate other subscribers or treat stale offline-ONU UNI data as live.
func (u *onuUsecase) getEthernetPorts(ctx context.Context, board, pon, onu int, onuState, onuType string) model.ONUEthernetStatus {
	out := emptyEthernet(onuState)
	if onuState != "Online" {
		return out
	}
	index := ponIfIndex(board, pon)
	rows := map[int]*model.ONUEthernetPort{}
	failed := false
	for column, prefix := range ethColumnPrefixes() {
		if ctx.Err() != nil {
			failed = true
			break
		}
		root := fmt.Sprintf("%s%s.%d.%d", u.cfg.OltCfg.BaseOID1, prefix, index, onu)
		err := u.snmpRepository.BulkWalk(root, func(pdu gosnmp.SnmpPDU) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			suffix := parseEthernetSuffix(pdu.Name, root)
			if suffix == nil || len(suffix) != 1 || suffix[0] < 1 {
				if suffix != nil && (len(suffix) != 1 || suffix[0] < 1) {
					failed = true
				}
				return nil
			}
			value, ok := pduInt(pdu)
			if !ok {
				if pdu.Type != gosnmp.NoSuchObject && pdu.Type != gosnmp.NoSuchInstance && pdu.Type != gosnmp.EndOfMibView {
					failed = true
				}
				return nil
			}
			port := suffix[0]
			row := rows[port]
			if row == nil {
				row = newEthernetPort(port)
				rows[port] = row
			}
			applyEthernetColumn(row, column, value)
			return nil
		})
		if err != nil {
			failed = true
		}
	}
	ports, decodeFailed := decodeEthernetPorts(rows)
	return padEthernet(onuType, finishEthernet(out, ports, failed || decodeFailed))
}

type ponEthONU struct {
	rows   map[int]*model.ONUEthernetPort
	failed bool
}

func (u *onuUsecase) walkPONEthernet(ctx context.Context, board, pon int) (map[int]*ponEthONU, bool) {
	index := ponIfIndex(board, pon)
	byONU := map[int]*ponEthONU{}
	walkFailed := false
	get := func(onu int) *ponEthONU {
		row := byONU[onu]
		if row == nil {
			row = &ponEthONU{rows: map[int]*model.ONUEthernetPort{}}
			byONU[onu] = row
		}
		return row
	}
	for column, prefix := range ethColumnPrefixes() {
		if ctx.Err() != nil {
			walkFailed = true
			break
		}
		root := fmt.Sprintf("%s%s.%d", u.cfg.OltCfg.BaseOID1, prefix, index)
		err := u.snmpRepository.BulkWalk(root, func(pdu gosnmp.SnmpPDU) error {
			if err := ctx.Err(); err != nil {
				return err
			}
			suffix := parseEthernetSuffix(pdu.Name, root)
			if suffix == nil || len(suffix) != 2 || suffix[0] < 1 || suffix[1] < 1 {
				return nil
			}
			value, ok := pduInt(pdu)
			if !ok {
				if pdu.Type != gosnmp.NoSuchObject && pdu.Type != gosnmp.NoSuchInstance && pdu.Type != gosnmp.EndOfMibView {
					get(suffix[0]).failed = true
				}
				return nil
			}
			bucket := get(suffix[0])
			port := suffix[1]
			row := bucket.rows[port]
			if row == nil {
				row = newEthernetPort(port)
				bucket.rows[port] = row
			}
			applyEthernetColumn(row, column, value)
			return nil
		})
		if err != nil {
			walkFailed = true
		}
	}
	return byONU, walkFailed
}

func ethernetFromPONWalk(onuState string, collected *ponEthONU, walkFailed bool) model.ONUEthernetStatus {
	out := emptyEthernet(onuState)
	if onuState != "Online" {
		return out
	}
	if collected == nil {
		return out
	}
	ports, decodeFailed := decodeEthernetPorts(collected.rows)
	return finishEthernet(out, ports, walkFailed || collected.failed || decodeFailed)
}

// attachPONEthernet fills Ethernet on a PON list using three column walks under
// the PON index (not three walks per ONU). Offline ONUs keep empty UNI lists.
func (u *onuUsecase) attachPONEthernet(ctx context.Context, board, pon int, onus []model.ONUInfoPerBoard) {
	if len(onus) == 0 {
		return
	}
	anyOnline := false
	for i := range onus {
		if onus[i].Status == "Online" {
			anyOnline = true
			break
		}
	}
	if !anyOnline {
		for i := range onus {
			onus[i].Ethernet = emptyEthernet(onus[i].Status)
		}
		return
	}
	byONU, walkFailed := u.walkPONEthernet(ctx, board, pon)
	for i := range onus {
		onus[i].Ethernet = padEthernet(onus[i].OnuType, ethernetFromPONWalk(onus[i].Status, byONU[onus[i].ID], walkFailed))
	}
}

func ethernetSpeed(raw int) (int, string) {
	switch raw {
	case 2:
		return 10, "half"
	case 3:
		return 10, "full"
	case 4:
		return 100, "half"
	case 5:
		return 100, "full"
	case 6:
		return 1000, "full"
	case 7:
		return 10000, "full"
	default:
		return 0, "unknown"
	}
}

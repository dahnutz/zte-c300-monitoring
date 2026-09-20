package usecase

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/gosnmp/gosnmp"
	"zte-c300-monitoring/config"
	"zte-c300-monitoring/internal/model"
	"zte-c300-monitoring/internal/utils"
)

func (u *onuUsecase) DiscoverUnconfiguredONUs(ctx context.Context) (model.UnauthDiscovery, error) {
	out := model.UnauthDiscovery{
		Status:     "unsupported",
		OID:        config.OnuUncfgSerialOID,
		ObservedAt: time.Now().UTC(),
		ONUs:       []model.UnauthONU{},
	}
	if ctx.Err() != nil {
		out.Status = "unavailable"
		out.Message = ctx.Err().Error()
		return out, nil
	}
	type row struct {
		onu    model.UnauthONU
		suffix string
	}
	bySerial := map[string]row{}
	walked := false
	err := u.snmpRepository.BulkWalk(config.OnuUncfgSerialOID, func(pdu gosnmp.SnmpPDU) error {
		if err := ctx.Err(); err != nil {
			return err
		}
		if pdu.Type == gosnmp.NoSuchObject || pdu.Type == gosnmp.NoSuchInstance || pdu.Type == gosnmp.EndOfMibView {
			return nil
		}
		walked = true
		serial := strings.TrimSpace(utils.ExtractName(pdu.Value))
		if serial == "" {
			return nil
		}
		suffix := parseEthernetSuffix(pdu.Name, config.OnuUncfgSerialOID)
		onu := model.UnauthONU{Serial: serial}
		if len(suffix) > 0 {
			if b, p, ok := decodeOnuIDIfIndex(suffix[0]); ok {
				onu.Board, onu.PON = b, p
			}
		}
		bySerial[serial] = row{onu: onu, suffix: joinSuffix(suffix)}
		return nil
	})
	if err != nil {
		out.Status = "unsupported"
		out.Message = "unconfigured-ONU table walk failed; firmware may not expose this MIB"
		return out, nil
	}
	if !walked {
		out.Status = "unsupported"
		out.Message = "no unconfigured-ONU table instances; treat as unsupported until CLI-matched"
		return out, nil
	}
	types := map[string]string{}
	_ = u.snmpRepository.BulkWalk(config.OnuUncfgTypeOID, func(pdu gosnmp.SnmpPDU) error {
		suffix := joinSuffix(parseEthernetSuffix(pdu.Name, config.OnuUncfgTypeOID))
		typ := strings.TrimSpace(utils.ExtractName(pdu.Value))
		if suffix != "" && typ != "" {
			types[suffix] = typ
		}
		return nil
	})
	for serial, item := range bySerial {
		if typ := types[item.suffix]; typ != "" {
			item.onu.OnuType = typ
			bySerial[serial] = item
		}
		out.ONUs = append(out.ONUs, item.onu)
	}
	if len(out.ONUs) == 0 {
		out.Status = "unsupported"
		out.Message = "table returned no usable serials; an empty result is not confirmed"
		return out, nil
	}
	out.Status = "ok"
	return out, nil
}

func decodeOnuIDIfIndex(idx int) (board, pon int, ok bool) {
	if idx < config.OnuIDIfIndexBase {
		return 0, 0, false
	}
	rest := idx - config.OnuIDIfIndexBase
	board = rest / config.OnuIDSlotStride
	pon = (rest % config.OnuIDSlotStride) / config.OnuIDIncrement
	if board < 1 || board > config.MaxBoardID || pon < 1 || pon > config.MaxPonID {
		return 0, 0, false
	}
	return board, pon, true
}

func joinSuffix(parts []int) string {
	if len(parts) == 0 {
		return ""
	}
	var b strings.Builder
	for i, n := range parts {
		if i > 0 {
			b.WriteByte('.')
		}
		b.WriteString(strconv.Itoa(n))
	}
	return b.String()
}

// snmp-fixture is a synthetic, read-only SNMPv2c agent for deployment tests.
// It models ONE ONU on slot 3 / PON 1. It is not an OLT emulator or MIB authority.
package main

import (
	"log"
	"net"
	"sort"
	"strconv"
	"strings"

	"github.com/gosnmp/gosnmp"
	"zte-c300-monitoring/config"
)

func compareOID(a, b string) int {
	aa, bb := strings.Split(strings.Trim(a, "."), "."), strings.Split(strings.Trim(b, "."), ".")
	for i := 0; i < len(aa) && i < len(bb); i++ {
		x, _ := strconv.Atoi(aa[i])
		y, _ := strconv.Atoi(bb[i])
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	if len(aa) < len(bb) {
		return -1
	}
	if len(aa) > len(bb) {
		return 1
	}
	return 0
}

func fixture() []gosnmp.SnmpPDU {
	rows := []gosnmp.SnmpPDU{}
	add := func(oid string, typ gosnmp.Asn1BER, val interface{}) {
		rows = append(rows, gosnmp.SnmpPDU{Name: oid, Type: typ, Value: val})
	}
	add(".1.3.6.1.2.1.1.3.0", gosnmp.TimeTicks, uint32(123456))
	cfg, _ := config.GenerateBoardPonOID(3, 1)
	for _, row := range []struct{ oid, value string }{
		{cfg.OnuIDNameOID, "TEST-ONU"}, {cfg.OnuDescriptionOID, "Synthetic fixture"},
		{cfg.OnuSerialNumberOID, "ZTEG00000001"},
	} {
		add(config.BaseOID1+row.oid+".1", gosnmp.OctetString, []byte(row.value))
	}
	add(config.BaseOID2+cfg.OnuTypeOID+".1", gosnmp.OctetString, []byte("SYNTHETIC-ONU"))
	add(config.BaseOID1+cfg.OnuRxPowerOID+".1.1", gosnmp.Integer, 5000)
	add(config.BaseOID1+cfg.OnuTxPowerOID+".1.1", gosnmp.Integer, 16000)
	add(config.BaseOID1+config.OnuEthAdminPrefix+".285278977.1.1", gosnmp.Integer, 1)
	add(config.BaseOID1+config.OnuEthOperPrefix+".285278977.1.1", gosnmp.Integer, 1)
	add(config.BaseOID1+config.OnuEthSpeedPrefix+".285278977.1.1", gosnmp.Integer, 6)
	add(config.BaseOID1+cfg.OnuStatusOID+".1", gosnmp.Integer, 4)
	add(config.BaseOID2+cfg.OnuIPAddressOID+".1", gosnmp.IPAddress, "192.0.2.10")
	add(config.BaseOID1+cfg.OnuGponOpticalDistanceOID+".1", gosnmp.Integer, 1000)
	add(config.BaseOID1+cfg.OnuLastOfflineReasonOID+".1", gosnmp.Integer, 2)
	// DateAndTime octets, in the synthetic device's configured timezone (UTC).
	for _, oid := range []string{cfg.OnuLastOnlineOID, cfg.OnuLastOfflineOID} {
		add(config.BaseOID1+oid+".1", gosnmp.OctetString, []byte{7, 234, 9, 1, 12, 0, 0, 0})
	}
	add(config.OidIfName+".100", gosnmp.OctetString, []byte("xgei_1/19/1"))
	add(config.OidIfAdminStatus+".100", gosnmp.Integer, 1)
	add(config.OidIfOperStatus+".100", gosnmp.Integer, 1)
	add(config.OidIfHighSpeed+".100", gosnmp.Gauge32, uint32(10000))
	add(config.OidIfName+".200", gosnmp.OctetString, []byte("SmartGroup1"))
	add(config.OidIfAdminStatus+".200", gosnmp.Integer, 1)
	add(config.OidIfOperStatus+".200", gosnmp.Integer, 1)
	add(config.OidIfHighSpeed+".200", gosnmp.Gauge32, uint32(10000))
	add(config.OidIfStackStatus+".200.100", gosnmp.Integer, 1)
	add(config.OidEntPhysicalClass+".200", gosnmp.Integer, 9)
	add(config.OidEntPhysicalDescr+".200", gosnmp.OctetString, []byte("Synthetic Ethernet interface card"))
	sort.Slice(rows, func(i, j int) bool { return compareOID(rows[i].Name, rows[j].Name) < 0 })
	return rows
}

func respond(req *gosnmp.SnmpPacket, rows []gosnmp.SnmpPDU) *gosnmp.SnmpPacket {
	out := &gosnmp.SnmpPacket{Version: req.Version, Community: req.Community, PDUType: gosnmp.GetResponse, RequestID: req.RequestID}
	if req.PDUType != gosnmp.GetRequest && req.PDUType != gosnmp.GetNextRequest && req.PDUType != gosnmp.GetBulkRequest {
		out.Error = gosnmp.ReadOnly
		out.Variables = req.Variables
		return out
	}
	// The collector issues walks with one root; support multiple roots as well.
	cursors := make([]string, len(req.Variables))
	for i, p := range req.Variables {
		cursors[i] = p.Name
	}
	count := 1
	if req.PDUType == gosnmp.GetBulkRequest {
		count = int(req.MaxRepetitions)
		if count > 50 {
			count = 50
		}
	}
	for repeat := 0; repeat < count; repeat++ {
		for i, name := range cursors {
			next := gosnmp.SnmpPDU{Name: name, Type: gosnmp.EndOfMibView}
			if req.PDUType == gosnmp.GetRequest {
				next.Type = gosnmp.NoSuchInstance
			}
			for _, p := range rows {
				cmp := compareOID(p.Name, name)
				if (req.PDUType == gosnmp.GetRequest && cmp == 0) || (req.PDUType != gosnmp.GetRequest && cmp > 0) {
					next = p
					break
				}
			}
			out.Variables = append(out.Variables, next)
			cursors[i] = next.Name
		}
	}
	return out
}

func main() {
	conn, err := net.ListenPacket("udp", ":1161")
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close()
	log.Print("synthetic SNMP fixture listening on UDP 1161")
	rows := fixture()
	buffer := make([]byte, 65535)
	for {
		n, peer, err := conn.ReadFrom(buffer)
		if err != nil {
			log.Fatal(err)
		}
		req, err := gosnmp.Default.SnmpDecodePacket(buffer[:n])
		if err != nil || req.Version != gosnmp.Version2c || req.Community != "fixture-only" {
			continue
		}
		wire, err := respond(req, rows).MarshalMsg()
		if err != nil {
			log.Print(err)
			continue
		}
		if _, err = conn.WriteTo(wire, peer); err != nil {
			log.Print(err)
		}
	}
}

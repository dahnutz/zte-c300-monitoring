package usecase

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/gosnmp/gosnmp"
	"zte-c300-monitoring/config"
)

func TestDiscoverUnconfiguredUnsupported(t *testing.T) {
	uc := NewOnuUsecase(&mockSnmpRepository{
		BulkWalkFunc: func(string, func(gosnmp.SnmpPDU) error) error {
			return errors.New("no such name")
		},
	}, &mockRedisRepository{}, &config.Config{})
	got, err := uc.DiscoverUnconfiguredONUs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "unsupported" || !strings.Contains(got.Message, "walk failed") {
		t.Fatalf("%+v", got)
	}
}

func TestDiscoverUnconfiguredSerials(t *testing.T) {
	uc := NewOnuUsecase(&mockSnmpRepository{
		BulkWalkFunc: func(oid string, walk func(gosnmp.SnmpPDU) error) error {
			if strings.HasPrefix(oid, config.OnuUncfgSerialOID) {
				return walk(gosnmp.SnmpPDU{
					Name: config.OnuUncfgSerialOID + ".1", Type: gosnmp.OctetString, Value: []byte("ZTEGNEW0001"),
				})
			}
			return walk(gosnmp.SnmpPDU{
				Name: config.OnuUncfgTypeOID + ".1", Type: gosnmp.OctetString, Value: []byte("F601"),
			})
		},
	}, &mockRedisRepository{}, &config.Config{})
	got, err := uc.DiscoverUnconfiguredONUs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if got.Status != "ok" || len(got.ONUs) != 1 || got.ONUs[0].Serial != "ZTEGNEW0001" || got.ONUs[0].OnuType != "F601" {
		t.Fatalf("%+v", got)
	}
}

func TestDiscoverUnconfiguredMissingOrBlankIsNotVerifiedEmpty(t *testing.T) {
	for _, blank := range []bool{false, true} {
		uc := NewOnuUsecase(&mockSnmpRepository{
			BulkWalkFunc: func(oid string, walk func(gosnmp.SnmpPDU) error) error {
				if blank {
					return walk(gosnmp.SnmpPDU{Name: oid + ".1", Type: gosnmp.OctetString, Value: []byte("")})
				}
				return nil
			},
		}, &mockRedisRepository{}, &config.Config{})
		got, err := uc.DiscoverUnconfiguredONUs(context.Background())
		if err != nil || got.Status != "unsupported" {
			t.Fatalf("blank=%v: status=%s err=%v", blank, got.Status, err)
		}
	}
}

# ONU optical and Ethernet telemetry

The individual ONU detail endpoint includes optical TX/RX and the ONU's Ethernet
UNI ports. These are separate from the OLT uplinks returned by `/uplinks`.
Collection is read-only. The PON list (and the history poller that snapshots it)
adds TX as a fifth GET varbind per ONU and walks the three Ethernet UNI columns
once per PON.

## ONU transmit power

TX now uses `zxAnGponRmAniTxOptLevel` from ZTE-AN-GPON-REMOTE-ONU-MIB:

```text
1.3.6.1.4.1.3902.1082.500.20.2.2.2.1.14.<pon_index>.<onu_id>.1
```

The last index selects ANI 1. The PON index uses the same encoding as the existing
ONU RX object, rather than the separate legacy ONU-type index. The previous TX
path used the `.1012` tree and returned `NoSuchObject` on the tested firmware.

The MIB specifies 0.002 dBuW units. Convert to dBm as `raw * 0.002 - 30`, then
format the API value to two decimals. The documented unavailable marker `65535`
is returned as an empty optical value, never the misleading number `101.07`.
This handling applies to both ONU RX and TX.
[Vendor TX object definition](https://mibs.observium.org/object/ZTE-AN-GPON-REMOTE-ONU-MIB/zxAnGponRmAniTxOptLevel/).

`tx_power` is the ONU's upstream transmit power. `rx_power` is the ONU's downstream
receive power. They are not the OLT's transmit/receive readings. Readings can
change between SNMP and CLI samples; keep timestamps when comparing.

## ONU Ethernet ports

A detail response includes this additional object (synthetic example):

```json
{
  "ethernet": {
    "status": "ok",
    "observed_at": "2026-01-01T12:00:00Z",
    "ports": [
      {
        "port_index": 1,
        "admin_state": "enabled",
        "link_state": "up",
        "speed_mbps": 1000,
        "duplex": "full",
        "admin_raw": 1,
        "oper_raw": 1,
        "speed_raw": 6
      }
    ]
  }
}
```

Individual detail walks these columns **under the requested ONU**, merging by
UNI index. PON list/poller collection walks under the requested PON and groups
by ONU and UNI, attaching observations only to known online ONUs. Neither path
assumes one Ethernet port per ONU.

| Meaning | ZTE object | Column OID before `.pon_index.onu_id.port_index` |
|---|---|---|
| Administrative state | `zxAnGponRmEthUniAdminState` | `.1.3.6.1.4.1.3902.1082.500.20.2.3.2.1.5` |
| Operational state | `zxAnGponRmEthUniOperState` | `.1.3.6.1.4.1.3902.1082.500.20.2.3.2.1.6` |
| Reported speed/duplex | `zxAnGponRmEthUniIfSpeedStatus` | `.1.3.6.1.4.1.3902.1082.500.20.2.3.2.1.7` |

Admin values 1/2 are unlocked/locked, exposed as enabled/disabled. Operational
values 1/2 are enabled/disabled, exposed as up/down. Unknown or absent values
remain `unknown`, not down. Raw enums are retained for diagnosis.
[Admin definition](https://mibs.observium.org/object/ZTE-AN-GPON-REMOTE-ONU-MIB/zxAnGponRmEthUniAdminState/),
[operational definition](https://mibs.observium.org/object/ZTE-AN-GPON-REMOTE-ONU-MIB/zxAnGponRmEthUniOperState/).

Speed codes 2–7 represent 10 half/full, 100 half/full, 1000 full and 10000 full.
Code 1 does not establish a negotiated rate, and 65535 means not in service.
The API sets `speed_mbps` to null and duplex to unknown unless the link is up and
its rate is recognized. Even if a down port retains an old speed code, that code
is not presented as its live rate.
[Speed definition](https://mibs.observium.org/object/ZTE-AN-GPON-REMOTE-ONU-MIB/zxAnGponRmEthUniIfSpeedStatus/).

`port_index` preserves the MIB's UNI identifier. The tested single-port ONU
reported index 1, corresponding to CLI `eth_0/1`; do not assume the same display
naming on every model. Known models (for example F601 = 1 UNI, F668 = 4) pad
missing indexes `1..N` as `unknown`; unknown models are not padded. Ethernet
`observed_at` is collector UTC time, not the last link-change time or a
guarantee that the OLT refreshed its own remote-state cache then. Live ONU
detail also sets top-level `observed_at` for that GET.

## Collection status and monitoring interpretation

| `ethernet.status` | Meaning |
|---|---|
| `ok` | Port states were read; an established link has a recognized speed |
| `partial` | Some columns failed, are missing, or contain unknown values |
| `unavailable` | No usable port rows; unsupported data, timeout or malformed response may be the cause |
| `onu_not_online` | Ethernet collection skipped because the ONU is not online |
| `onu_state_unknown` | Ethernet collection skipped because ONU state could not be established |

For an offline ONU, the collector avoids presenting cached remote Ethernet state
as a current link observation. An empty port list does not mean the ONU has zero
Ethernet ports. Missing values and partial collection must not trigger a confirmed
link-down alarm.

For monitoring, distinguish:

- ONU offline: investigate PON/power/device reachability first.
- ONU online, Ethernet administratively disabled: port configuration state.
- ONU online, enabled port, link down: customer-facing link observation; it may be
  expected on unused ports. Alert only against an inventory of expected active ports.
- Link up at an unexpected rate: compare with the intended service and connected
  equipment; a 100 Mbit/s client may be intentional.

This change supplies API observations. History samples persist TX and a compact
Ethernet summary (`eth_status`, `eth_link_state`, `eth_admin_state`,
`eth_speed_mbps`, `eth_ports`) from the same PON-list walk. They do not add
alarm state, traffic/error counters or per-ONU Prometheus metrics. No customer
port is disconnected or toggled for tests.

Stored snapshots remain the source of truth. Schema v3 also derives
`onu_status_events` and `onu_eth_events` from those samples (backfill on
upgrade, then each poller cycle). That is a change log, not a guarantee that
every flap between polls was captured. The planned [installer field
view](field-use.md) must still acquire fresh optical readings and report
per-field collection success and age. History and last-known values must stay
visibly separate from new observations.

# First C300 acceptance test

Do this only after the synthetic test and service startup succeed. A passing
simulator proves software wiring, not vendor MIB compatibility. Keep results
under ignored `private/` paths or another private system.

## Before querying

Record privately: C300 model and firmware, installed cards and physical slots,
PON count per card, shelf, SNMP view permissions, one known PON/ONU, OLT timezone,
collector image ID and `/version`. Use a read-only community restricted to the
collector's outbound address. Leave `POLL_ENABLED=false`, `CACHE_PREWARM=false`
and concurrency 1 during these on-demand checks. Enable bounded background
polling only after the collection/load checks pass.

## Acceptance sequence

1. Confirm uptime through SNMP, then `/healthz` and `/readyz` through the collector.
2. Select one PON with a few known ONUs. Retrieve its list once.
3. Compare ONU IDs, names, types and serials with the device CLI. Confirm the
   physical shelf/slot/PON mapping before interpreting any values.
4. Retrieve one known online ONU's detail. Compare RX/TX units and direction,
   optical distance, state, IP and online/offline timestamps with CLI output.
   An absent optional field should be recorded, not invented or treated as zero.
5. Compare one already-offline ONU if available. **Do not disconnect a subscriber
   for this test.** Check the raw sentinel/no-signal value and its API rendering.
6. Repeat the PON list within the 60-second example TTL. Confirm the cache is used
   in logs. Clear that PON's cache through the API, then repeat once.
7. Check a nonexistent/unconfigured slot returns 400 and a request without a key
   returns 401. Confirm neither case causes collection against another device.
8. Query `/uplinks` once. Compare interface names/states/speeds and raw card
   descriptions and `slot_source`; distinguish reported positions from fallback guesses.
   Confirm ZTE `SmartGroupN` LAGs appear as `kind: lag` and that member `gei_`/`xgei_`
   ports list the aggregator in `lag` when the agent exposes ifStack.
9. Measure cold and cached request latency, SNMP timeouts, host memory/CPU and OLT
   management load. Extend to a second PON only after the first is consistent.
10. Restart only the collector and check readiness and the same ONU again. Preserve
    the exact settings and results for the first accepted baseline.
11. After polling is enabled, confirm `/history/runs` and `/history/samples`
    (`run=latest`) contain the known PON. Check `/history/status-events` for a
    serial that changed state. Do not treat a new schema version as data loss:
    `onu_samples` must still be present.
12. Query `/history/unauth`. Record `status` (`ok` / `empty` / `unsupported` /
    `unavailable`). Compare any listed serials with the OLT CLI. `unsupported`
    is not a verified zero.

Use firmware-specific **show/read** commands you already know on the OLT. This
repository does not ship configuration/provisioning commands for the chassis.

## Result template

| Check | CLI/raw SNMP observation | API observation | Pass / mismatch / unavailable |
|---|---|---|---|
| Physical mapping | | | |
| ONU identity and serial | | | |
| Online/offline state | | | |
| RX/TX power and units | | | |
| Offline optical sentinel | | | |
| Distance | | | |
| Timestamps/timezone | | | |
| Uplink interfaces | | | |
| Card ent_index / slot | | | |
| Cold/cached latency | | | |
| Host/OLT load | | | |
| History samples after first poll | | | |
| Unconfigured-ONU walk status | | | |

Do not call the hardware supported until mandatory identity, mapping, state and
optical checks pass. Mark optional unsupported fields explicitly. A readiness
200 or a nonempty JSON response alone is insufficient.

## Known interpretation risks

- ZTE OID layout and both index families are inherited. Shelf 1 and ONU IDs 1–128
  are assumed. Other shelves, cards or software variants may need profiles.
- ANI optical conversion uses the MIB-defined `raw * 0.002 - 30`, with 65535
  treated as unavailable. Verify other firmware encodings before adapting them.
- Dates are decoded from eight-byte device values. Other encodings and clock
  errors can yield blank or incorrect durations. All OLTs share one timezone.
- Card slot prefers ENTITY-MIB parent-relative position. If absent, it falls back
  to `ent_index / 10 - 1` with `slot_source=index_heuristic`; retain the raw index.
- Partial field failures may still produce HTTP 200. Logs and field completeness
  must be reviewed together.
- The HTTP deadline is 90 seconds; some SNMP operations run with their own timeout
  and retry limits. Load limits and shutdown behavior under sustained polling
  require a separate scale test before production integration.

## If a check fails

Stop expanding scope. Capture only the minimum raw OID/value pair and matching
CLI observation privately, redact identifiers for a fixture, and add a regression
test before changing mapping/conversion code. Avoid repeated whole-enterprise
walks or raising concurrency to mask timeouts. Leave the previous tested image
available for rollback.

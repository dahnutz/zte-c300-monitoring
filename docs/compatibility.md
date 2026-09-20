# Compatibility and validation boundaries

The intended target is the ZTE C300 family. Existing C320-derived code is not a
guarantee for every card, ONU or firmware. Exact operator hardware inventory,
firmware revisions, subscriber identities and raw captures are private evidence,
not part of the public compatibility description.

## Behaviors observed or covered by regression tests

Limited hardware comparisons informed these implementation rules:

- Verify ONU identity, slot/PON indexing and operational state against CLI/raw
  SNMP. A successful uptime query alone does not validate vendor objects.
- ONU RX and Ethernet state have limited CLI cross-checks. TX uses the documented
  GPON ANI object and its unavailable marker; sequential CLI/SNMP readings are
  not a simultaneous numerical calibration.
- Optional IP and offline-history fields may be unavailable. Do not substitute
  zeros or invent timestamps.
- Uplink discovery recognizes `gei_`, `xgei_` and `SmartGroupN` naming patterns.
  LAG membership uses `ifStackStatus` where exposed. These are protocol/parser
  patterns, not a statement about any operator's installed interfaces.
- Some agents classify line/control cards as ENTITY-MIB class 3 and power cards
  as class 6. Description-qualified acceptance complements standard module class
  9; synthetic tests reject non-card chassis/power rows.
- `entPhysicalParentRelPos` is preferred for card positions. `slot_source` labels
  the heuristic fallback. Parent-relative position is not a globally unique slot.

Public fixtures use artificial identities, positions and measurements. They
exercise software behavior and do not establish hardware/model compatibility.
The current history store, poller and frontend require their own acceptance;
earlier collector tests do not certify the full four-service deployment.

## Evidence required for a deployment

Follow [hardware testing](hardware-testing.md). Record firmware, cards, ONU types,
raw readings and collection cost privately. Check already-offline ONU behavior,
partial/unsupported values and multiport UNI handling without service disruption.
Measure polling cost on a bounded scope before enabling a whole-device pass.

A public compatibility claim for a specific firmware/model should only be added
with deliberately approved, sanitized evidence. No broader compatibility or
production-readiness claim follows from the limited observations above.

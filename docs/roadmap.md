# NOC roadmap

Updated: 20 September 2026. This is the source of truth for priorities and planned
work. [Architecture](architecture.md) explains the design decisions; the
[API catalog](api.md) documents only routes that exist. A planned route or
configuration mode below is not available until implemented and tested.

## Product boundary

A standalone PON monitoring and troubleshooting app for a small ISP's NOC and
field installers. Keep the current collector, Redis, TimescaleDB and React/nginx
frontend. Integrations are optional consumers of a platform-neutral API; no
external platform is required and no replacement of this app is planned.

Installer access is a primary use case: on a phone, find the correct ONU/PON,
request real optical observations and see whether a reading is fresh, stale or
unavailable while working on fiber. History supports diagnosis but cannot stand
in for a new measurement. See [field mode](field-use.md).

| Responsibility | Direction |
|---|---|
| Vendor OIDs, index mapping, unavailable values, ONU/UNI identity | Collector |
| NOC inventory, local history and collection status | Existing app and frontend |
| Fresh optical readings for onsite work | Authenticated mobile field mode; next priority |
| Unconfigured ONU observations | Candidate walk + `/history/unauth` + Unconfigured page; CLI match still required |
| MAC/VLAN correlation | Collector/API/frontend after verified FDB mapping |
| Persistent observations and cache | Keep TimescaleDB and Redis in their current distinct roles |
| Optional external consumers | Versioned API with stable identity, timestamps and bounded reads |
| Intended service and permitted network scope | Approved operator configuration or optional inventory integration |

Access to the OLT remains read-only. No automatic ONU registration, deletion,
reset, provisioning or configuration changes are in this roadmap. Avoid adding
a CRM, billing, ticketing or general-purpose monitoring platform as a prerequisite.

## Implemented baseline and current gaps

“Implemented” means present in the source. It does not imply production
acceptance on every firmware, completed security review or current deployment.

| Capability | Current state | Next acceptance requirement |
|---|---|---|
| Go SNMP/API + Redis | Implemented; limited hardware observations | Error/freshness semantics and repeatable firmware acceptance |
| ONU RX/TX and Ethernet UNI | Implemented in details and PON collection | Partial data, offline/unknown state and cost under populated PONs |
| Uplinks, LAGs and cards | Implemented | Physical/parent-relative mapping and missing `ifStack` evidence |
| Timescale history + poller | Implemented; optional natively, included in Compose | Retention, restore, interrupted-run handling and measured cycle budget |
| React frontend served by nginx | Implemented; history plus Query now and Unconfigured | Authentication/TLS, stale-data presentation, default-OLT limitation |
| Browser login / TLS | Not implemented in supplied nginx template | P0 access boundary below |
| Mobile live optical view | Not implemented; Query now is a live GET only | Optical freshness contract, bounded reads and phone acceptance |
| External integrations | Versioned API exists; no platform adapter required | Consistent documented data contract |
| Unconfigured ONUs | Implemented as candidate walk | CLI-matched OID; `unsupported` must not be treated as zero |
| MAC history, traffic counters | Not implemented | Firmware/OID evidence before implementation |
| Current storage | Redis cache + Timescale history | Keep both; retention/restore and measured collection cost |

The frontend now chooses the newest finished run, shows collection time and
warns about partial/limited results. It does not auto-refresh or paginate past
the row cap. Query now validates the returned ONU identity, clears previous
readings before retry and ignores cancelled requests. Per-field sensor age,
complete snapshots and shared mobile access remain unfinished. Existing optical
charts are basic troubleshooting views; field mode still needs acceptance.

## Priority and dependencies

| Priority | Deliverable | Depends on | Done when |
|---|---|---|---|
| P0 | Public-data hygiene and consistent documentation | Source inventory | Fixtures are synthetic; docs describe actual routes/defaults; private captures excluded |
| P0 | Authenticated HTTPS frontend | Existing nginx UI | UI and proxied API require login; no alternate unauthenticated path; certificates renew |
| P0 | Trustworthy collection and snapshots | Current collector/poller | Staleness, completeness and failures are explicit; load and shutdown limits verified |
| P0 | Installer field mode | Auth + optical freshness contract | Selected-ONU live readings, mobile UI, stale/error states and bounded refresh |
| P1 | Platform-neutral API contract | P0 observation semantics | Optional consumers can use documented snapshots without metric-by-metric SNMP |
| P1 | Unconfigured ONU list (CLI-confirmed) | Candidate `/history/unauth` | Known unconfigured ONU matches CLI; unsupported table is not reported as zero |
| P1 | Current-stack operations | Existing Redis/Timescale | Retention, backup/restore and database growth are verified |
| P2 | Optional MAC/VLAN history | Verified FDB mapping + storage decision | Search and changes are useful across ONU moves, aging and missed scans |
| P3 | Optional per-ONU traffic | Verified counter scope + polling budget | Counter scope/discontinuities verified; rates and consumers are explicitly documented |
| Release gate | Repeatable install/upgrade/restore | Chosen feature scope | Clean VM acceptance, safe source export, pinned tested images, migration/rollback evidence |

Priorities are an order of work, not delivery dates. Each feature must include
its tests, API/schema updates and documentation in the same change.

## P0 — shared access and reliable observations

### Frontend and authentication

Extend the existing nginx frontend rather than starting a second dashboard.
Use named accounts over HTTPS for NOC staff and installers on mobile devices.
Basic Auth is the first option for a common approved read-only network view. The
recommended default is nginx with a mounted password file and certificates;
reuse an existing Nginx Proxy Manager deployment if it already owns certificates
and proxy hosts. See [edge access](edge-access.md) for the comparison and gates.

- Protect static pages and all browser API routes, including version/readiness.
- Keep collector, Redis and database ports internal/loopback; no auth bypass.
- Keep the collector key server-side. Browser users currently share that key's
  privileges; Basic Auth alone does not implement API per-user ownership. If
  installer scopes differ, enforce device/PON permissions server-side before access.
- Add per-user access logs without credentials or subscriber payloads. Only
  accept forwarded identity from a trusted edge, never arbitrary client headers.
- Review browser route/method exposure: current nginx forwards read routes across the API,
  including expensive live reads; cache mutation methods are blocked. Limit the browser
  path to approved read-only history and bounded selected-ONU live reads. Block
  cache mutation/bulk administrative routes for installer accounts.
- Verify HTTPS, rejected credentials, certificate renewal/reload, logout limits
  of Basic Auth, mobile reconnection/revocation, and that healthchecks cannot
  expose an unauthenticated API. Phones never receive server API/SNMP credentials.

### Installer field mode

Implement manual **Read now** for one selected ONU before auto-refresh. Add
large RX/TX values, units/direction, ONU/UNI state, collection time and age.
A fresh read bypasses history/PON cache; a failed field remains unavailable or
last-known, never a false current value. Device-side sensor age may be unknown.

Then add opt-in bounded refresh while the page is visible. Pause on page hide,
network loss or session end, coalesce concurrent users and enforce per-OLT
budgets. A candidate cadence is 10 seconds per selected ONU, subject to device
validation; do not shorten whole-network polling to make one phone feel live.
The detailed contract and mobile acceptance cases are in [field use](field-use.md).

### Poller/API correctness and NOC usability

- Define a snapshot contract with device identity, run ID, start/end times,
  observation age, expected/completed PONs, partial status and field errors.
- Never turn a timeout, missing MIB, missing serial or failed PON into “offline”,
  zero traffic or an empty discovery result. Keep last good data visibly stale.
- Prefer latest completed observations with explicit gaps. An older larger run
  must be labelled as fallback, not silently presented as the newest inventory.
- Use `(device_id, serial)` when available; retain position and timestamp, and
  handle missing/duplicate serials without merging unrelated ONUs. Reused ONU
  IDs do not establish device identity.
- Add per-device/PON scope and collection classes: fast state, slower optical/UNI,
  slow discovery/FDB. Defaults must be measured, configurable and conservative.
- Share collection work across poller, API and cache refresh; cap work per OLT,
  bound cycle duration, add jitter/backoff and avoid catch-up bursts after overruns.
  A serial loop alone does not bound one stalled SNMP walk.
- Expose cycle duration, last success, lag, errors and queue/load indicators to
  the API/collector metrics. Exercise cancellation, OLT outage and restart recovery.
- Start with polling off and one selected PON. Enable wider polling only after
  CLI/raw-SNMP comparison and management-load measurement.

## P1 — platform-neutral API

Keep the current HTTP JSON/OpenAPI interface available to multiple optional
consumers. No mandatory external platform, platform-specific template or local
feature removal is implied.

- Define versioned snapshot and live-observation contracts: device/ONU identity,
  source, timestamps, age, completeness, unsupported values and errors.
- Snapshot/list/history reads serve existing observations without an SNMP query
  per consumer metric. Live acquisition must be explicit, authorized and bounded.
- Document filters, pagination/limits, stable IDs, freshness, error handling and
  compatibility policy. Do not force consumers to know the internal SQL schema.
- Use separate integration credentials with approved device scope. Do not give
  mobile users integration secrets or assume one global key implements roles.
- Verify behavior across ONU movement/reused IDs, failed PONs, no-data and recovery.
- Keep future adapters separate from core collection. Add one only when there
  is a concrete use case and testable contract.

## P1 — unconfigured / unregistered ONUs

This means devices discovered by the OLT but not provisioned. It is different
from an unused ONU ID (`/onu_id/empty`), and different from a provisioned ONU
that is offline. A free-ID list cannot implement this feature.

- Confirm the firmware's discovery/unauthorized-ONU table with named MIB objects
  and CLI output; do not invent an OID or require production CLI scraping.
  `/history/unauth` and the Unconfigured UI page are wired to a candidate walk;
  a failed walk is stored as `unsupported`, never as a verified empty list.
- Proposed data: device, reported serial/vendor, observed PON/position if known,
  collector first/last seen, sample time, discovery state and completeness.
- Move discovery from every poller cycle to its own bounded slow cadence.
  Retain the existing stored-list API and document any contract changes.
- Keep `unsupported`, `unavailable`, `partial` and verified-empty distinct.
- Show a simple searchable table in the existing UI; export the same observation
  through the API for optional consumers. It must offer no “authorize” button
  or automatic registration.

Acceptance: known unconfigured ONU matches CLI; absent/unsupported table is not
reported as zero devices; transitions into provisioned inventory retain identity.
No customer ONU is deprovisioned merely to create a test case.

## P2 — MAC history and retention

### Keep the current storage direction

Redis remains the inventory/cache service; TimescaleDB remains the durable
observation/history store. Do not replace either as part of installer access.
Field reads must explicitly bypass stale observations while using the existing
collection budget. Implement retention, backup/restore and interrupted-run
handling. Storage simplification is a later measured decision, not a prerequisite.
See [architecture](architecture.md).

### MAC/VLAN observations

The operational question is: “Where was this MAC last observed, on which ONU,
UNI and VLAN, and did that attachment change?”

- Verify bridge/FDB indexes and mapping to ONU/UNI/PON. Preserve VLAN and device
  scope; the same MAC may legitimately appear in different VLANs or devices.
- Store first/last observed times, sample time, attachment and scan completeness.
  Report moves and last-known location without asserting ownership/person identity.
- A missed/failed scan is not disappearance. Dynamic FDB aging and an idle client
  can legitimately produce no MAC while UNI is up.
- Do not infer transparent L2 service health from MAC absence alone, and do not
  require active traffic injection for routine discovery.
- Poll FDB separately from optics with bounded rows, cadence and retention.
  Proposed initial retention: 30 days of attachment changes, operator configurable
  and subject to measured volume; not repeated full-table copies every cycle.
- Add lookup and an attachment timeline/export plus a platform-neutral API
  summary. Keep customer/operational identifiers restricted to authorized users.

Acceptance covers aging, failed scans, ONU relocation, multiple VLANs/UNIs and
restart persistence. MACs/serials are operational identifiers: access and export
controls must cover them, and raw observations never become repository fixtures.

## P3 — per-ONU traffic, conditional

First establish whether a reliable per-ONU/UNI counter exists. A PON aggregate,
GEM/service counter and Ethernet UNI counter are not interchangeable.

- Reuse validated standard interface counters where their ONU/UNI mapping is
  established; do not infer per-ONU traffic from a shared PON aggregate.
- If vendor decoding is needed, expose raw 64-bit octet counters, direction,
  counter scope, observation time and discontinuity/reset information.
- Rate requires two successful observations: `8 * delta_octets / delta_seconds`.
  First sample, reboot, counter reset/wrap, missing data or identity change must
  not create negative rates or spikes. Unknown is not zero.
- Validate directions against known CLI counters, consider 32-bit wrap limitations,
  and measure collection cost before enabling all ONU/UNI counters.
- Expose documented counters and optional validated rates through the API. Decide
  local traffic views versus external consumers after collection is reliable;
  billing and 95th-percentile service reports are not part of this milestone.

## Preview publication versus deployment acceptance

An early source preview can be published with synthetic tests, clear limits and
contribution guidance. Shared installer access requires the P0 authentication and
freshness gates. A source release does not certify a deployment or firmware.

## Release and maintenance gates

- Re-run public-file/fixture review on the actual export; exclude credentials,
  unapproved operator details, real serials, private hardware inventory and generated bytecode.
- Test clean Debian/Compose install, TLS edge, image upgrade, database restore,
  polling disable and graceful shutdown. Do not equate code presence with proof.
- Implement bounded retention for samples **and** run/identity/MAC metadata.
  Preserve identity/last-seen semantics when deleting old observations.
- Make migrations versioned and reversible by restore; a previous image alone
  may not be a valid database rollback. Review dependencies and pin release images.
- Protect management traffic; evaluate SNMPv3 support if required by deployment
  policy, with tested firmware credentials/algorithms and no bundled secrets.
- Keep API examples/schema, UI behavior and configuration defaults synchronized
  with tests. Avoid “done” labels based solely on a successful synthetic read.

## Deferred / explicitly excluded

No provisioning, writes, billing, CRM, CMDB, topology editor, ticket system,
custom notification engine, SLA dashboard or custom identity provider. No
Kubernetes/HA or vendor expansion without an actual deployment and test need.
NetBox inventory and links to existing monitoring are useful later integrations;
Wazuh can receive sanitized collector logs where used, but is not assumed installed.

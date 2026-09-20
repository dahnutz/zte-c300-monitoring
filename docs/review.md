# Repository review and current gaps

Updated: 20 September 2026. This is a source/configuration review, not a complete
security audit or certification of every retained upstream behavior.
[Roadmap](roadmap.md) owns future priorities; this page records the implementation
and the scope of verification.

## Retained foundation and cleanup

The imported Go collector retains static multi-OLT inventory, API key ownership,
SNMP/Redis repositories, cache management, health endpoints and collector metrics.
Original attribution remains in `LICENSE` and `UPSTREAM.md`.

Upstream deployment/publishing extras, Helm material, IDE files, load generators
and promotional assets were excluded. A subsequent pass removed 22 source/test
files: the trap/notification subsystem, development webhook routes and external
HTTP device-registry client. Their hooks, stale settings and cron dependency
were removed. Read-only device collection remains the scope.

## Implemented changes

- Configuration/authentication validation, resolved Redis defaults, bounded JSON
  SNMP port parsing, explicit slot/PON checks and a metadata race fix.
- HTTP timeouts, shutdown hooks, image healthcheck, nonroot collector image,
  loopback publication, internal databases, cache cap and rotated logs.
- ENTITY-MIB card classification and reported parent-relative positions.
- Documented GPON ANI TX object/index, optical unavailable marker, Ethernet
  admin/link/speed/duplex and partial-data semantics.
- PON-scoped Ethernet list collection, Timescale sample/run storage, sequential
  polling, additive schema v3 (status/UNI events, unconfigured-ONU discovery),
  history endpoints and a React/nginx frontend (Query now, Unconfigured page).
- Public deployment, configuration, API, telemetry and hardware-acceptance guides.

## Evidence boundaries

The initial collector baseline passed formatting/vet/race tests and a synthetic
container smoke test with Redis and a UDP SNMP fixture. Limited real-device
comparisons informed some telemetry mappings; see [compatibility](compatibility.md).
These facts do not certify every later history/frontend change.

`scripts/smoke.sh` tests collector/Redis/fixture behavior, authentication, topology
validation and cached data during simulated OLT loss. It does **not** deploy
TimescaleDB, exercise database restore or test browser authentication/TLS.
A UI build proves compilation, not the correctness of every operator workflow.
A clean Debian/Docker deployment and complete shared-access acceptance remain
release gates; do not substitute a local container test for them.

## Documentation/publication reconciliation

The current tree has four Compose services and a frontend. Older statements
about “no dashboard”, API-only deployment and future-only UI were stale. Updated
README, configuration, deployment and UI guides now distinguish the actual code
from proposed modes. The [publication review](publication-review.md) records the
privacy boundary without reproducing removed values.

Public defaults now require a unique database password and leave polling off for
initial acceptance. Existing private `.env` values and running containers are
not changed by editing the public defaults. No password rotation or redeployment
is implied. Changing an initialized database password requires a coordinated
role/configuration update.

## Outstanding risks to address

- The UI has no browser authentication/TLS; proxying a server API key is not a
  login. The proxy permits GET/HEAD only, but still exposes more read routes than the pages need.
- The frontend now selects the newest finished run and warns on partial/capped
  data. Complete pagination, per-field age and coordinated snapshots are pending.
- Some failed fields still yield partial HTTP 200 data. Empty/unknown/offline
  need consistent semantics across API, storage, UI and optional integrations.
- Sequential polling does not provide a complete deadline/backoff/budget policy;
  foreground refresh and polling can still contend for device resources.
- TimescaleDB has a persistent volume but no configured retention policy. Backup,
  restore, migration and interrupted-run recovery need acceptance evidence.
- Redis remains coupled to API/cache readiness; the current poller requires the
  durable store. Keep both services. Installer field mode must add fresh
  acquisition and metadata without presenting cached/history values as live.
- Default-OLT cache keys remain unprefixed. Do not share a Redis database between
  independent collectors without isolation/coordination work.
- Unconfigured-ONU discovery is a candidate walk (`/history/unauth`); firmware
  CLI match is still required. MAC history, traffic counters, mobile field mode
  and SNMPv3 are not implemented. No provisioning is planned.
- Base image tags are development references. Release images need dependency
  review, reproducible tests and pinned identities.

See the [prioritized roadmap](roadmap.md) and [architecture decisions](architecture.md)
for the next work and its acceptance criteria. Keep changes reviewable; source
presence, test coverage and deployed acceptance are separate claims.

This repository is maintained by one person in spare time. Contributions that
reproduce and fix inconsistencies are welcome; see [CONTRIBUTING](../CONTRIBUTING.md).

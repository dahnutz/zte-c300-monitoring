# Upstream provenance

- Source: https://github.com/Cepat-Kilat-Teknologi/snmp-olt-zte
- Revision: `a8f4635e94929d21d634b5ccd77c47a4510fa233`
- Imported: 2026-09-16
- License: MIT; the original copyright notice is retained in `LICENSE`.

Imported the Go runtime, tests, dependencies and API specification. This adapted
project is hosted at https://github.com/dahnutz/zte-c300-monitoring. The Go module
retains the local name `zte-c300-monitoring`; this repository builds an application,
not a separately versioned Go client library.

Excluded upstream Kubernetes/Helm examples, IDE settings, load generators,
organization deployment files, publishing workflows and promotional material.
Added a local build/test setup, container health checks, a synthetic SNMP fixture,
a reproducible smoke test, and a documented Compose deployment.

The subsequent repository review removed the trap/power monitor and outbound
notification modules, development webhook endpoints, the external HTTP inventory
client and its poller, and the unused cron dependency (22 source/test files).
Static multi-OLT inventory and API key ownership are retained.

Local defaults: cache pre-warming off, UTC OLT timezone, native HTTP on localhost.
Service startup requires API_KEY or API_USERS. Compose publishes only localhost
HTTP and keeps Redis internal. Read-only collection and cache management remain.

Fixed an inherited metadata race in the dynamic OLT registry found by the race
detector: reconciliation now replaces entry metadata snapshots instead of
mutating entries that active readers may still hold. Existing connections are
reused. The regression test checks that readers retain a stable snapshot.

Automated test success does not establish hardware compatibility. Several OID mappings and conversions remain inherited; selected optical and
Ethernet paths have since been adapted. Broader hardware acceptance remains
required; see docs/compatibility.md. This project is independent of the upstream maintainer.

Further fixes: preserve resolved Redis defaults in production; reject overflowing
JSON SNMP ports and malformed explicit slot layouts; classify ENTITY-MIB modules
using standard class 9 plus description-qualified firmware classes 3/6, with
reported parent-relative positions preferred; load local environment before logging; bound header/idle waits; release
signal handlers on shutdown. See docs/review.md for verification and remaining
hardware assumptions. No upstream test is evidence of testing our target device.

The first hardware iteration also corrects ONU TX to the GPON ANI object under
1082, handles the documented 65535 optical-unavailable marker, and adds scoped
ONU Ethernet UNI observations to the detail response. Definitions and behavior
are documented in docs/onu-telemetry.md; no proprietary MIB source is bundled.

Later additions include Timescale history, a sequential poller and a React/nginx
frontend. They are local extensions, not upstream features or a claim of full
production acceptance. Current capabilities, installer access, fresh optical
readings and optional platform-neutral integrations are documented in
docs/roadmap.md and docs/architecture.md.

# Installer mobile workflow and live optical readings

**Planned field mode is not implemented yet.** ONU detail already has **Query
now**: one HTTP request with multiple SNMP reads for that ONU (`observed_at` is the collector clock).
That button is not field mode. It lacks named-user HTTPS, per-field optical
age, bounded auto-refresh and an accepted phone layout. The current button
checks ONU identity and clears previous values before a new attempt; a complete
per-field stale/failed presentation remains planned.

The next user-facing workflow: an installer signs in on a phone, finds the
ONU/PON being worked on, and checks fresh optical readings while installing or
repairing fiber. Inventory pages show historical samples and must not be
presented as a live fiber-work screen.

## Workflow

1. Open the HTTPS app and authenticate with an individual installer account.
2. Find the target by permitted OLT/PON, ONU serial or an approved service label.
   Keep the selected device, position and identity visible throughout the session.
3. Open **Field mode** and explicitly request a fresh reading. Show a loading
   state; do not relabel the previous history value as current while waiting.
4. Display large RX/TX values with dBm units, ONU state, UNI link state, collection
   time and age. Provide a large **Read now** button for one-handed use.
5. Optionally start bounded auto-refresh for that ONU only. Make its running,
   paused, offline and failed states obvious. Pause when leaving/hiding the page.
6. Stop when the work ends; retain normal history separately. A phone reconnect
   or tab resume requires a fresh read before any measurement is labelled live.

The inventory view helps locate the network element. It may use cached/stored
observations with visible age; it must not trigger a whole-OLT walk on every
phone refresh. Field mode is a separate acquisition action.

## What “live” must mean

- Request a fresh SNMP read of the selected ONU through the collector. Do not
  serve a Redis PON-list value or Timescale history row as a new live measurement.
- Report actual collection start/end and success for the optical fields. Update
  age from those timestamps, not the phone's page-render or HTTP arrival time.
- Distinguish **fresh**, **refreshing**, **stale**, **unavailable**, and **failed**.
  These are proposed UI states, not existing API enum values.
- A failed RX read cannot inherit the timestamp of a successful TX/UNI read.
  Show partial results honestly; HTTP 200 alone is not successful optical acquisition.
- If the last good value remains visible, label it “Last known” with its age and
  failure reason. It must not retain a reassuring live/green indicator.
- Do not turn missing signal, unsupported MIB or the optical sentinel into
  `0 dBm`. Offline/disconnected ONUs may not supply fresh remote optical values.
- Include units and direction: **ONU RX** is downstream receive power at the ONU;
  **ONU TX** is upstream emission by the ONU. Neither is **OLT RX**. Add OLT-side
  readings only after their separate objects/directions are verified.
- Do not infer fiber attenuation from ONU RX and ONU TX: they are opposite
  directions. A link-loss calculation needs the corresponding transmitter and
  receiver measurements in the same direction, with timing/validity checks.
- SNMP acquisition time is not guaranteed sensor sampling time. If the device
  does not provide measurement age, say that it is unknown; measure its refresh
  behavior during hardware acceptance.

No promised sub-second streaming: OLT/ONU reporting cadence, SNMP latency and
load determine useful refresh frequency. A starting candidate is **10 seconds
per selected ONU**, configurable after measurement; it is not a shipped setting
or a guarantee. Manual refresh should be available first. Work requiring faster
or independent optical measurements still needs the appropriate field instrument.

## Collection and API implementation requirements

The current individual ONU-detail endpoint already bypasses Redis. It also reads
many unrelated fields sequentially and can return partial data without optical
freshness metadata. Reuse validated OID/conversion code, but assess a small optical
read path so field refresh does not repeatedly fetch description/offline history.
Do not invent or document a new endpoint as available before it exists.

- Explicit collection source, request identity, device/ONU identity, time window,
  per-field availability/error and last-good age in the response contract.
- No-store behavior on authenticated live responses at collector, nginx and
  browser; no service-worker/offline cache may masquerade as a current reading.
- At most one outstanding refresh per viewer/target. Coalesce identical in-flight
  requests and retain the shared read's real timestamps. Do not silently reuse
  older results under the same live label.
- Server-enforced per-user and per-OLT limits, finite request deadlines and
  bounded active monitoring sessions. Browser pause alone cannot bound load.
- Coordinate with the background poller; account for several installers and API
  clients on the same OLT. Back off on timeouts and do not queue catch-up bursts.
- On target change, discard late responses for the previous ONU. On mobile network
  loss, immediately show that updates have stopped; age keeps increasing.
- Do not fix freshness by flushing shared cache on every refresh, reducing the
  global poll interval for all ONUs, or exposing cache mutation routes to installers.

## Authentication and mobile presentation

- Named read-only users over HTTPS, with a usable revocation process. Keep
  passwords out of source, URLs and app-managed browser storage. Server
  API/SNMP keys must never be delivered to the browser.
- For a common approved network view, nginx Basic Auth is the initial option.
  Per-user OLT/PON restrictions require server-side authorization. Hiding UI
  links is insufficient. See [edge access](edge-access.md).
- Show only identifiers/service labels needed to locate the work; do not expose
  customer contact details or bulk exports merely because someone can view levels.
- Responsive portrait layout, large touch controls and readable values outdoors.
  Use text/icons alongside colour, and display freshness near each measurement.
- Keep existing history as an explicitly separate view. Numeric thresholds must
  be labelled/configured for the applicable optics/service, not presented as a
  universal pass/fail test.

## Acceptance before installer access

| Scenario | Required result |
|---|---|
| Normal fresh read | Correct target, units/direction, timestamp and CLI/meter comparison where applicable |
| Device reports an unchanged value | Show actual acquisition time; do not invent a changing measurement |
| Optical field fails but other reads succeed | Field unavailable/partial, no false live value |
| OLT unreachable / phone loses connectivity | Clear failure/stale state; previous value remains last-known only |
| ONU already offline | No fabricated current optical level; no customer disconnect for testing |
| Background tab / locked phone / closed page | Client refresh paused; server work remains bounded |
| Return to page / mobile network changes | Fresh acquisition before displaying a live badge |
| Change target during an in-flight read | Old response cannot overwrite the newly selected ONU |
| Several users on one OLT | Load bounded, requests coalesced, timestamps accurate; no fleet-wide walk |
| Invalid/revoked user / disallowed target | Pages and API deny access; direct URL cannot bypass scope |
| iOS and Android browsers | Trusted HTTPS, usable authentication, portrait layout and explicit reconnect behavior |

Record refresh latency, observed sensor-update behavior and management load
privately. Do not create public fixtures from installer/customer device data.

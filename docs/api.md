# HTTP API catalog

The collector is an HTTP JSON API. The Compose `ui` service is a browser
dashboard for Timescale history; it proxies `/api/` to this collector and is
documented in [UI](ui.md). That proxy currently has no browser authentication;
keep it on loopback. Direct API use remains `http://127.0.0.1:8081`.
Remote operators use the SSH tunnel in [deployment](deployment.md).

All `/api/v1` routes require `X-API-Key`. Liveness, readiness, version and
`/metrics` are unauthenticated; keep the listener on a private network.

Every `/api/v1/...` path below is also available as
`/api/v1/olt/{olt_id}/...` when more than one OLT is configured. The default
OLT continues to answer the bare paths.

Machine-readable schemas live in [OpenAPI](../api/openapi.yaml). This catalog
is the operator-facing list of every reachable path and the query parameters
that actually work.

## Conventions

| Item | Value |
|---|---|
| Auth | Header `X-API-Key` |
| Envelope | `{ "code": 200, "status": "success", "data": ... }` |
| Time | RFC3339 (synthetic example: `2026-01-01T12:00:00Z`) |
| Board / slot | Physical GPON slot from the operator-supplied `OLT_BOARDS` configuration |
| PON | Port within the configured count for that slot |
| ONU identity in history | Device scope plus `serial` (`serial_number` in JSON); position can change |
| SNMP | GET/GETBULK/GETNEXT only. HTTP POST/DELETE touch Redis, never the OLT |
| Rate limit | 100 rps, burst 200 |
| Timeout | 90 seconds (cold SNMP walks can be slow) |

Response headers include `X-API-Version`, `X-App-Version` and a request ID for
log correlation. Logs and payloads can contain subscriber names and serials;
sanitize before sharing.

Common errors: **400** bad parameters, **401** missing/unknown key, **404**
unknown OLT / missing ONU / missing collection run, **429** rate limit, **503**
OLT/Redis/Timescale unavailable, **5xx** collection failure.

## Helper

On the Compose host, use `scripts/query.py` (it is in git; `private/` is not).
It reads `API_KEY` from `.env`. Pass filters as separate `KEY=VALUE` arguments —
**do not** put unquoted `?a=1&b=2` on the command line; bash treats `&` as a
background job and only the first parameter is sent.

```sh
python3 scripts/query.py /api/v1/board/3/pon/1
python3 scripts/query.py /api/v1/history/runs limit=5
python3 scripts/query.py /api/v1/history/samples run=latest status=Offline
python3 scripts/query.py /api/v1/history/samples run=latest count_by=status
python3 scripts/query.py /api/v1/history/status-events serial=ZTEG00000001 limit=20
python3 scripts/query.py /api/v1/history/eth-events serial=ZTEG00000001 port=1
python3 scripts/query.py /api/v1/history/unauth
# quoted query string also works:
python3 scripts/query.py '/api/v1/history/samples?run=latest&status=Offline'
```

Pipe to `jq` when you want a slice of the envelope. Curl is optional; if you use
it, quote the whole URL.

---

## Unauthenticated

| Method | Path | Purpose |
|---|---|---|
| GET | `/` | Plain-text ping (`Hello, this is the root endpoint!`) |
| GET | `/health` | Liveness alias |
| GET | `/healthz` | Liveness (preferred) |
| GET | `/readyz` | Dependency probes (Redis, SNMP, Timescale). **503** if a critical probe is down |
| GET | `/version` | Build metadata (`version`, `api_version`, `commit`, `build_time`, `uptime`) |
| GET | `/metrics` | Prometheus text: HTTP, cache, SNMP, **poller** cost. Not per-ONU series |

```sh
curl --fail http://127.0.0.1:8081/healthz
curl --fail http://127.0.0.1:8081/readyz | jq
curl --fail http://127.0.0.1:8081/version | jq
curl --fail http://127.0.0.1:8081/metrics | head
```

`/readyz` returning 503 is expected when the OLT or Redis is unreachable.

---

## Authenticated live / cache (`/api/v1`)

These talk to SNMP and/or Redis. They are **not** the Timescale history.

| Method | Path | Query | Behavior |
|---|---|---|---|
| GET | `/board/{slot}/pon/{pon}` | `onu_id` is the only allowed query key; it is accepted and **ignored** (does not filter) | Cached PON ONU list |
| GET | `/board/{slot}/pon/{pon}/onu/{id}` | — | Live ONU detail (optical, ethernet, `observed_at`, last online/offline where firmware exposes them) |
| GET | `/board/{slot}/pon/{pon}/onu_id_sn` | `nocache=true` forces a fresh SNMP serial walk | Cached ONU ID + serial list |
| GET | `/paginate/board/{slot}/pon/{pon}` | `page` (1-indexed, default 1), `limit` (default 10, max 100) | Same PON list, sliced. **Not** `page_size` |
| GET | `/board/{slot}/pon/{pon}/onu_id/empty` | — | Cached observation of unoccupied IDs in 1..128 |
| POST | `/board/{slot}/pon/{pon}/onu_id/update` | — | Refresh the free-ID cache from SNMP |
| DELETE | `/board/{slot}/pon/{pon}/cache/clear` | — | Drop the PON list cache in Redis |
| DELETE | `/board/{slot}/pon/{pon}/onu/{id}/cache/clear` | — | Drop related ONU/list cache keys |
| GET | `/uplinks` | — | Live IF-MIB / ENTITY-MIB card and uplink discovery (uncached) |

Replace slot `3` / PON `1` / ONU `1` with values that exist on the OLT:

```sh
python3 scripts/query.py /api/v1/board/3/pon/1
python3 scripts/query.py /api/v1/board/3/pon/1/onu/1
python3 scripts/query.py /api/v1/board/3/pon/1/onu_id_sn
python3 scripts/query.py /api/v1/board/3/pon/1/onu_id_sn nocache=true
python3 scripts/query.py /api/v1/paginate/board/3/pon/1 page=1 limit=10
python3 scripts/query.py /api/v1/board/3/pon/1/onu_id/empty
python3 scripts/query.py /api/v1/uplinks
```

POST/DELETE still need curl (they only touch Redis):

```sh
BASE_URL=http://127.0.0.1:8081
read -r -s -p 'API key: ' COLLECTOR_KEY; echo
printf 'X-API-Key: %s\n' "$COLLECTOR_KEY" |
  curl --fail-with-body --silent --show-error --max-time 95 --header @- \
    -X POST "$BASE_URL/api/v1/board/3/pon/1/onu_id/update"
printf 'X-API-Key: %s\n' "$COLLECTOR_KEY" |
  curl --fail-with-body --silent --show-error --max-time 95 --header @- \
    -X DELETE "$BASE_URL/api/v1/board/3/pon/1/cache/clear"
```

PON list shape (synthetic, reduced):

```json
{
  "code": 200,
  "status": "success",
  "data": [
    {
      "board": 3,
      "pon": 1,
      "onu_id": 1,
      "name": "TEST-ONU",
      "onu_type": "SYNTHETIC-ONU",
      "serial_number": "ZTEG00000001",
      "rx_power": "-20.00",
      "tx_power": "2.00",
      "status": "Online",
      "ethernet": {
        "status": "ok",
        "observed_at": "2026-01-01T14:12:21Z",
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
  ]
}
```

Optical values on live endpoints are **strings**. Blank fields plus HTTP 200 do
not prove complete telemetry. ONU detail also has `ethernet` (admin/link,
speed, duplex) for an online ONU, `tx_power`, and `observed_at` (collector UTC
clock during that read, not a device sensor timestamp). One detail HTTP request
performs several SNMP operations and is not an atomic sensor snapshot. See [ONU telemetry](onu-telemetry.md). Cached PON
lists now include `tx_power` and `ethernet` (three UNI column walks per PON,
not per ONU). Known models pad missing UNI indexes `1..N`.

POST/DELETE change Redis only. They do not provision, delete or reboot an ONU.
An "empty" ONU ID is not a reservation.

Uplinks: `data.cards` and `data.ports`. Card `slot_source` is
`parent_relative_position` when the device reports a usable position, otherwise
`index_heuristic`. Physical uplinks are `gei_` / `xgei_`. ZTE LAG interfaces
(`SmartGroupN`) are included as `kind: lag`; `members` lists stacked member
ifNames when `ifStackStatus` is available, and member ports carry `lag`.

Named OLT (inventory id `lab-a`):

```sh
python3 scripts/query.py /api/v1/olt/lab-a/board/3/pon/1
python3 scripts/query.py /api/v1/olt/lab-a/uplinks
python3 scripts/query.py /api/v1/olt/lab-a/history/samples run=latest count_by=status
python3 scripts/query.py /api/v1/olt/lab-a/history/unauth
python3 scripts/query.py /api/v1/olt/lab-a/history/status-events limit=10
```

---

## Authenticated history (TimescaleDB)

Present only when TimescaleDB is configured (Compose always starts it). These
routes **do not query the OLT**. They read PON-list snapshots written by the
poller (`POLL_INTERVAL_SECONDS`, default 120). Without Timescale the same paths
return **503**.

| Method | Path | Purpose |
|---|---|---|
| GET | `/history/samples` | ONU snapshots (or a `count_by` aggregate) |
| GET | `/history/runs` | Poller cycle cost: duration, PONs ok/error, ONU rows written |
| GET | `/history/status-events` | Derived Online/Offline (and other) transitions |
| GET | `/history/eth-events` | Per-port UNI admin/link flaps |
| GET | `/history/unauth` | Last unconfigured-ONU discovery (`ok` / `empty` / `unsupported` / `unavailable`) |

The same five paths exist under `/api/v1/olt/{olt_id}/history/...`. They read
Timescale only, except that `/history/unauth` reports the last poller discovery
(itself an SNMP walk, not a live request from this GET).

### `GET /history/samples`

| Query | Default | Meaning |
|---|---|---|
| `run` | unset | `latest` / `last` = time window of the newest collection run; or a numeric run `id` from `/history/runs` |
| `status` | unset | Case-insensitive match (`Online`, `Offline`, …). Exact string the PON list stored |
| `serial` | unset | One ONU serial |
| `board` | unset | Physical slot |
| `pon` | unset | PON port |
| `onu_id` | unset | ONU index on that PON |
| `from` / `to` | last 24 hours | RFC3339 window. **Ignored** when `run` is set (the run's `started_at`–`finished_at` is used) |
| `limit` | 500 (2000 when `run` is set) | Max rows. Hard cap 2000. Ignored for `count_by` |
| `count_by` | unset | Aggregate instead of row list: `status`, `board`, `pon`, `onu_type`, `eth_link` |

Filters combine with AND. `count_by` still honours `run`, `status`, `serial`,
`board`, `pon`, `onu_id`.

Offline samples usually have `"rx_power": null` and `"tx_power": null` (the OLT
does not report optics when the ONU is down). Ethernet is stored as
`eth_status`, `eth_link_state`, `eth_admin_state`, `eth_speed_mbps`, and
`eth_ports`. Offline ONUs use `eth_status=onu_not_online` and do not keep a
stale UNI list. `eth_link_state` is `up` if any UNI is up. Sample rows also
include `status_changed_at`, `previous_status` and `expected_eth_ports` from
the current `onus` identity row (not rewritten onto each historical sample).

#### Last poller cycle, everything offline

```sh
python3 scripts/query.py /api/v1/history/samples run=latest status=Offline
python3 scripts/query.py /api/v1/history/samples run=latest status=Offline | jq '.data | length'
```

#### Last cycle, everything online

```sh
python3 scripts/query.py /api/v1/history/samples run=latest status=Online
```

#### Count in the last cycle (by status, board, PON, ONU type, or Ethernet link)

```sh
python3 scripts/query.py /api/v1/history/samples run=latest count_by=status | jq .data
python3 scripts/query.py /api/v1/history/samples run=latest count_by=board | jq .data
python3 scripts/query.py /api/v1/history/samples run=latest count_by=pon | jq .data
python3 scripts/query.py /api/v1/history/samples run=latest count_by=onu_type | jq .data
python3 scripts/query.py /api/v1/history/samples run=latest count_by=eth_link | jq .data
```

Synthetic `count_by=status` payload:

```json
{
  "code": 200,
  "status": "success",
  "data": {
    "group_by": "status",
    "run_id": 7,
    "from": "2026-01-01T12:58:01Z",
    "to": "2026-01-01T12:58:39Z",
    "total": 10,
    "counts": [
      { "key": "Online", "count": 8 },
      { "key": "Offline", "count": 2 }
    ]
  }
}
```

#### One serial over time (default last 24 hours)

```sh
python3 scripts/query.py /api/v1/history/samples serial=ZTEG00000001 limit=50
```

#### Slot / PON / ONU index

```sh
python3 scripts/query.py /api/v1/history/samples board=2 pon=1
python3 scripts/query.py /api/v1/history/samples board=2 pon=1 onu_id=4
python3 scripts/query.py /api/v1/history/samples board=2 pon=1 run=latest status=Offline
```

#### Explicit time window

```sh
python3 scripts/query.py /api/v1/history/samples \
  from=2026-01-01T00:00:00Z to=2026-01-01T23:59:59Z limit=200
```

#### A specific poller run by id

Ids are not stable across a Timescale recreate. List runs first, then pass that
`id` — do not copy `12` from the sample JSON above.

```sh
python3 scripts/query.py /api/v1/history/runs limit=5
python3 scripts/query.py /api/v1/history/samples run=latest status=Offline
# after you have a real id from /history/runs:
python3 scripts/query.py /api/v1/history/samples run=3 status=Offline
python3 scripts/query.py /api/v1/history/samples run=3 count_by=status | jq .data
```

`run=latest` with no stored cycles returns **404**. Unknown `count_by` returns
**400**.

### `GET /history/runs`

| Query | Default | Meaning |
|---|---|---|
| `limit` | 50 | Max cycles (cap 200), newest first |

```sh
python3 scripts/query.py /api/v1/history/runs
python3 scripts/query.py /api/v1/history/runs limit=5 |
  jq '.data[] | {id,started_at,status,pons_ok,pons_error,onus_sampled,duration_ms}'
```

`status` on a run is the poller outcome (`ok`, `partial`, …), not ONU Online/
Offline.

### `GET /history/status-events`

Derived Online/Offline (and other operational-state) changes. Built from
`onu_samples` on upgrade and from each new poller cycle. Existing samples are
not rewritten.

| Query | Default | Meaning |
|---|---|---|
| `serial` | all | ONU serial |
| `limit` | 50 | Newest first, cap 500 |

```sh
python3 scripts/query.py /api/v1/history/status-events serial=TEST00000001 limit=20
```

### `GET /history/eth-events`

UNI admin/link flaps, one row per port change.

| Query | Default | Meaning |
|---|---|---|
| `serial` | all | ONU serial |
| `port` | all | UNI index |
| `limit` | 50 | Newest first, cap 500 |

```sh
python3 scripts/query.py /api/v1/history/eth-events serial=TEST00000001 port=1
```

### `GET /history/unauth`

Last unconfigured-ONU discovery for this OLT. `status` is `ok`, `empty`,
`unsupported` or `unavailable`. `unsupported` means the firmware walk is not
confirmed — it is not a verified zero. Before the first discovery, status is
`unavailable` and there is no collection timestamp (the current JSON schema
serializes the zero time). `empty` is reserved for a confirmed empty discovery;
a missing table or rows without usable serials are `unsupported`. Last-known
rows may be retained after a failed scan and must be treated as stale.
Provisioned serials are filtered out.
This table is separate from `onu_samples`; history is not deleted when a
serial leaves the unconfigured list.

```sh
python3 scripts/query.py /api/v1/history/unauth
```

Sample rows also carry `status_changed_at`, `previous_status` and
`expected_eth_ports` from the current `onus` row. Those fields are not stored
on each historical sample.

---

## jq one-liners

```sh
# How many ONUs were Offline in the last poller pass?
python3 scripts/query.py /api/v1/history/samples run=latest count_by=status \
  | jq '.data.counts[] | select(.key=="Offline") | .count'

# Serials currently Offline in the last pass
python3 scripts/query.py /api/v1/history/samples run=latest status=Offline \
  | jq -r '.data[].serial_number'

# Online count on synthetic slot 2 only
python3 scripts/query.py /api/v1/history/samples run=latest board=2 count_by=status | jq .data

# Last status change for one serial
python3 scripts/query.py /api/v1/history/status-events serial=ZTEG00000001 limit=1 \
  | jq '.data[0] | {time,status,previous_status}'

# Unconfigured-ONU discovery (unsupported is not a verified zero)
python3 scripts/query.py /api/v1/history/unauth | jq '{status,count,message}'
```

## Planned capabilities are not current routes

MAC-history and traffic-counter endpoints are not present. The existing free-ID
endpoint still reports unused provisioning indexes; discovered unconfigured
devices are `/history/unauth`. Platform-neutral observation contracts and mobile
field mode are planned in the [roadmap](roadmap.md). Individual ONU detail is a
live SNMP read (`observed_at` is the collector clock). History responses are
stored observations; see [field workflow](field-use.md).

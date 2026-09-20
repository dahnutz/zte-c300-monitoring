# Configuration

Copy `.env.example` to `.env` and keep it private (`chmod 600`). Compose passes it
into the collector. A native run loads `.env` without overwriting already-exported
variables. Settings are read at startup: recreate the Compose container after edits.
Use single quotes around Compose values containing `$`, `#` or spaces.
Do not put real addresses, credentials or inventory into the public template.

## First-device settings

| Setting | Code default | Supplied template / purpose |
|---|---|---|
| `SNMP_HOST` | Required in single-OLT mode | Blank; supply a management address/hostname |
| `SNMP_PORT` | `161` | `161`, UDP |
| `SNMP_COMMUNITY` | Required in single-OLT mode | Blank; read-only SNMPv2c credential |
| `API_KEY` | No default | Required at service startup unless `API_USERS` is configured; always required by the supplied Compose file |
| `OLT_BOARDS` | Legacy `1,2` | **Set explicitly for C300**; template `3:16` is synthetic |
| `OLT_PONS_PER_BOARD` | `16` | Used for bare slot entries such as `3,5`; explicit `slot:count` takes precedence |
| `OLT_TIMEZONE` | `UTC` | Match the OLT clock, using an IANA name; shared by all OLTs in this instance |
| `SNMP_MAX_CONCURRENT` | `5` | Template `1`; limits per-OLT SNMP connections |
| `SNMP_TIMEOUT_SECONDS` | `5` | Per SNMP request, not the whole HTTP request |
| `SNMP_RETRIES` | `2` | Template `1`; zero is allowed |
| `SNMP_MAX_REPETITIONS` | `20` | Template `10`; GETBULK response size, positive integer |
| `CACHE_PREWARM` | `false` | Keep false during first acceptance; true collects all configured PONs at startup |
| `REDIS_ONU_INFO_TTL` | `1800` seconds | Template `60`; PON list and serial-list cache |
| `REDIS_EMPTY_ONU_ID_TTL` | `300` seconds | Template `60`; free-ID observations |

Slots are bounded to 1–30, PON counts to 1–16 and ONU IDs to 1–128 in the inherited
mapping. These bounds are not proof every combination exists on a C300. Explicit
malformed/duplicate slot specifications fail configuration validation. **Do not
omit `OLT_BOARDS`** and rely on the legacy C320 default for a C300.

Mixed example (replace with actual cards):

```dotenv
OLT_BOARDS=3:16,5:8
```

Each entry is `slot:port_count`: this example permits PONs 1–16 on slot 3
and PONs 1–8 on slot 5. Requests for unconfigured slots or out-of-range PONs
return HTTP 400 before querying the device. These are runtime settings, not
compiled limits. After editing `.env`, apply the change to the existing image:

```sh
docker compose up -d --no-build --wait collector
```

A container restart alone does not reload changed Compose environment values;
Compose must recreate the collector. Permitting a slot does not establish that
every PON contains ONUs or that every metric is supported by its firmware.

Supported mappings use the inherited ZTE enterprise roots `1.3.6.1.4.1.3902.1082`
and `1.3.6.1.4.1.3902.1012`. Internal overrides `OLT_BASE_OID_1`, `OLT_BASE_OID_2`,
`ONU_ID_NAME_PREFIX`, `ONU_TYPE_PREFIX` exist for development; they are not a
complete firmware profile mechanism and should remain unset during initial tests.
The inherited `REDIS_ONU_DETAIL_TTL` setting remains in the configuration type,
but live ONU detail requests bypass the detail cache; it does not control their
freshness. It is intentionally omitted from the template.

## Redis and HTTP

| Setting | Code default | Notes |
|---|---|---|
| `REDIS_HOST` / `REDIS_PORT` | `localhost` / `6379` | Compose forces host `redis` |
| `REDIS_PASSWORD` | Empty | Only for a separately configured Redis; setting it alone does not configure authentication on the bundled Redis |
| `REDIS_DB` | `0` | Give each independent collector its own Redis/database |
| `REDIS_MIN_IDLE_CONNECTIONS` | `10` | Template `1` |
| `REDIS_POOL_SIZE` | `100` | Template `4` |
| `REDIS_POOL_TIMEOUT` | `240` seconds | Wait for a Redis pool connection |
| `SERVER_HOST` / `SERVER_PORT` | `127.0.0.1` / `8081` | Compose forces container listener `0.0.0.0:8081` |
| `HTTP_PORT` | `8081` | Compose only: collector localhost port |
| `UI_PORT` | `8080` | Compose only: operator UI localhost port |
| `BIND_ADDR` | `127.0.0.1` | Shared by API/UI publication; leave loopback until an authenticated edge prevents bypass |
| `UI_IMAGE` | `zte-c300-monitoring-ui:dev` | UI image tag; template `local-01` |
| `APP_ENV` | Logging defaults to production | Template production: structured JSON logs |
| `COLLECTOR_IMAGE` | `zte-c300-monitoring:dev` | Compose image; template uses `local-01` |
| `VERSION`, `COMMIT`, `BUILD_TIME` | `dev`, `none`, `unknown` | Build arguments; changing runtime env alone does not change `/version` |
| `REDIS_IMAGE` | `docker.io/library/redis:7.2-alpine` | Pin a tested digest for a published release |

The supplied Compose file always starts its own disposable Redis and a
TimescaleDB volume. Connecting to an external Redis or Postgres requires
adapting those services. Do not publish Redis or Postgres ports just to use
the collector. Redis remains a code dependency even if the poller is off.
The current poller requires a connected history store, and the UI reads that
store. There is no supported “Redis-only” or “history-off UI” mode; keep
both services. See [architecture](architecture.md).

## TimescaleDB and the PON-list poller

| Setting | Code default | Notes |
|---|---|---|
| `TIMESCALEDB_HOST` | Empty (history off) | Compose forces `timescaledb` |
| `TIMESCALEDB_PORT` | `5432` | Internal only |
| `TIMESCALEDB_USER` / `TIMESCALEDB_DB` | `c300` / `c300_monitoring` | Generic application role/database names |
| `TIMESCALEDB_PASSWORD` | Empty in native runs | Blank in template; Compose requires a unique nonempty value |
| `TIMESCALEDB_SSLMODE` | `disable` | Internal Docker network |
| `POLL_ENABLED` | `false` | Template/Compose default `false`; explicitly enable after bounded acceptance |
| `POLL_INTERVAL_SECONDS` | `120` | Minimum 30; sequential cycles, not a per-ONU freshness guarantee |
| `POLL_START_DELAY_SECONDS` | `15` | Lets SNMP/Redis become ready |
| `DEVICE_VENDOR` / `DEVICE_FAMILY` / `DEVICE_ROLE` | `zte` / `c300` / `olt` | Stored with samples for later Huawei/Cisco adapters |

The poller only reads SNMP. HTTP `DELETE .../cache/clear` drops Redis keys, not
ONUs. History: `GET /api/v1/history/samples` (`run=latest`, `status`, `count_by`,
`serial`, `board`, `pon`, `onu_id`, `from`, `to`, `limit`; `count_by` also accepts
`eth_link`), `GET /api/v1/history/runs`, `GET /api/v1/history/status-events`,
`GET /api/v1/history/eth-events` and `GET /api/v1/history/unauth`. The same
paths exist under `/api/v1/olt/{olt_id}/history/...`. Schema upgrades are
additive (`schema_migrations`); existing `onu_samples` are kept. See
[API catalog](api.md).

PON snapshots include RX/TX and Ethernet. Poll interval is a scheduling target;
large/slow cycles can overrun it. No built-in retention policy or retention
setting exists yet. Database size and collection age need operational monitoring.
Changing the database password in `.env` does not rotate an existing PostgreSQL
role: initialized volumes require a coordinated database credential change.

CORS is unused for the Compose UI: the browser talks to nginx on the UI
container, which attaches `X-API-Key`. This does not authenticate browser users.
The current nginx template is HTTP-only and has no Basic Auth; see
[edge access](edge-access.md). CORS settings remain for a future
cross-origin client. CORS does not replace API authentication or a firewall.

Native TLS settings `USE_TLS`, `TLS_CERT_FILE`, `TLS_KEY_FILE` are inherited.
The supplied container healthcheck and guide assume local HTTP behind an SSH
tunnel or a TLS reverse proxy. Do not enable native TLS in this Compose deployment
without also changing and testing its healthcheck and certificate mounts.

## Optional static multi-OLT mode

Configuration source priority: inline `OLTS` JSON, then `OLTS_FILE`, then single
`SNMP_*` values. An invalid configured inventory file fails startup. Changes
require a restart; there is no external inventory polling service.

```dotenv
OLTS='[{"id":"lab-a","host":"192.0.2.10","community":"REPLACE_LOCALLY","boards":"3:16","maxConcurrent":1},{"id":"lab-b","host":"192.0.2.11","community":"REPLACE_LOCALLY","boards":"5:8","maxConcurrent":1,"walk":true}]'
DEFAULT_OLT=lab-a
```

All addresses and credentials above are placeholders. Each entry supports `id`,
`host`, `community`, optional `port` (161), `boards`, `ponsPerBoard` (16),
`maxConcurrent`, `walk` (false) and `user_id` (0). `walk:true` selects GETNEXT
instead of GETBULK for firmware/links that require it. It can increase request count.
Timeouts, retry settings, cache settings and timezone remain shared.

The default device answers `/api/v1/board/...`; explicit selection uses
`/api/v1/olt/lab-a/board/...`. IDs must be unique and use letters, digits, `_`, `-`.
If `DEFAULT_OLT` is absent, the first inventory entry becomes default.

For a private file, store JSON under `private/olts.json`, set
`OLTS_FILE=/run/config/olts.json`, unset `OLTS`, and add this collector mount via a
local `compose.override.yaml` (ignored by Git):

```yaml
services:
  collector:
    volumes:
      - ./private/olts.json:/run/config/olts.json:ro
```

The supplied Compose file still requires nonempty single-device values and
`API_KEY`; keep them configured for the default device even when using `OLTS`.
They are ignored for device selection when an inventory is present. A dedicated
multi-OLT deployment file can relax these template checks later.

## Optional per-user keys

```dotenv
API_USERS='[{"user_id":1,"role":"user","api_key":"REPLACE_USER_KEY"},{"role":"admin","api_key":"REPLACE_ADMIN_KEY"}]'
```

When set, `API_USERS` replaces single-key authentication. Assign a matching
`user_id` to each OLT entry. A user sees only their OLTs; an admin sees all.
Unowned OLTs (`user_id:0`) require an admin. Foreign/unknown OLT requests return
404; missing/invalid keys return 401. These are static keys, not an identity
provider, login UI or automatic key rotation system. Start the trial with one
API key; validate ownership behavior before offering access to other users.

## Cross-origin browser requests

CORS is disabled by default; the Compose UI uses its same-origin nginx proxy.
Only set `CORS_ALLOWED_ORIGINS` to an explicit comma-separated list of trusted
origins if a separate browser client must call the authenticated collector API.
Avoid wildcard origins. CORS is not authentication and does not block non-browser
clients. The UI proxy suppresses CORS allow headers, rejects cross-site fetch
metadata and permits GET/HEAD only; it never offers its injected key to another
website. These controls do not supply the pending browser login/TLS boundary.

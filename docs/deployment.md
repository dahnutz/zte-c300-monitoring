# First deployment on Debian

This guide targets a fresh **Debian 13 (Trixie), amd64** VM. Debian 12 is also
supported by Docker; check the official installation page for other architectures.
The supplied stack has collector, Redis, TimescaleDB and nginx UI services.
It is a controlled acceptance deployment; shared NOC access still needs the
[authenticated TLS edge](edge-access.md). Run the host
commands below as root (or prefix administrative commands with `sudo` when using
another account).

## 1. Prepare the host

- Start with 2 vCPUs, 4 GB RAM and 40 GB disk; allow space for TimescaleDB,
  temporary build layers and logs. Measure actual memory and request latency
  before scaling.
- Install Debian with SSH access, DNS resolution, correct time/NTP and updates.
- Give the VM a stable management address and a route to the test OLT.
- Permit UDP 161 from the collector's actual outbound source address to the OLT,
  with replies allowed. Bridge networking normally uses the host's outbound
  address; confirm the observed source before setting the OLT ACL.
- Configure read-only SNMPv2c access on the OLT using the firmware's supported
  commands. Record populated GPON slots, PON counts and the OLT clock timezone.
- Allow outbound HTTPS/DNS for Debian packages, Go modules and container
  registries while building. Use an image archive if the destination is offline.
- Reserve TCP 8081 (API) and 8080 (UI) on loopback. No public app/database port is required.

Docker access grants substantial host control; use a dedicated administrator.
The commands below follow the [official Docker Debian installation instructions](https://docs.docker.com/engine/install/debian/).
If this is not a fresh host, check that page for conflicting packages first.

```sh
apt update
apt install -y ca-certificates curl git openssl jq snmp nano python3
install -m 0755 -d /etc/apt/keyrings
curl -fsSL https://download.docker.com/linux/debian/gpg -o /etc/apt/keyrings/docker.asc
chmod a+r /etc/apt/keyrings/docker.asc
cat > /etc/apt/sources.list.d/docker.sources <<EOF_DOCKER
Types: deb
URIs: https://download.docker.com/linux/debian
Suites: $(. /etc/os-release && echo "$VERSION_CODENAME")
Components: stable
Architectures: $(dpkg --print-architecture)
Signed-By: /etc/apt/keyrings/docker.asc
EOF_DOCKER
apt update
apt install -y docker-ce docker-ce-cli containerd.io docker-buildx-plugin docker-compose-plugin
systemctl enable --now docker
docker run --rm hello-world
docker compose version
```

Use the modern `docker compose` plugin, not the retired Python `docker-compose`
1.x implementation. No Go compiler, Node.js, PHP or Kubernetes is required on
the host when using the Docker build. TimescaleDB and the UI are started by Compose.

## 2. Place the adapted project on the VM

### Clone from GitHub

On the deployment VM:

```sh
cd /opt
git clone https://github.com/dahnutz/zte-c300-monitoring.git
cd zte-c300-monitoring
# For a tagged deployment: git checkout THE_REVIEWED_RELEASE_TAG
```

Replace `THE_REVIEWED_RELEASE_TAG` with an actual reviewed tag when releases
exist, or use a reviewed commit. This is the adapted repository; the upstream
application has different deployment instructions.

### Alternative: transfer a source archive

On the computer that contains this project, from its directory:

```sh
# List exactly the tracked + nonignored files; review this set before export.
git ls-files --cached --others --exclude-standard -z > /tmp/zte-c300-source-files
# An ignored file that was already tracked is still listed. Review the list:
tr '\0' '\n' < /tmp/zte-c300-source-files
# GNU tar: the NUL list preserves spaces and treats filenames literally.
tar --null --verbatim-files-from --owner=0 --group=0 --numeric-owner \
    -T /tmp/zte-c300-source-files \
    -czf /tmp/zte-c300-monitoring.tar.gz
# Replace DEPLOY_HOST with your VM address.
scp /tmp/zte-c300-monitoring.tar.gz root@DEPLOY_HOST:/tmp/
```

Review the archive listing before transfer; exclusion patterns are not a substitute
for checking files outside the documented private paths. See
[publication review](publication-review.md).

On the VM:

```sh
install -d -m 0750 /opt/zte-c300-monitoring
cd /opt/zte-c300-monitoring
tar -xzf /tmp/zte-c300-monitoring.tar.gz
```


## 3. Configure the first OLT

```sh
cd /opt/zte-c300-monitoring
cp .env.example .env
chmod 600 .env
openssl rand -hex 32
nano .env
```

Fill in:

| Variable | Value to supply |
|---|---|
| `SNMP_HOST` | Management address or resolvable hostname of your test OLT |
| `SNMP_COMMUNITY` | Read-only community, single-quoted if it contains special characters |
| `API_KEY` | The generated random hex value |
| `TIMESCALEDB_PASSWORD` | A separate randomly generated database password; there is no public default |
| `POLL_ENABLED` | Keep `false` until the first-PON checks and load measurement pass |
| `OLT_BOARDS` | Actual physical GPON slots and PON counts, such as `3:16` |
| `OLT_TIMEZONE` | IANA timezone matching the OLT wall clock, or `UTC` if that is what it uses |
| `COLLECTOR_IMAGE` / `VERSION` | A unique candidate name, initially `zte-c300-monitoring:local-01` / `local-01` |
| `UI_IMAGE` | Matching UI tag, initially `zte-c300-monitoring-ui:local-01` |

Keep concurrency 1, prewarm false, and the short example cache TTL for the first
trial. The example topology is not device discovery. See [all settings](configuration.md).

Do not `source .env` as shell code. Docker Compose reads it. `docker compose
config --quiet` validates without printing the resolved credentials; plain
`docker compose config` prints those credentials.

```sh
docker compose config --quiet
```

An optional independent reachability check uses the Debian `snmpget` tool. It is
not required for the application, but helps separate network problems from code.
Read the community interactively to keep it out of shell history (it may still
be briefly visible to local administrators in the process arguments):

```sh
read -r -p 'OLT management address: ' OLT_ADDRESS
read -r -s -p 'Read-only SNMP community: ' OLT_COMMUNITY; echo
snmpget -v2c -c "$OLT_COMMUNITY" -t 3 -r 1 "$OLT_ADDRESS" .1.3.6.1.2.1.1.3.0
unset OLT_COMMUNITY
```

A successful uptime response confirms basic access, not vendor telemetry OIDs.

## 4. Test, build once, then start

```sh
docker build --target test -t zte-c300-monitoring:tests .
./scripts/smoke.sh
BUILD_TIME=$(date -u +%Y-%m-%dT%H:%M:%SZ) docker compose build collector
docker compose build ui
docker compose pull redis timescaledb
docker compose up -d --no-build --wait
docker compose ps
curl --fail http://127.0.0.1:8081/healthz
curl --fail http://127.0.0.1:8081/readyz
curl --fail http://127.0.0.1:8081/version
curl --fail http://127.0.0.1:8080/healthz
# Operator UI (history + Query now + Unconfigured): http://127.0.0.1:8080
docker compose logs --tail=100 collector
```

The smoke test uses synthetic data and separate disposable containers. It does
not read `.env`. The subsequent Compose start connects to your configured OLT. With polling off,
a new history database/UI has no samples; use bounded API reads first. After
acceptance, set `POLL_ENABLED=true` and recreate the collector to fill history.
Do not expect Docker to restart an unhealthy running container automatically;
`restart: unless-stopped` handles process exits and host reboots. Investigate an
unhealthy state through logs and `/readyz`.

`/healthz` proves HTTP liveness. `/readyz` returns 200 when Redis and the default
OLT respond, plus TimescaleDB when registered, or 503 when a critical dependency fails. Probe results are cached
(up to 5 seconds for Redis, 30 seconds for SNMP). An additional OLT can show as
degraded without making the default OLT unavailable. A missing OID can still
produce incomplete data even with readiness 200.

Now follow the [API catalog](api.md) and [hardware acceptance](hardware-testing.md).
Record the image ID and build information with the private test notes:

```sh
docker compose images
curl --fail http://127.0.0.1:8081/version
```

## 5. Access from your workstation

Both UI and API are published on localhost. From your workstation:

```sh
ssh -N -L 8080:127.0.0.1:8080 -L 8081:127.0.0.1:8081 root@DEPLOY_HOST
```

Then open `http://127.0.0.1:8080` for the operator UI (inventory, optical
history, Unconfigured, Query now on ONU detail), or use
`http://127.0.0.1:8081` with the API key. The UI itself has no login today; the
tunnel is the access boundary. For shared access follow the
[nginx/NPM authentication and TLS design](edge-access.md); keep the raw UI/API
ports unreachable around that edge. Do not expose them with `BIND_ADDR=0.0.0.0`
as a shortcut. Health/metrics endpoints have no API key and
can reveal operational details; do not expose the collector directly to the internet.

For installer phones, the planned supported path is the authenticated HTTPS
frontend described in [edge access](edge-access.md), with the
[live field workflow](field-use.md). Query now on ONU detail is a live GET, not
that workflow. Do not give installers a raw API key. Do not represent a stored
history sample as a fresh optical reading.

## 6. Operate, update and roll back

```sh
docker compose logs --tail=100 collector
docker compose logs --tail=100 redis
docker compose restart collector       # restarts the existing container/image
# After editing .env (restart alone does not reload container environment):
docker compose up -d --no-build --force-recreate collector
# Stop this project:
docker compose down
# Start the same tested images again:
docker compose up -d --no-build --wait
```

Logs rotate at 10 MB with three retained files per service. Redis has a 256 MB
cache cap and LRU eviction. It is not published on the host, and stores no durable
history. Its process uses some additional memory beyond that cache allowance.
Back up `.env` and any private inventory files securely, plus the matching source
revision, image identifiers and deployment configuration. No Redis backup is
needed for this baseline. TimescaleDB is durable: back up and test restore of
its database/volume before upgrades. No automatic retention is configured, so
monitor disk use and define retention before continuous operation.

`docker compose down` preserves the named database volume. Do not use `down -v`
unless deliberate deletion of history is intended. A password change in `.env`
alone does not update a database role in an existing volume.

For an update:

1. Retain previous collector/UI images, matching configuration and a restorable
   database backup (`pg_dump` of the Timescale database). Schema upgrades are
   additive (`schema_migrations`); they must not drop `onu_samples`.
2. Obtain/review the new source and run the tests and smoke test.
3. Change `COLLECTOR_IMAGE`, `UI_IMAGE` and `VERSION` to **new** tags (for example `local-02`).
4. Build the changed images once (`docker compose build collector ui`), then
   recreate only the app containers:
   `docker compose up -d --no-build --no-deps --wait collector ui`.
   Do not `down -v`. Leave the Timescale volume running.
5. Check `/version`, `/readyz`, `SELECT MAX(version) FROM schema_migrations`,
   and that `COUNT(*) FROM onu_samples` did not fall.
6. To roll back images, restore the previous tags and run the same `up --no-deps`
   command when the schema is backward-compatible. Otherwise restore the matching
   database backup into a new volume. Do not overwrite old tags or prune them
   before acceptance.

Do not run `up --build` as the normal start procedure. Updates should be deliberate,
with repeatable image identification. Base image tags in this development
Dockerfile can move; record their digests and pin them for the first published release.

### Prebuilt images later

Once release images exist, a build machine/CI should test and publish them. Set
`COLLECTOR_IMAGE` to the actual `registry/owner/image@sha256:...`, explicitly
`docker pull` that reference, and start with `up --no-build`. Compose's collector
pull policy is `never`, so it cannot silently replace an already tested image.
The registry and digest must be real values from your release process.

For offline transfer today, use `docker save -o /tmp/collector.tar
zte-c300-monitoring:local-01`, transfer the archive, and use `docker load -i
/tmp/collector.tar` on a host of the same architecture. Transfer the pinned Redis
and TimescaleDB images and the built UI image too. You still need the matching
Compose file and private configuration.

## Troubleshooting

| Symptom | Check |
|---|---|
| Compose refuses to start | Required blank values in `.env`, modern Compose plugin, local image built |
| `/healthz` works, `/readyz` is 503 | Redis logs, OLT route/ACL/community, UDP replies and DNS inside the container network |
| Uptime works, PON response is empty | Actual slot/PON, populated ONUs, firmware MIB access and index formulas |
| API responds 401 | `X-API-Key`, key changes requiring container recreation, optional `API_USERS` mode |
| API responds 400 | Slot/PON is outside configured boundaries or ONU ID is outside 1–128 |
| Implausible optical power or dates | Raw values/units/timezone; compare with CLI before changing conversions |
| Cold request is slow | PON population, packet loss, retries and GETBULK size; start with one request at a time |
| Port is already used | Set `HTTP_PORT` in `.env`, recreate collector, adjust curl/tunnel port |
| Host SNMP works, container SNMP fails | Container route/NAT source, VPN policy, firewall; host reachability alone is insufficient |
| Tests pass, real telemetry fails | Synthetic test verifies software paths, not device firmware compatibility |
| UI empty after first install | Polling defaults off; complete one-PON acceptance, then explicitly enable it |
| UI shows older inventory | Pages do not auto-refresh; check the displayed run time, poller failures and row-limit warning |
| Database authentication fails after env edit | Existing database role password is not changed by editing container environment |

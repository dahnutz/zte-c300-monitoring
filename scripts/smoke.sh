#!/usr/bin/env bash
# Deploy disposable containers with synthetic SNMP data; never queries a real OLT.
set -euo pipefail
cd "$(dirname "$0")/.."
engine=${CONTAINER_ENGINE:-docker}
image=${COLLECTOR_IMAGE:-zte-c300-monitoring:smoke}
fixture_image=${FIXTURE_IMAGE:-zte-c300-snmp-fixture:smoke}
redis_image=${REDIS_IMAGE:-docker.io/library/redis:7.2-alpine}
run_id="zte-smoke-$$"
state=$(mktemp -d)
cleanup() {
 status=$?
 if [ "$status" -ne 0 ]; then
  cat "$state/ready.json" 2>/dev/null || true
  "$engine" logs "$run_id-collector" 2>/dev/null || true
  "$engine" logs "$run_id-fixture" 2>/dev/null || true
 fi
 "$engine" rm -f "$run_id-collector" "$run_id-fixture" "$run_id-redis" >/dev/null 2>&1 || true
 "$engine" network rm "$run_id" >/dev/null 2>&1 || true
 rm -rf "$state"
}
trap cleanup EXIT
command -v curl >/dev/null
command -v python3 >/dev/null
if [ "${SKIP_BUILD:-0}" != 1 ]; then
 "$engine" build --target runtime -t "$image" .
 "$engine" build --target fixture -t "$fixture_image" .
fi
"$engine" network create "$run_id" >/dev/null
"$engine" run -d --name "$run_id-redis" --network "$run_id" --network-alias redis "$redis_image" redis-server --save '' --appendonly no >/dev/null
"$engine" run -d --name "$run_id-fixture" --network "$run_id" --network-alias fixture "$fixture_image" >/dev/null
"$engine" run -d --name "$run_id-collector" --network "$run_id" --read-only --cap-drop ALL --security-opt no-new-privileges \
 -p 127.0.0.1::8081 -e SNMP_HOST=fixture -e SNMP_PORT=1161 -e SNMP_COMMUNITY=fixture-only \
 -e API_KEY=fixture-api-key -e OLT_BOARDS=3:16 -e CACHE_PREWARM=false -e OLT_TIMEZONE=UTC \
 -e SNMP_MAX_CONCURRENT=1 -e SNMP_RETRIES=0 -e SNMP_TIMEOUT_SECONDS=1 \
 -e REDIS_HOST=redis -e REDIS_MIN_IDLE_CONNECTIONS=1 -e REDIS_POOL_SIZE=4 "$image" >/dev/null
address=$("$engine" port "$run_id-collector" 8081/tcp | head -n 1)
base="http://$address"
for attempt in $(seq 1 30); do
 if curl -fsS "$base/readyz" > "$state/ready.json" 2>/dev/null; then break; fi
 sleep 1
done
curl -fsS "$base/readyz" > "$state/ready.json"
"$engine" exec "$run_id-collector" /zte-c300-monitoring healthcheck
[ "$(curl -s -o /dev/null -w '%{http_code}' "$base/api/v1/board/3/pon/1")" = 401 ]
[ "$(curl -s -o /dev/null -w '%{http_code}' -H 'X-API-Key: fixture-api-key' "$base/api/v1/board/2/pon/1")" = 400 ]
for endpoint in 'board/3/pon/1' 'board/3/pon/1/onu/1' uplinks; do
 case "$endpoint" in */onu/1) output=detail;; uplinks) output=uplinks;; *) output=list;; esac
 curl -fsS -H 'X-API-Key: fixture-api-key' "$base/api/v1/$endpoint" > "$state/$output.json"
done
python3 - "$state" <<'PY'
import json,sys,pathlib
p=pathlib.Path(sys.argv[1])
rows=json.loads((p/'list.json').read_text())['data']
assert len(rows)==1, rows
onu=rows[0]
assert onu['onu_id']==1 and onu['name']=='TEST-ONU', onu
assert onu['status']=='Online' and onu['rx_power']=='-20.00', onu
raw=json.loads((p/'detail.json').read_text())['data']
detail=raw[0] if isinstance(raw,list) else raw
assert detail['onu_id']==1 and detail['onu_type']=='SYNTHETIC-ONU',detail
assert detail['tx_power']=='2.00',detail
eth=detail['ethernet']
assert eth['status']=='ok' and len(eth['ports'])==1,eth
port=eth['ports'][0]
assert port['admin_state']=='enabled' and port['link_state']=='up',port
assert port['speed_mbps']==1000 and port['duplex']=='full',port
uplinks=json.loads((p/'uplinks.json').read_text())['data']
assert uplinks['ports'][0]['speed_mbps']==10000,uplinks
assert len(uplinks['cards'])==1,uplinks
print('PASS: readiness, API authentication, topology validation, ONU list/detail, uplink/card data')
PY
# A cached list must still work while the synthetic OLT is stopped.
"$engine" stop "$run_id-fixture" >/dev/null
curl -fsS -H 'X-API-Key: fixture-api-key' "$base/api/v1/board/3/pon/1" > "$state/cached.json"
cmp "$state/list.json" "$state/cached.json"
echo 'PASS: cached ONU list survives OLT unavailability; temporary containers will be removed'

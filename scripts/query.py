#!/usr/bin/env python3
"""GET a collector API path. Query parameters are KEY=VALUE arguments so the
shell does not treat & as a background job.

    python3 scripts/query.py /api/v1/board/3/pon/1
    python3 scripts/query.py /api/v1/history/runs limit=5
    python3 scripts/query.py /api/v1/history/samples run=latest status=Offline
    python3 scripts/query.py /api/v1/history/samples run=latest count_by=status
    python3 scripts/query.py /api/v1/history/status-events serial=ZTEG00000001
    python3 scripts/query.py /api/v1/history/eth-events serial=ZTEG00000001 port=1
    python3 scripts/query.py /api/v1/history/unauth
    python3 scripts/query.py '/api/v1/history/samples?run=latest&status=Offline'
"""
import json
import sys
import urllib.error
import urllib.parse
import urllib.request
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
DEFAULT_BASE = "http://127.0.0.1:8081"

USAGE = """Usage:
  python3 scripts/query.py /path [KEY=VALUE ...]
  python3 scripts/query.py '/path?already=quoted'

Reads API_KEY from the repo .env. Do not pass unquoted ?a=1&b=2 — bash treats
& as a background job. Put each filter as its own KEY=VALUE argument instead.

Examples:
  python3 scripts/query.py /api/v1/board/3/pon/1
  python3 scripts/query.py /api/v1/history/runs limit=5
  python3 scripts/query.py /api/v1/history/samples run=latest status=Offline
  python3 scripts/query.py /api/v1/history/samples run=latest count_by=status
  python3 scripts/query.py /api/v1/history/status-events serial=ZTEG00000001
  python3 scripts/query.py /api/v1/history/unauth
"""


def load_dotenv(path):
    values = {}
    if not path.is_file():
        return values
    for raw in path.read_text().splitlines():
        line = raw.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, value = line.split("=", 1)
        values[key.strip()] = value.strip().strip("'\"")
    return values


def endpoint_from_args(argv):
    if len(argv) < 2 or argv[1] in ("-h", "--help") or not argv[1].startswith("/"):
        return None
    parsed = urllib.parse.urlsplit(argv[1])
    query = dict(urllib.parse.parse_qsl(parsed.query, keep_blank_values=True))
    for item in argv[2:]:
        if item.startswith("-") or "=" not in item:
            return None
        key, value = item.split("=", 1)
        key = key.strip()
        if not key:
            return None
        query[key] = value
    return urllib.parse.urlunsplit(
        ("", "", parsed.path, urllib.parse.urlencode(query), parsed.fragment)
    )


def query(base, api_key, endpoint):
    req = urllib.request.Request(
        base.rstrip("/") + endpoint,
        headers={"X-API-Key": api_key},
    )
    with urllib.request.urlopen(req, timeout=95) as response:
        return json.load(response)


def main(argv):
    endpoint = endpoint_from_args(argv)
    if endpoint is None:
        print(USAGE, end="", file=sys.stderr)
        return 2
    env = load_dotenv(ROOT / ".env")
    api_key = env.get("API_KEY", "")
    if not api_key:
        print("API_KEY missing from .env", file=sys.stderr)
        return 1
    try:
        print(json.dumps(query(DEFAULT_BASE, api_key, endpoint), indent=2))
    except urllib.error.HTTPError as error:
        print(f"HTTP {error.code}: {error.reason}", file=sys.stderr)
        body = error.read().decode("utf-8", errors="replace")
        try:
            print(json.dumps(json.loads(body), indent=2), file=sys.stderr)
        except json.JSONDecodeError:
            print(body, file=sys.stderr)
        return 1
    except (urllib.error.URLError, TimeoutError, OSError) as error:
        print(f"API connection failed: {error}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv))

#!/bin/sh
set -eu
if [ -z "${API_KEY:-}" ]; then
  echo "API_KEY is required so the UI can call the collector" >&2
  exit 1
fi
envsubst '${API_KEY}' < /etc/nginx/templates/default.conf.template > /tmp/default.conf
exec nginx -g 'daemon off;'

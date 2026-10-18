#!/usr/bin/env bash
set -euo pipefail
image=${1:?Usage: smoke-test.sh IMAGE}
container=""
trap 'if [[ -n "$container" ]]; then docker rm -f -v "$container" >/dev/null; fi' EXIT

docker run --rm --entrypoint /opt/factorio/bin/x64/factorio "$image" --version | grep -F 'Version: 2.1.22'
container=$(docker run -d -p 127.0.0.1::80 "$image")
port=$(docker port "$container" 80/tcp)
ready=false
for _attempt in {1..30}; do
  if curl --fail --silent "http://$port/login" >/dev/null; then
    ready=true
    break
  fi
  sleep 1
done
[[ "$ready" == true ]] || { echo 'Manager did not become ready'; exit 1; }
curl --fail --silent "http://$port/ups-config.js" | grep -F 'window.fsmUPS'
docker exec "$container" test -s /opt/fsm-data/sqlite.db
docker exec "$container" test -s /opt/fsm-data/bootstrap-admin-password
if docker top "$container" -eo args | grep -F '/opt/factorio/bin/x64/factorio'; then
  echo 'Game unexpectedly autostarted'
  exit 1
fi
docker stop --time 30 "$container" >/dev/null
[[ $(docker inspect --format '{{.State.ExitCode}}' "$container") == 0 ]]
echo 'Manager startup, bundled version, UI, UPS configuration and shutdown passed'

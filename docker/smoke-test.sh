#!/usr/bin/env bash
set -euo pipefail
image=${1:?Usage: smoke-test.sh IMAGE [EXPECTED_VERSION]}
script_dir=$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)
expected_version=${2:-$(sed -n 's/^ARG FACTORIO_VERSION=//p' "$script_dir/Dockerfile.standalone")}
[[ "$expected_version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo 'Invalid expected Factorio version'; exit 1; }
container=""
trap 'if [[ -n "$container" ]]; then docker rm -f -v "$container" >/dev/null; fi' EXIT

docker run --rm --entrypoint /opt/factorio/bin/x64/factorio "$image" --version | awk -v expected="$expected_version" '$1 == "Version:" && $2 == expected { found = 1 } END { exit !found }'
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

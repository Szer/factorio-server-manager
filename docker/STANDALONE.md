# Standalone amd64 manager

`Dockerfile.standalone` bundles Factorio for Linux amd64, the startup mod
settings editor and five minutes of UPS history. The game starts from the UI.

```sh
podman build --platform linux/amd64 -f docker/Dockerfile.standalone -t factorio-manager:local .
```

To upgrade the bundled game, change only `ARG FACTORIO_VERSION` in
`docker/Dockerfile.standalone`. The download, image version label and CI smoke test
use that value. Merging to `master` builds, tests and publishes the updated image.
For a local override, pass `--build-arg FACTORIO_VERSION=<version>` and supply the
same version as the second argument to `docker/smoke-test.sh IMAGE <version>`.

Mount `/opt/fsm-data`, `/opt/factorio/saves`, `/opt/factorio/mods` and
`/opt/factorio/config` persistently. The entrypoint preserves accounts, credentials,
mods and saves, and backs up the original manager JSON before updating container
paths. Back up all four directories while stopped before replacing an image.

Fresh installations create an `admin` account and store its generated password in
`/opt/fsm-data/bootstrap-admin-password`. Existing accounts retain their passwords.
`ADMIN_USER`, `ADMIN_PASS`, `COOKIE_ENCRYPTION_KEY` and `FACTORIO_VERSION` environment
variables are not configuration inputs. The bundled game version is fixed.

The manager samples localhost RCON every ten seconds without Prometheus or Beszel.
Actual UPS is tick delta divided by monotonic elapsed time; target UPS is game
speed times 60. Pauses, reconnects, speed changes and game restarts leave gaps.
History stays in memory for five minutes and clears when the manager restarts.
An explicit `prometheus_url` still selects the external telemetry backend.
`UPS_BACKENDS` enables ordered backend fallback and best-effort multi-backend writes.
See [UPS backends](../docs/UPS.md) for environment settings and Beszel provisioning.

Publish HTTP port 80 through the existing authenticated access path and game UDP
34197 as needed. RCON needs no published host port. Give the container a two-minute
stop grace period so the manager can save and stop the game on SIGTERM.

## CI images

Pull requests run Go race tests on the minimum and image toolchains, build the
frontend and amd64 image, and smoke-test fresh manager startup and shutdown with
the game stopped. No Factorio account or production data is needed.

Successful builds on `master` publish the tested image to
`ghcr.io/szer/factorio-server-manager:sha-<full-commit-sha>` and update `:master`.
The workflow summary includes the digest for Dockge pinning. Manual workflow runs
publish only when run on `master`. Publishing uses the repository's `GITHUB_TOKEN`;
the existing GHCR package must grant this repository Actions write access.

Use the digest from a successful run in Dockge's `image` field, preserving all
volumes and configuration. Stop the game before recreating the manager. The new
container leaves the game stopped until explicitly started in the UI. Rollback
uses the previous image digest with the same persistent mounts.

## Settings diagnostics

`FSM_LOG_LEVEL` accepts `info` (default, existing logs) or `debug`. In Compose:

```yaml
environment:
  - FSM_LOG_LEVEL=debug
```

Debug logs show settings reads/writes, their source, resolved paths, process UID/GID,
file and parent directory ownership/modes, and successful byte/field counts.
Settings values, passwords and tokens are not logged by these diagnostics.

FSM reads `server-settings.json` at manager startup; the settings UI reads its
in-memory snapshot. Starting the game writes that snapshot back to disk.
Write failures are always logged, failed API saves return an error to the UI,
and a failed startup settings write prevents the game from launching.

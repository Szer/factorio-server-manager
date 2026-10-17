# Standalone amd64 manager

`Dockerfile.standalone` bundles Factorio 2.1.22 for Linux amd64, the startup mod
settings editor and five minutes of UPS history. The game starts from the UI.

```sh
podman build --platform linux/amd64 -f docker/Dockerfile.standalone -t factorio-manager:2.1.22 .
```

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

Publish HTTP port 80 through the existing authenticated access path and game UDP
34197 as needed. RCON needs no published host port. Give the container a two-minute
stop grace period so the manager can save and stop the game on SIGTERM.

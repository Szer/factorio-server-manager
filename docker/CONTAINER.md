# Container runtime

`Dockerfile.multiarch` builds the Go manager for `TARGETARCH` and the browser UI
on `BUILDPLATFORM`. `GAME_IMAGE` supplies an optional matching-architecture game
runtime; Factorio is not downloaded when the container starts.

Pass a writable manager configuration with `--conf` and explicit game paths.
`factorio_extra_args` is a JSON array appended to game startup arguments, suitable
for whitelist, ban-list and server-identity options. The configured `config_file`
and `mods_dir` are passed explicitly to Factorio, including save creation.

The manager handles SIGTERM and SIGINT by closing its HTTP listener, interrupting
the game, and waiting up to 110 seconds for saving and exit. Give the container
at least 120 seconds to terminate. Game autostart remains opt-in.

On an empty account database, `FSM_ADMIN_PASSWORD_FILE` supplies an initial admin
password of at least 12 characters. Without it, a cryptographically random password
is written to `bootstrap-admin-password` beside the manager configuration with mode
0600. Passwords and server settings are not logged. Change the bootstrap password
in the UI after first login; the bootstrap file is not a password-reset mechanism.

## Startup settings editor

Mods → Startup settings edits values already stored in `mods/mod-settings.dat`.
Booleans, numbers, strings, exact 64-bit integers and RGBA colors are supported;
unknown compound values remain read-only. The editor validates storage types and
color ranges; Factorio applies mod-specific limits when loading. It does not
extract defaults or allowed choices from Lua setting prototypes.

Saving requires a stopped server, serializes against game startup and mod uploads,
and rejects a stale file revision. Only edited startup value bytes are replaced;
runtime sections and the version header are preserved. Original files are backed
up by SHA-256 under `mods/.startup-settings-backups` before atomic replacement.
To restore one, stop the game, copy the desired backup locally as
`mod-settings.dat`, and use Mods → Upload Mod.

## UPS history

The Controls page shows actual/target UPS, players and the last five minutes of
history, refreshed every ten seconds. Configure `prometheus_url` and optional
`telemetry_labels` in the manager JSON to select one Factorio exporter. The
exporter must provide `factorio_ups`, `factorio_target_ups`, `factorio_players`,
`factorio_paused`, `factorio_rcon_up` and `factorio_sample_timestamp_seconds`.

The authenticated `/api/server/ups` endpoint issues a fixed, bounded range query;
browsers do not connect directly to Prometheus. Paused, disconnected, stale and
missing samples leave gaps. The average includes only valid active samples.

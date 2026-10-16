# Mod settings and UPS monitoring

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

UPS monitoring is optional. Without `prometheus_url`, the chart reports that
telemetry is not configured. A compatible exporter and Prometheus must be supplied
separately; the manager does not collect game metrics itself.

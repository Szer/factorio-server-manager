# UPS backends

The manager samples Factorio through local RCON and writes each sample independently
to every backend in `UPS_BACKENDS`. Reads try backends in the listed order and return
the first fresh history. The browser uses the same manager API for every backend;
backend credentials never reach the browser.

```yaml
environment:
  UPS_BACKENDS: beszel,prometheus,local
  UPS_SERVER_ID: factorio
  UPS_BESZEL_URL: http://beszel:8090
  UPS_BESZEL_TOKEN: ${UPS_BESZEL_TOKEN}
  UPS_PROMETHEUS_URL: http://prometheus:9090
```

Valid backend names are `local`, `prometheus`, and `beszel`. Whitespace and duplicate
names are ignored. Set a unique `UPS_SERVER_ID` for each manager. Changing the list
requires restarting the manager. No history is copied between backends when switching.
The game does not depend on any telemetry backend being available.

With `UPS_BACKENDS` unset, an existing `prometheus_url` configuration (or
`UPS_PROMETHEUS_URL`) retains external-exporter, read-only operation. Otherwise the
default is `local`. Explicitly listing `prometheus` enables RCON sampling and writes.
Invalid numeric/duration values log a warning and use their defaults; unknown backend
names are skipped. An entirely invalid list logs a warning and uses `local`.

## Failure behavior

Each external backend has one write worker and a bounded queue. Failed writes are
not retried; new samples continue independently. A full queue drops new samples.
This is best-effort telemetry, not durable delivery. `local` retains a bounded memory
history and is useful as the last fallback; it clears on manager restart.

Read failures, malformed responses, and stale or empty histories advance to the next
backend. Fresh paused/disconnected samples are valid data, not backend failures.
After consecutive failures, a backend's read circuit stays open for the cooldown.
The next request after cooldown probes it; success restores its original priority.
Reads to each backend are serialized, including recovery probes. Each attempt gets
at most its read timeout and an equal share of the remaining total budget, reserving
time for fallback. Caller cancellation
does not count as backend failure. Write health is independent of the read circuit.

Logs report sampler state, write failures/recovery, queue overflow, read failures,
circuit opening/recovery, and source changes. Repeated events, including flapping,
are rate-limited per event/backend. Logs omit URLs, tokens and response bodies.

## Environment settings

Durations use Go syntax, for example `750ms`, `3s`, `5m`, or `24h`. Sampling uses
whole seconds between 1s and 1m. The chart follows the configured history window
and polling interval. Its request timeout includes the server read budget plus
one read timeout as transport margin. Public timings load asynchronously without
blocking the app; the chart uses 10s polling and a 13s request timeout until loaded.

| Variable | Default | Purpose |
| --- | --- | --- |
| `UPS_BACKENDS` | `local`, or legacy Prometheus | Write destinations and read priority |
| `UPS_SERVER_ID` | `factorio` | Beszel record owner and default Prometheus `server` label |
| `UPS_SAMPLE_INTERVAL` | `10s` | RCON sampling and UI polling |
| `UPS_PROBE_TIMEOUT` | `3s` | RCON request deadline |
| `UPS_READ_TIMEOUT` | `3s` | Deadline for each backend read, including pagination |
| `UPS_READ_TOTAL_TIMEOUT` | `10s` | Entire fallback chain and waiting for concurrent reads |
| `UPS_WRITE_TIMEOUT` | `3s` | Each external sample write, including Beszel cleanup |
| `UPS_HISTORY_WINDOW` | `5m` | Requested history, up to 24h |
| `UPS_FRESHNESS` | 3 × sampling interval | Maximum sample age; also prevents rates spanning long sampling gaps |
| `UPS_CIRCUIT_FAILURES` | `3` | Consecutive read failures before opening the circuit |
| `UPS_CIRCUIT_COOLDOWN` | `1m` | Wait before the next read probe |
| `UPS_WRITE_QUEUE_SIZE` | `6` | Pending samples per external backend |
| `UPS_LOG_INTERVAL` | `5m` | Minimum interval for repeated log events |
| `UPS_PROMETHEUS_URL` | Existing `prometheus_url` | Prometheus query base URL |
| `UPS_PROMETHEUS_WRITE_URL` | Query URL + `/api/v1/write` | Native remote-write receiver URL |
| `UPS_PROMETHEUS_TOKEN` | Empty | Complete Authorization header, e.g. `Bearer …` |
| `UPS_PROMETHEUS_LABELS` | Existing `telemetry_labels` | JSON object of query/write labels |
| `UPS_BESZEL_URL` | Empty | Beszel/PocketBase base URL |
| `UPS_BESZEL_TOKEN` | Empty | Restricted PocketBase record API auth token |
| `UPS_BESZEL_COLLECTION` | `factorio_ups` | Preprovisioned custom collection |
| `UPS_BESZEL_RETENTION` | `24h` | Age after which custom records may be deleted; at least the history window |
| `UPS_BESZEL_CLEANUP_INTERVAL` | `1m` | Cleanup cadence when caught up |

HTTP redirects are rejected. All network operations use their operation deadline.
Bounded queues, response sizes, pagination, and history storage limit resource use.
A malformed backend configuration affects telemetry, not the game process.

## Prometheus

Enable the receiver with `--web.enable-remote-write-receiver`. Writes use the native
remote-write 1.0 protobuf/Snappy format through the official Prometheus protobuf
package. This dependency requires Go 1.22 or newer to build the manager.

The adapter writes `factorio_ups`, `factorio_target_ups`, `factorio_players`,
`factorio_paused`, `factorio_rcon_up`, and `factorio_sample_timestamp_seconds`.
Missing values are NaN so a prior active UPS cannot leak across pauses or disconnects.
Original sample timestamps determine freshness, not query evaluation timestamps.

Explicit multi-backend mode adds `server=UPS_SERVER_ID` unless a `server` label is
provided in `UPS_PROMETHEUS_LABELS`. Give this writer distinct labels or remove a
competing exporter scrape; two producers must not write the same time series.
Prometheus owns retention. Authentication is optional and supports a complete
Authorization header supplied through the environment.

## Beszel / PocketBase setup

This adapter uses a dedicated PocketBase collection inside Beszel. It does not add
charts to Beszel, change its agent, or modify Beszel's built-in collections. Beszel's
standard aggregation, retention and alerts do not apply to this custom collection.
An administrator must provision it; the manager does not receive superuser credentials.

Create a **base collection** named `factorio_ups` with these fields:

| Field | Type | Required |
| --- | --- | --- |
| `server` | Text | Yes |
| `sample_time` | Number, integer | Yes |
| `point` | JSON | Yes |

Add a unique index on `(server, sample_time)` and an index on `sample_time`.
For the default collection name, the SQL index definitions are:

```sql
CREATE UNIQUE INDEX idx_factorio_ups_server_time ON factorio_ups (server, sample_time);
CREATE INDEX idx_factorio_ups_time ON factorio_ups (sample_time);
```

Use a dedicated restricted account/auth collection for the manager. For example,
create an auth collection `ups_writers` with a required `server` text field; its
account-management rules should remain locked to administrators. Give the manager's
account the same `server` value as `UPS_SERVER_ID`. On `factorio_ups`, set:

- List/view/delete: `@request.auth.collectionName = "ups_writers" && server = @request.auth.server`
- Create: `@request.auth.collectionName = "ups_writers" && @request.body.server = @request.auth.server`
- Update: locked (null).

Create rules must enforce ownership; query filters alone are not authorization.
Keep `ups_writers` account creation, update and management locked so the service
account cannot change its own server assignment. Obtain its normal PocketBase auth
token and supply it as `UPS_BESZEL_TOKEN`. Expired/revoked tokens cause logged 401
failures and fallback. Token refresh is manual: rotate the environment value and
restart. Never use a PocketBase superuser impersonation token for routine telemetry.

The writer deletes only this server's expired records in batches of at most 100,
within its write deadline. A full batch retries cleanup on the next sample instead
of waiting for the interval. Failed cleanup reports `sample stored; retention failed`
and retries on the next sample. Cleanup requires the manager to be running and able
to write; retention is best-effort and can temporarily exceed the configured age.

Provision and test this collection against the installed Beszel/PocketBase version
before enabling it. Keep a backup before upgrading Beszel: custom collection support
uses PocketBase's API, not a stable Beszel custom-metrics contract.

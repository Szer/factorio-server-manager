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

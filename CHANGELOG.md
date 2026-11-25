# Changelog

All notable changes are listed here, newest first ([Keep a Changelog](https://keepachangelog.com),
[Semantic Versioning](https://semver.org)).

## [Unreleased]

### Added
- USAGE §11: karga-server logs each operation's end; failed ones at warning level.
- `karga reconcile --empty-volumes`: create a release's volumes that are missing in Docker, empty
  (karga-server no longer does on its own repairs); USAGE §11.
- `karga volume undo-restore <release> <volume>`: put back the files a volume had before its last
  restore (its restore point).
- `volume restore --merge` (write over the files, as before) and `--no-restore-point`. Restore and
  undo-restore show their progress, and volume commands say which workloads were down and for how
  long; `events` shows it too (`down 4.2s`).
- USAGE §5: image pulls show their progress (karga-server).
- USAGE: exposure `pathType` (prefix, by segment, or exact) and `to` (karga-server D43).
- `karga gateway edit`: the settings in force in `$KARGA_EDITOR`/`$EDITOR`; a document with
  problems opens again with them on top; saving it unchanged gives up and keeps the file.
- The gateway settings as a document you keep (kind `Gateway`): `karga gateway get`, `diff -f`,
  `apply -f [--yes]`, `history` and `rollback [revision]`; `gateway status` shows the revision in
  force.
- `karga gateway listener add` takes `--timeout`, `--idle-timeout`, `--max-body`, `--rate-limit` and
  `--proxy-protocol`; `karga gateway listener set <name>` changes them (`0` or `off` removes one);
  `listener list` shows them in OPTIONS.

### Changed
- USAGE §8 rewritten for the new volume backup and restore (what stops, `--live`, replace and `--merge`,
  restore points, interruptions); §2 shows API 4.0, §11 and §16 the new errors and limits.
- API 4.0 (karga-sdk): `volume backup` stops the workloads writing to the volume by default; `--live`
  copies while they run (`--stop` is gone). `volume restore` replaces the volume's files and keeps
  the old ones as a restore point. A backup whose workloads did not get ready again is kept, and the
  error says so.
- USAGE: the workload's health check is the only one; a `tcp` check's empty connection is punished
  by sshd and fail2ban, so those need `http` or `exec` (karga-server D44).
- USAGE: the gateway reaches release proxies on the `karga-edge` network; proxies publish no port
  (karga-server D42).
- Gateway listeners `http` and `https` instead of `web` and `websecure` in `karga create`, the
  examples and USAGE; `listener add|set --redirect-https` (karga-server D41).
- USAGE: the gateway is `karga-gateway-<hash>` and changes image without an outage
  (karga-server D38).
- USAGE: the release proxy is `<release>-karga-proxy-<hash>` and is replaced without an outage
  (karga-server D37); the plan shows `PROXY proxy (replace)`.
- USAGE: a recreate creates the new container before stopping the old one, and adding a Service to
  a workload no longer recreates it (karga-server D36).
- Workload containers are named after their digest (`demo-web-3f9a2c1b`, karga-server D34); USAGE's
  examples follow, and say how to find one by label.
- `karga status` exits 6 when a Service is routed to no backend (`not routed`, now shown in red).
- `-o json`/`-o yaml` print an empty list as `[]` in every command (some printed `null`);
  `karga logs` refuses them instead of ignoring them.
- Exit codes: an unknown subcommand of a group (`karga gateway nope`) exits 2 instead of showing
  the help with 0, as do an invalid `--strategy` or `--log-format` and a request karga-server
  refuses as invalid. Ctrl-C during an operation exits 7, with a hint to follow it. `karga status`
  and `karga lint` no longer print an `Error:` line after the report that already says what is
  wrong.
- The questions before commands that delete data or expose services come from the SDK, the same
  rule karga-server checks. A failed read of the answer is an error instead of a "no";
  `gateway set listen` refuses an address that is not an IP.
- API 3.0 (karga-sdk): this karga works with karga-server of API 3 only; `karga version` shows
  both. Release names that are not DNS labels, or that start with `karga`, are refused by every
  command, not only `install`; USAGE §1.
- Template loops are limited to 1,000,000 iterations per render.
- `--set` and `--set-string` apply in the order given, and `--set a[1]=x` edits the default list
  instead of replacing it with `[null, x]`; USAGE §4.
- Networks of a package are `bridge` and Volumes `local` (karga-sdk): other drivers are refused.
- `karga lint` prints warnings (`[WARNING]`, `warnings` in JSON) that do not fail it: a variable
  set from a Secret is visible to anyone who can inspect the container; USAGE §9.
- `karga history` shows `superseded` for deployed revisions a later one replaced (karga-server
  with this change); USAGE §5.

### Added
- `karga template --revision N` renders as revision N (`.Release.Revision`); its help says that
  `upgrade --dry-run -o yaml` shows what an upgrade would deploy.
- `make dist` builds the release archives and `checksums.txt` without a release; `install.sh`
  installs them from `file://`. USAGE: requirements, installing from source, and a new section 15
  that puts an API and its database online on a domain.
- `lint` and `template` know `environmentFrom` (workload variables from Secrets); USAGE §9.

### Fixed
- `gateway apply`, `rollback` and `edit` with `-o json` wrote two JSON documents (the plan, then the
  revision), which `jq` and other parsers refuse; they write one, `{plan, revision}`. USAGE §6.
- `get values` printed large whole numbers as floats (`1e+06` in YAML), which an upgrade from that
  file could take wrongly or refuse (`type: integer`); they stay integers (karga-sdk).
- `karga gateway edit` was bounded by `--request-timeout`: after 30 seconds the editor was killed
  and the edit lost. It, `gateway apply`, `rollback`, `set listen`, `listener remove` and `volume
  undo-restore` are not bounded now: the clock ran while they asked for a confirmation too.
- Exit codes match USAGE §11: Ctrl-C once a request went out exits 7 (was 1), and karga-server going
  away during a log, a backup's download, an exec or an upload exits 3 (was 1, `unexpected EOF`)
  (karga-sdk). `karga reconcile` exits 1 when a repair failed (was 0).
- USAGE §11: a repair needed again within 10 minutes counts as failed, and karga tries again 10
  minutes after giving up (karga-server).
- USAGE §6: the gateway's `x-forwarded-for` holds only the client address now, request headers
  have 10 seconds, idle connections an hour, https TLS 1.2 or newer (karga-server); `--max-body`
  notes the gateway's memory.
- USAGE error table: a backup needs room under the server's `KARGA_HOME` for its copy.
- `karga list` failed for every release when one release's state file was damaged; it shows that
  one as `unreadable` (red, without a date) now (karga-server); USAGE error table.
- `volume restore` of a backup cut short emptied the volume and started the workloads on what it
  had written; a restore that failed writing (Docker, a full disk) started them on an empty or half
  written volume. The backup is read to its end before anything changes, and a failed write leaves
  the workloads stopped and the release paused (karga-server); USAGE §8.
- `volume backup` stopped mid-download (Ctrl-C, a timeout) kept the part it had and said `written`
  with exit 0; it removes the file and exits 7 now (karga-sdk). Stopped mid-copy, the workloads it
  stopped start again (karga-server); USAGE §2.
- A user outside the `karga` group got `karga-server is not running` (exit 3) though it ran; it is
  `permission denied on /run/karga/karga.sock` (exit 1) now (karga-sdk).
- USAGE §13 and README: files in `templates/` whose name starts with `.` are ignored (karga-sdk);
  a macOS `._*` file failed `lint` with `yaml: control characters are not allowed`.
- Times in `list`, `history`, `status`, `events` and `gateway history` had no zone, while the
  gateway's access log is UTC; they show their offset now (`2026-11-17 19:01:09 +03:00`).
- `history` put a failed revision's whole error in DESCRIPTION (one row ~300 columns wide); the
  table cuts it at 60 characters, as `events` cuts WHAT at 100. `-o json` and `-o yaml` keep it whole.
- `--verbose` plans and `karga diff` showed an added field as `path: value`, like nothing changed;
  it is `path: (added) -> value` now.
- USAGE §6: a request shows up in the gateway access log within about a second (karga-server), and
  its times are UTC.
- README and USAGE said a failed recreate left the previous revision serving; its container was
  replaced, so the workload is down until the rollback the error names (karga-server).

## [0.1.1] - 2026-11-18

First release (the v0.1.0 tag has no release: its release workflow failed).

### Repositories
- karga is split into three repositories with their history: the karga CLI (this one),
  karga-server (`github.com/hasankaradmir/karga-server`, the server, its install script and the
  design documents) and karga-sdk (`github.com/hasankaradmir/karga-sdk`, the package engine, the
  API and the example packages). Each installs with a public script, without credentials.
- The CLI's tests run against a fake karga-server; the server's tests cover what it does.

### Changed
- `karga state backup` is gone: karga-server keeps its state in files, and a backup is a copy of
  its `$KARGA_HOME` (USAGE.md, "Back up and move a host").
- The module is `github.com/hasankaradmir/karga-cli` (the repository is karga-cli); the binary is
  still `karga`.
- API 2.0: works with any karga-server of the same major API version. A request an older server
  does not understand (an unknown field or endpoint) exits with 3 and says to upgrade it.
- `-o json` and `-o yaml` are the API's views and stable within an API major version.
- Managing a server from another machine is `ssh host karga …`; forwarding its socket is not supported.
- `-o text|json|yaml` on every command (`table` is an older name for `text`); `status --watch` and
  `events -f` stream one JSON document per line, or YAML documents.
- Exit codes tell scripts why karga failed: 2 usage, 3 server unavailable or incompatible, 4 not
  found, 5 conflict, 6 `karga status` not healthy (see USAGE.md).
- `uninstall`, `volume restore`, `gateway set listen` (beyond loopback), `gateway listener remove`
  and `upgrade`/`rollback --delete-volumes` ask first; in scripts they need `--yes`.
- `--request-timeout` (default 30s) bounds commands that only ask karga-server something.
- `uninstall --dry-run`. `get values|manifest|notes <release> [revision]` take the revision as an
  argument instead of `--revision`. `volume restore --from-other-volume` replaces `--force`.
- `--timeout` and `--history-max` are sent only when given: karga-server's defaults apply.
- `karga template` masks writeOnly values wherever they occur, and takes `-o json|yaml`.
- `history --diff` asks karga-server for the differences.
- `karga version` says why it cannot reach the server (not running, permission denied) and exits
  with 3 when the CLI and the server do not work together.

### Release
- Built with Go 1.27.4 (net/http fix GO-2026-6859); CI runs govulncheck 1.10.0.
- Releases carry an SBOM per archive and a keyless (Sigstore) signature of `checksums.txt`;
  `install.sh` verifies it when `cosign` is installed (`KARGA_SKIP_SIGNATURE=1` skips it).
- The release workflow runs the tests and the linter before it publishes; CI runs `govulncheck`, and
  every GitHub Action is pinned by commit.

### Fixed
- `exec -t` without `-i` always failed.
- `events` said newest first but printed oldest first; long messages are cut at a character.
- Status colors covered states that do not exist and missed upgrading, uninstalling and aborted;
  only state columns are colored now.
- A second Ctrl-C ends karga at once.
- Released binaries report `v0.1.0` like local builds.

### Breaking (compared with earlier builds)
- Workloads no longer expose ports: `hostPort`, `hostIP` and `routes` are gone from the Workload
  (a clear error points to Services). Ports are named, and a `kind: Service` gives a workload a name
  inside its release and exposes it on gateway listeners.
- The gateway's listen addresses and listeners belong to the host's administrator: `karga gateway
  listener add|remove|list`, `karga gateway set listen` (default `127.0.0.1`). Whoever used
  `hostIP: 0.0.0.0` sets `karga gateway set listen 0.0.0.0`.
- karga-server owns the state and the runtime; the karga CLI is its client on
  `/run/karga/karga.sock` and stops with `karga-server is not running` without it (`create`,
  `lint`, `template`, `show` work alone). The service runs as the `karga` user with its state in
  `/var/lib/karga`; karga-server's install script sets up the service. The CLI's `--home` is gone (`--socket` instead).
- `dependsOn` on a workload: it starts only once the Services it names are ready behind the release
  proxy; a failed dependency keeps it from starting.
- karga-server repairs releases on its own (Docker events and every 30 seconds): removed or
  hand-changed containers are recreated, stopped ones started, unhealthy ones restarted, without a
  new revision. `karga reconcile`, `karga pause|resume` and `karga events`; `karga status` shows the
  last repair and what it gave up on.
- Operations go on when the CLI is interrupted (Ctrl-C, a lost connection); the CLI says how to
  follow them. A karga-server that is killed mid-operation finishes it when it starts again
  (`resume` in `karga events`).
- The CLI and karga-server check that they speak the same API version (`karga version` shows
  it). Operations record the user of the CLI that asked for them (`USER` in `karga events`).
- Linux only: without karga's gateway and release proxy (macOS, a remote Docker host) a plan with
  Services is refused. Upgrading an installed release with its new package recreates its workloads
  once.

### Packages and releases
- Packages: Go templates with Sprig, `values.yaml` and JSON Schema (writeOnly values are never
  stored), `karga create` to start one, `lint`, `template`, notes (`NOTES.txt`).
- Resources: Workload, Service, Volume, Network, Config, Secret.
- Releases: install, upgrade (`--reuse-values`, `--install`), rollback, uninstall; immutable
  revisions in karga-server's state; `history`, `diff`, `status --watch`, `get values|manifest|notes`.
- Plans as a summary (`--verbose` for fields) and live progress while applying.
- An upgrade recreates containers, networks and volumes removed outside karga; an install that
  fails before creating anything leaves no release behind.
- Data in an image's `VOLUME` path that no Volume covers survives upgrades (the anonymous volume
  moves to the new container), with a warning to mount a Volume there.
- `stopTimeout` on a workload: how long it gets to shut down before it is killed (default 10s);
  the Postgres example uses 60s.
- An upgrade or rollback that would delete a `retention: delete` volume stops unless given
  `--delete-volumes`; the plan marks what happens to a removed volume's data.

### Services and upgrades without downtime
- A release proxy (Envoy) serves the release's Services; `updateStrategy: replace` starts the new
  container next to the old one and switches the proxy over, so no client loses a request.
- A host gateway (Envoy) serves the administrator's listeners: tcp, and HTTP(S) by host and path,
  with TLS from a Secret and redirects from port 80.
- `tcp`/`http` health checks need no tool in the image.

### Operations
- `karga logs`, `karga exec`, `karga gateway status`,
  `karga volume backup|restore`, shell completion.
- Memory (without extra swap), CPU and process limits; OOM kills and restarts in `status`;
  container log rotation; `no-new-privileges` by default and a `security` block.

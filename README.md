# karga

Package management for containers on a **single host**, without Kubernetes.

karga renders a versioned package (templates + values + schema) into its own small resource model
(Workload, Service, Volume, Network, Config, Secret), applies it to Docker, and keeps every install,
upgrade and rollback as an immutable revision in karga-server's state.

karga is two programs: **[karga-server](https://github.com/hasankaradmir/karga-server)** owns the
state, talks to Docker and keeps every release running as declared (it repairs removed, stopped,
unhealthy or hand-changed containers); the **karga** CLI, in this repository, is its client on a
local unix socket. Both use [karga-sdk](https://github.com/hasankaradmir/karga-sdk), which also
holds the example packages used below.

Nothing a package runs is reachable on its own. Workloads publish no port; a **Service** gives a
workload a name inside its release, and only the host's administrator decides what is reachable from
outside, by defining the listeners of the host **gateway** that Services can be exposed on.

```bash
karga install cache examples/redis
karga upgrade cache examples/redis --set config.maxmemory=128mb --dry-run
karga upgrade cache examples/redis --set config.maxmemory=128mb
karga history cache
karga rollback cache 1
karga uninstall cache          # the data volume is kept
```

## Status

MVP, for one Linux host with Docker. Not available (yet): Podman, package repositories/registries,
package dependencies, automatic certificates, lifecycle jobs. macOS and Windows are not supported: Services
need karga's gateway and release proxy, which need Linux with a local Docker daemon (the macOS build
only lints and renders packages).

## Requirements

- karga-server on a Linux host with a local Docker Engine and systemd.
- The CLI runs on Linux and macOS. On macOS it writes, lints and renders packages; to manage a
  server, run karga on its host (`ssh host karga …`). Go 1.27 to build.

## Install

karga-server first, on the Linux host it manages (a systemd service; see its
[README](https://github.com/hasankaradmir/karga-server#install)):

```bash
curl -fsSL https://raw.githubusercontent.com/hasankaradmir/karga-server/master/scripts/install.sh | sh
sudo usermod -aG karga $USER      # then log in again: members of the karga group may use karga
```

Then the CLI (Linux or macOS, amd64 or arm64), checked against the release checksums:

```bash
curl -fsSL https://raw.githubusercontent.com/hasankaradmir/karga-cli/master/scripts/install.sh | sh
# KARGA_VERSION=v0.1.0 picks a version, KARGA_BIN_DIR another directory (default /usr/local/bin)
```

The CLI finds the server's socket at `/run/karga/karga.sock`, or at `--socket` / `KARGA_SOCKET`.
Without the server every command but `create`, `lint`, `template`, `show`, `completion` and
`version` stops with `karga-server is not running`. The CLI and the server work together when they
speak the same API version (`karga version` shows both).

Without a published release, `make dist` in both repositories builds the release archives and the
same scripts install them ([USAGE.md, "From source"](USAGE.md#from-source)); `make build` alone
produces `bin/karga`. Shell completion: `karga completion
bash|zsh|fish`. The examples below use the packages in karga-sdk's
[`examples/`](https://github.com/hasankaradmir/karga-sdk/tree/master/examples).

## Quickstart

```bash
karga lint examples/redis
karga template cache examples/redis --set image.tag=7.4.2      # render only, touches nothing

karga install cache examples/redis
karga status cache
karga exec cache redis -- redis-cli ping                       # nothing is reachable from the host

# The host's administrator opens a tcp listener; the package exposes its Service on it.
karga gateway listener add redis --port 16379 --protocol tcp
karga upgrade cache examples/redis --set service.listener=redis --dry-run
karga upgrade cache examples/redis --set service.listener=redis
redis-cli -p 16379 ping                                        # through the gateway, on 127.0.0.1 only
karga history cache
karga get values cache
karga get manifest cache

karga rollback cache 1
karga uninstall cache                    # asks first; keeps the volume; --delete-volumes removes it
```

Every command accepts `-o text|json|yaml` (streams write one JSON document per line; `logs` always
writes the containers' lines) and exits
with a code scripts can test (`USAGE.md`, "Output and exit codes"); commands that delete data or
expose services ask first, or need `--yes` in a script. `--debug` writes logs to stderr (`--log-format json` for structured
logs). The state lives in karga-server's `KARGA_HOME` (`/var/lib/karga` for the service; the server's
`--home` or `KARGA_HOME`, else `~/.local/share/karga`). Below, `$KARGA_HOME` means that directory.

## Commands

| Command | What it does |
|---|---|
| `create <dir>` | Start a new package: a web workload with an http health check, replace updates and notes |
| `lint <package>...` | Validate `Package.yaml`, values and schema, render and check every resource |
| `template <release> <package>` | Render locally; Secret and writeOnly values are shown as `<redacted>` unless `--show-secrets` |
| `show package\|values\|readme <package>` | Show package metadata, default values or README |
| `version` | Versions of karga and karga-server and their API version (exit code 3 if they do not work together) |
| `reconcile <release>` | Repair now: bring what runs back to the current revision (karga-server also does it on its own) |
| `pause\|resume <release>` | Stop or restart karga-server's repairs of a release (maintenance) |
| `events [release]` | Commands and repairs, oldest first (`-f` follows, `-n` how many) |
| `install <release> <package>` | Install. Re-running with the same package and values does nothing |
| `upgrade <release> <package>` | Apply new values or a new package version. `--reuse-values`, `--install`, `--strategy`, `--delete-volumes` |
| `rollback <release> [revision]` | Re-apply an earlier revision as a new revision. `--strategy`, `--delete-volumes` |
| `uninstall <release>` | Remove workloads and networks (asks first). `--dry-run`, `--delete-volumes`, `--keep-history`, `--force` |
| `history`, `list`, `status <release>` | Revisions (`--diff` adds what changed), releases, and desired vs actual state (`--watch`) |
| `diff <release> <rev> [rev]` | What differs between two revisions (values, package, resource fields; secrets only as changed) |
| `get values\|manifest\|notes <release> [revision]` | Stored values (`--all` merges defaults), rendered resources, or the package's notes |
| `exec <release> <workload> [-it] [-- cmd]` | Run a command in a workload (default: an interactive `sh`); exits with its code |
| `logs <release> [workload...]` | Container logs (`--tail`, `-f`); `--proxy` for the release proxy, `--gateway` for the gateway |
| `gateway status` | Health, version, image and listen addresses of the host gateway, and what it serves |
| `gateway get\|diff -f\|apply -f` | The gateway settings as a document you keep (kind `Gateway`): print, compare, apply as a new revision (a failed apply keeps the one in force) |
| `gateway edit` | Edit the settings in force in your editor; problems reopen it; applies a revision |
| `gateway history\|rollback [rev]` | The revisions applied (the last 10), and applying an earlier one again |
| `gateway listener list\|add\|set\|remove` | The ports Services can be exposed on: `add <name> --port <port> --protocol tcp\|http\|https`; `set <name>` changes its timeouts, body and rate limits and PROXY protocol; a listener in use cannot be removed |
| `gateway set listen <ip>[,<ip>]` | Where every listener binds (default `127.0.0.1`; `0.0.0.0` is every interface) |
| `gateway set access-log on\|off` | Log every request and connection the gateway handles (off by default) |
| `volume backup\|restore <release> <volume> <file>` | Copy a volume's files to a gzipped tar (its workloads stopped meanwhile; `--live` while they run), or replace them with one (`--merge`, `--no-restore-point`, `--from-other-volume`) |
| `volume undo-restore <release> <volume>` | Put back the files a volume had before its last restore |
| `completion bash\|zsh\|fish\|powershell` | Shell completion |

`install`, `upgrade` and `rollback` take `--dry-run`, `--verbose` (every changed field of the
plan), `--timeout` (how long to wait for workloads; karga-server's default `5m`), `--history-max`
(revisions kept; karga-server's default 10) and `--force`; they show their progress on stderr. A package's `NOTES.txt` (a template) is shown after install and upgrade. Tables are
colored on a terminal (`--color auto|always|never`, `NO_COLOR`). Values come from the package defaults, then each `-f/--values` file in
order, then `--set` and `--set-string`; `--set a=null` deletes a default.

## Packages

```text
redis/
├── Package.yaml          # apiVersion, name, version, runtime.supported
├── values.yaml           # defaults
├── values.schema.json    # optional JSON Schema, checked before rendering
├── README.md
├── templates/            # *.yaml render to resources; _*.tpl only hold named templates; .* are ignored
├── files/                # read from templates with .Files.Get / .Files.Glob
└── .kargaignore          # optional: paths left out of the package (one pattern per line)
```

A package directory is read without `.git` and without what `.kargaignore` lists (`*.env`,
`build/`, ...), at most 10000 files and 32 MiB. Rendering is bounded too: `include` and `tpl` nest
at most 100 deep, a render takes at most 10 seconds and writes at most 16 MiB, its `range` loops
run at most 1,000,000 iterations together, and no function returns more than 16 MiB of text or
1,000,000 elements.

```yaml
# Package.yaml
apiVersion: karga.io/v1alpha1
name: redis
version: 1.0.0
runtime:
  supported: [docker]
```

A package can also be a `.tgz` with the directory at its root.

Templates use Go `text/template` with the [Sprig](https://masterminds.github.io/sprig/) functions,
plus `include`, `tpl`, `required`, `toYaml` and `fromYaml`. The data is `.Values`,
`.Release.Name`, `.Release.Revision`, `.Package.Name`, `.Package.Version` and `.Files`. Rendering is
deterministic: functions that read the host or return different results on each run are removed
(`env`, `expandenv`, `getHostByName`, `now`, `date*`, `rand*`, `uuidv4`, `shuffle`, `gen*`,
`encryptAES`, `bcrypt`, `htpasswd`), and `keys`/`values` return sorted results. `--strict` rejects
values that the schema does not declare.

Mark passwords and tokens `"writeOnly": true` in `values.schema.json`. They are used to render but are
stored and printed as `<redacted>`; `--reuse-values` asks for them again.

## Resources

All resources use `apiVersion: karga.io/v1alpha1` and `metadata.name` (a DNS label). Runtime objects
are named `<release>-<name>` and labelled `io.karga.*`.

**Workload**: one container. It publishes nothing and its name in the package is not a DNS name:
other workloads and outside clients reach it through a Service. (Docker still resolves the container
name, `<release>-<name>`, on the release's networks; there is no network policy yet.)

```yaml
kind: Workload
metadata: {name: web}
spec:
  image: nginx:1.27
  pullPolicy: if-not-present        # always | if-not-present | never
  restartPolicy: always             # always | on-failure | unless-stopped | never
  command: []
  args: []
  environment: {KEY: value}
  networks: [web-net]               # Services of this workload are reachable on these networks
  ports:                            # what the container listens on, for Services to refer to
    - {name: http, containerPort: 80}           # protocol defaults to tcp
  mounts:
    - {volume: data, mountPath: /data, readOnly: false}
    - {config: site, mountPath: /etc/nginx/conf.d}      # always read-only
    - {secret: creds, mountPath: /run/secrets/creds}    # always read-only
  healthCheck: {type: exec, command: [curl, -f, http://127.0.0.1/], interval: 10s, timeout: 3s, retries: 3, startPeriod: 30s}
  # or {type: http, port: 80, path: /healthz} / {type: tcp, port: 80}: no tool needed in the image
  resources: {memory: 512Mi, cpu: 1, pids: 1024}  # memory also caps swap; pids defaults to 1024 (0: no limit)
  logging: {maxSize: 10m, maxFiles: 3}            # the defaults: at most 30MB of container log
  security:                                       # noNewPrivileges defaults to true
    noNewPrivileges: true
    readOnlyRootFilesystem: false                 # true: image read-only, /tmp is a 64m tmpfs
    capDrop: [ALL]
    capAdd: [NET_BIND_SERVICE]
    user: "1000:1000"
  updateStrategy: {type: recreate, drainTimeout: 10s}   # recreate | replace
  dependsOn: [postgres]                           # Services of the release to wait for before starting
  stopTimeout: 10s                                # default; time to shut down after SIGTERM before a kill (1s-1h)
```

**Service**: a stable name for one workload inside the release, served by the release proxy, and
optionally exposed on the host gateway's listeners.

```yaml
kind: Service
metadata: {name: shop}              # other workloads of the release connect to shop:80
spec:
  workload: web
  ports:
    - {name: http, port: 80, targetPort: http}   # targetPort: a port name or number of the workload (default: name)
  expose:                                        # optional; listeners are defined by the host's administrator
    - {port: http, listener: http, host: shop.example.com, path: /}          # http listener: by Host and path (pathType prefix or exact; to rewrites)
    - {port: http, listener: https, host: shop.example.com, tls: {secret: shop-cert}}   # https; Secret has tls.crt, tls.key
    # - {port: db, listener: pg}                 # tcp listener: belongs to this one service
```

One release proxy serves every Service of a release on one address, so a port number belongs to one
Service of the release. A workload is reachable only on the networks it joins, so a Service's
workload must join one.

**Health checks:** `exec` runs a command inside the container (Docker's healthcheck), so the image
needs that tool. `tcp` (the port accepts a connection) and `http` (GET `path` answers 2xx/3xx) are run
by karga from the host against the container's address and need nothing in the image. Behind the
release proxy, Envoy checks the same HTTP path before switching traffic to a new container. All types take `interval`,
`timeout`, `retries` and `startPeriod`, and any of them makes a workload eligible for replace.

**Limits and defaults:** Docker enforces memory (without extra swap), CPU and the process limit;
`karga status` shows restarts and OOM kills. Container logs rotate (json-file). Workloads run with
`no-new-privileges`; an image that needs setuid (e.g. `sudo`) needs `noNewPrivileges: false`.
Releases installed by an earlier karga apply their workloads once more for these defaults; the plan
lists the new fields.

**Volume**: `driver` (`local`, the only one a package may use) and `retention: retain | delete` (default `retain`). A retained
volume survives uninstall and is reused by the next install of the same release. An upgrade or
rollback that would delete a `retention: delete` volume stops unless given `--delete-volumes`.
**Network**: `driver` (`bridge`, the only one: other drivers would put containers on the host's
network, past the gateway) and `internal`.
**Config**: `files: {name: content}`, mounted read-only.
**Secret**: `values: {name: content}`, mounted read-only as files.

## Upgrades

Before any change karga pulls every image it needs, so a bad tag fails without touching what runs.
`--dry-run` shows the plan: what is created, updated, deleted or unchanged, and which fields change.

- **recreate** (default): stop the old container, then start the new one. There is a short outage.
- **replace**: start the new container next to the old one, switch traffic to it once it is healthy,
  then let the old one drain for up to `drainTimeout` and remove it. If the new container never
  becomes healthy, the old one keeps running and the release stays deployed. Replace only applies
  when the workload mounts volumes read-only and has a healthcheck; otherwise the plan falls back to
  recreate and says why.

Traffic goes through karga's Envoys: a release proxy per release with Services, and the host
gateway. How they route, switch a replace without losing requests, serve TLS and what they cost is
in karga-server's [README](https://github.com/hasankaradmir/karga-server#traffic-through-kargas-envoys).

If a recreate fails, the failed container is stopped (but kept for `docker logs`) and the revision is
marked failed. The previous revision stays current, but its container was replaced: the workload is
down until you run the rollback the error names (`karga rollback <release> <revision>`).
Rollback re-applies the stored desired state; it cannot undo data changes such as database
migrations. Changing a Network or Volume spec in an upgrade is refused.

## State and security

karga-server keeps the state (YAML files under `$KARGA_HOME/state`) and runs as an unprivileged
user; access to its socket is as powerful as access to Docker. Backups (a copy of `$KARGA_HOME`,
`karga volume backup|restore`), the state key and the security model are described in
karga-server's [README](https://github.com/hasankaradmir/karga-server#state).

## Development

```bash
make test     # unit tests: the CLI against a fake karga-server behind karga-sdk's HTTP handler
make lint     # golangci-lint
make build    # bin/karga
```

The CLI depends only on [karga-sdk](https://github.com/hasankaradmir/karga-sdk) (the API, the
package engine, plans). What the server does with requests is tested in karga-server, whose e2e
tests build this CLI.

Releases: add the version to `CHANGELOG.md`, then `git tag vX.Y.Z && git push --tags`; the release
workflow builds the binaries with GoReleaser and creates a draft GitHub release with the changelog
section as notes; publish it to make it the latest release.

Documentation:

- [USAGE.md](USAGE.md): how to use karga, scenario by scenario.
- karga-server's [docs/](https://github.com/hasankaradmir/karga-server/tree/master/docs): how karga
  works, the roadmap and architecture decisions.

## License

Apache License 2.0; see [LICENSE](LICENSE). Security issues: see [SECURITY.md](SECURITY.md).

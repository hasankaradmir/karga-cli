# Using karga

This guide walks through karga by task: what to run, what you will see, and what to do when
something goes wrong. The command list is in the [README](README.md); how karga works inside is in
karga-server's [docs/design.md](https://github.com/hasankaradmir/karga-server/blob/master/docs/design.md). Outputs below are from real runs, shortened; client addresses are
replaced by documentation addresses.

- [1. Concepts in five minutes](#1-concepts-in-five-minutes)
- [2. Install karga](#2-install-karga)
- [3. Your first release](#3-your-first-release)
- [4. Configure with values](#4-configure-with-values)
- [5. Upgrade safely](#5-upgrade-safely)
- [6. Services and the gateway](#6-services-and-the-gateway)
- [7. Upgrades without downtime](#7-upgrades-without-downtime)
- [8. Databases and other stateful services](#8-databases-and-other-stateful-services)
- [9. Secrets](#9-secrets)
- [10. Resources and hardening](#10-resources-and-hardening)
- [11. Operate and troubleshoot](#11-operate-and-troubleshoot)
- [12. Back up and move a host](#12-back-up-and-move-a-host)
- [13. Write a package](#13-write-a-package)
- [14. Uninstall and clean up](#14-uninstall-and-clean-up)
- [15. Put your own application online](#15-put-your-own-application-online)
- [16. Limits to know](#16-limits-to-know)

## 1. Concepts in five minutes

| Term | Meaning |
|------|---------|
| **Package** | A directory (or `.tgz`) with `Package.yaml`, `values.yaml`, an optional `values.schema.json`, `templates/` and an optional `NOTES.txt`. |
| **Values** | The settings of a package: its defaults, overridden with `-f file.yaml` and `--set key=value`. |
| **Resources** | What templates render to: `Workload` (a container), `Service`, `Volume`, `Network`, `Config` (files), `Secret`. |
| **Release** | A package installed under a name, e.g. `shop`: a DNS label (lowercase letters, digits and `-`, at most 63) that does not start with `karga`, which karga keeps for its own objects. Its containers are named `<release>-<name>` (`shop-web`). |
| **Revision** | Each install, upgrade and rollback is a new, immutable revision of the release. |
| **Plan** | What a command will change: created (`+`), updated (`~`) and deleted (`-`) resources, and for workloads how (recreate or replace). |
| **Service** | The only way to reach a workload: a name inside the release (`web:80`), and optionally an exposure on a gateway listener. A workload alone publishes nothing. |
| **Release proxy** | One Envoy per release with Services (`<release>-karga-proxy-<hash>`). It answers to the Service names and forwards to the current container, so upgrades switch without losing requests. |
| **Gateway** | One Envoy per host (`karga-gateway-<hash>`), the only way in from outside. The host's administrator defines where it listens and its **listeners** (a name, a port, a protocol); packages can only expose Services on them. |

Where things live:

- **karga-server** (a systemd service) owns the state and talks to Docker. The **karga** CLI is its
  client on `/run/karga/karga.sock`; it sends packages and values files over the socket and runs
  nothing itself.
- Docker runs the containers, the gateway and the release proxies, and restarts them.
- The server's `KARGA_HOME` (`/var/lib/karga` for the service) holds the state (YAML files under
  `state/`), the key `state.key`, rendered Config and Secret files, the gateway settings
  revisions `gateway-settings/` and the Envoy configuration. Only the `karga` user can read it (mode 0700).

## 2. Install karga

### Requirements

- karga-server: Linux with systemd and Docker Engine, rootful (the default; not rootless Docker,
  not Docker Desktop): install it first ([docs.docker.com/engine/install](https://docs.docker.com/engine/install/)),
  so that the `docker` group exists. `curl` for the install scripts.
- The CLI: Linux or macOS. On macOS it writes, lints and renders packages; to manage a server, run
  karga on its host (`ssh host karga ...`).

### From a release

karga-server and the CLI are released from their own repositories
([karga-server](https://github.com/hasankaradmir/karga-server), [karga](https://github.com/hasankaradmir/karga-cli)).
Each install script checks the archive against the release checksums:

```console
$ curl -fsSL https://raw.githubusercontent.com/hasankaradmir/karga-server/master/scripts/install.sh | sh
karga-server v0.1.0 installed to /usr/local/bin/karga-server
karga-server is running; add users to the karga group to use karga: sudo usermod -aG karga $USER
$ curl -fsSL https://raw.githubusercontent.com/hasankaradmir/karga-cli/master/scripts/install.sh | sh
karga v0.1.1 installed to /usr/local/bin/karga
$ sudo usermod -aG karga $USER          # log in again afterwards
```

`KARGA_VERSION=v0.1.0` picks a version, `KARGA_BIN_DIR=~/bin` another directory. Without the
scripts: download `karga-server_0.1.0_linux_amd64.tar.gz` and `karga_0.1.0_linux_amd64.tar.gz`
from the releases pages, then install `karga-server`, `deploy/systemd/karga-server.service` and
`karga` from the archives.

### From source

Without a published release, build the same archives from the repositories and install them with
the same scripts. Build where Go 1.27 is installed (the server host, or another machine), then
run the scripts on the server host:

```console
$ git clone https://github.com/hasankaradmir/karga-server && git clone https://github.com/hasankaradmir/karga-cli
$ make -C karga-server dist
install: KARGA_VERSION=v0.1.0-5-g1a2b3c4 KARGA_DOWNLOAD_URL=file:///home/me/karga-server/dist KARGA_SKIP_SIGNATURE=1 sh scripts/install.sh
$ make -C karga-cli dist
install: KARGA_VERSION=v0.1.1-3-g5d6e7f8 KARGA_DOWNLOAD_URL=file:///home/me/karga-cli/dist KARGA_SKIP_SIGNATURE=1 sh scripts/install.sh

$ cd karga-server && KARGA_VERSION=v0.1.0-5-g1a2b3c4 KARGA_DOWNLOAD_URL=file://$PWD/dist KARGA_SKIP_SIGNATURE=1 sh scripts/install.sh
karga-server v0.1.0-5-g1a2b3c4 installed to /usr/local/bin/karga-server
karga-server is running; add users to the karga group to use karga: sudo usermod -aG karga $USER
$ cd ../karga-cli && KARGA_VERSION=v0.1.1-3-g5d6e7f8 KARGA_DOWNLOAD_URL=file://$PWD/dist KARGA_SKIP_SIGNATURE=1 sh scripts/install.sh
karga v0.1.1-3-g5d6e7f8 installed to /usr/local/bin/karga
$ sudo usermod -aG karga $USER          # log in again afterwards
```

`make dist` prints the install line with its version (from `git describe`). Built elsewhere, copy
`dist/` and `scripts/install.sh` to the server host and run the line there with that path. The
archives have the release names and contents, `checksums.txt` is checked, and there is no
signature to verify (`KARGA_SKIP_SIGNATURE=1`). Running the script again with a newer build
upgrades in place; the server restarts only if its binary changed.

The example packages used below (`examples/redis`, `examples/postgres`) are in
[karga-sdk](https://github.com/hasankaradmir/karga-sdk/tree/master/examples).

### The server

```console
$ systemctl status karga-server
$ karga version
karga v0.1.1 (commit 55fb34d8c63c, go1.27.4, API 4.1)
karga-server v0.1.0 (API 4.1)
$ journalctl -u karga-server -f          # the server's log
```

The service runs as the `karga` system user (in the `docker` group) with its state in
`/var/lib/karga`, and listens on `/run/karga/karga.sock` (mode 0660, group `karga`). Anyone in the
`karga` group can manage every release, which is as much as access to Docker: add only trusted
users. When the server is down, the CLI says so:

```console
$ karga list
Error: karga-server is not running (no server on /run/karga/karga.sock): start it with 'sudo systemctl start karga-server'
```

The CLI and the server work together when they speak the same major API version (`karga
version` shows both, e.g. `3.0` and `3.1`), whatever their release versions; otherwise every
command stops with `karga speaks API N, karga-server API M: upgrade ...` (exit code 3). A newer
CLI still works with an older server of the same major; what the server lacks fails with a
message to upgrade it. Every operation records the user of the CLI that asked
for it, as the kernel reports it on the socket (`USER` in `karga events`; `karga-server` for its
own repairs and resumes).

`create`, `lint`, `template`, `show`, `completion` and `version` work without it. `--socket` or
`KARGA_SOCKET` point the CLI at another socket.

Operations run in the server, not in the CLI. Ctrl-C (or a lost connection) stops only the
progress output: the operation goes on to its end, and `karga events -f <release>` follows it. A
volume backup is the exception, as its file is the output: Ctrl-C stops the copy, removes the file
and exits 7, and the workloads the backup stopped start again. When
the server stops (`systemctl stop`, a reboot) it waits up to 5 minutes for running operations. If
it is killed in the middle of one, it finishes the operation when it starts again, with the
options it was given (`--delete-volumes`, `--strategy`, `--timeout`, `--keep-history`): an install,
upgrade or rollback deploys its revision again (steps already done are skipped, no new revision),
an uninstall or a volume restore runs again, and other operations (a volume backup, a repair) are aborted. `karga events` shows the interrupted operation as `aborted` and
the one that finished it as `resume`.

### Development server

```console
$ make build        # in each checkout, needs Go 1.27: bin/karga (here), bin/karga-server (karga-server)
$ ../karga-server/bin/karga-server --home /tmp/karga --socket /tmp/karga.sock &     # a server of your own, for development
$ KARGA_SOCKET=/tmp/karga.sock bin/karga list
```

### Docker

karga needs Linux with a local, rootful (not rootless) Docker daemon: its Envoys share files, unix sockets and the host
network with it. On macOS, `lint` and `template` work, but a plan with Services is refused
(`services ... need karga's gateway and release proxy (Linux with a local, rootful Docker)`).

### Shell completion

```console
$ karga completion zsh > "${fpath[1]}/_karga"     # bash: karga completion bash > /etc/bash_completion.d/karga
```

Completion offers release, workload, volume and revision names from your state.

## 3. Your first release

Create a package to start from, check it and look at what it would do:

```console
$ karga create shop
Created package shop. Try: karga lint shop && karga install <release> shop

$ karga lint shop
1 package(s) linted, 0 failed

$ karga install demo shop --dry-run
Deployment Plan

Release:  demo
Revision: 1

  + NETWORK net
  + WORKLOAD web
  + SERVICE web
  + PROXY proxy

4 to create, 0 to update, 0 to delete, 0 unchanged.
```

The package has a workload `web` (nginx) and a Service `web` for it; karga adds the release proxy
that serves the Service. `karga template demo shop` shows the rendered resources. Install it.
Progress goes to stderr; the package's notes are printed at the end:

```console
$ karga install demo shop
[  0s] workload web: image nginx:1.27-alpine (if-not-present)
[  0s] workload web: creating demo-web
[  2s] workload demo-web: waiting for demo-web to be ready (...)
[  5s] proxy proxy: create done
Release "demo" installed (revision 1).
  create network net
  create workload web
  create service web
  create proxy proxy

NOTES:
demo (revision 1) runs nginx:1.27-alpine.

Open http://web:80/ from the release network (not exposed on the host)
Logs:   karga logs demo -f
Shell:  karga exec demo web
```

Nothing is reachable from the host yet: the workload publishes no port. Inside the release, the
Service name answers:

```console
$ karga exec demo web -- wget -qO- http://web:80/
<!DOCTYPE html>
<html>
<head>
<title>Welcome to nginx!</title>
...

$ karga status demo
Release:  demo
Status:   deployed
Revision: 1

KIND      NAME   RUNTIME NAME               REVISION  DESIRED  ACTUAL   RESTARTS  HEALTH/BACKEND
Network   net    demo-net                   1         applied  present
Proxy     proxy  demo-karga-proxy-d41a7c90  1         applied  running  0         -> karga-web-r1
Service   web    demo-karga-proxy           1         applied  routed             -> karga-web-r1
Workload  web    demo-web-8b1d04e7          1         applied  running  0         healthy

$ karga list
NAME  REVISION  STATUS    PACKAGE  VERSION  UPDATED
demo  1         deployed  shop     0.1.0    2026-11-17 17:59:08 +03:00
```

[Section 6](#6-services-and-the-gateway) exposes it on the host. `karga exec demo web` without a
command opens an interactive shell; `-it` gives any command a terminal. `karga logs demo -f`
follows the logs; `karga status demo --watch` refreshes every two seconds.

Installing the same package with the same values again does nothing: `Release "demo" is already
installed with the same package and values`.

[Section 15](#15-put-your-own-application-online) puts a complete application (an API and its
database, on a domain) online.

## 4. Configure with values

See a package's defaults with `karga show values <package>`. Override them, later sources winning:

```console
$ karga install db examples/postgres -f prod.yaml --set service.listener=pg --set database.password=s3cret-pw
```

- `-f file.yaml` (repeatable), then `--set a.b=c` and `--set-string a=007` in the order you give
  them (the last one wins). `--set` makes `true`, `false`, `null` and integers typed; decimals such as `7.4` stay strings (an image tag), so put a number-typed decimal in a `-f` file.
- `--set key=null` removes a default.
- Maps merge key by key; a list replaces the default list as a whole (`-f` with `hosts: [a]`
  leaves only `a`). An index edits the list instead: `--set hosts[1]=b` keeps `hosts[0]`.
- `upgrade` starts from the package defaults; add `--reuse-values` to keep the last revision's
  overrides and change only what you pass.

Values are checked against the package's schema before anything happens:

```console
$ karga install demo2 shop --set service.port=99999
Error: values do not match values.schema.json:
- at '/service/port': maximum: got 99,999, want 65,535
```

What a release was installed with (`--all` adds the defaults; `karga get values db 2` shows revision 2):

```console
$ karga get values db
database:
  password: <redacted>
service:
  listener: pg
```

Values the schema marks `"writeOnly": true` (passwords, tokens) are used to render but stored and
shown as `<redacted>`; `--reuse-values` cannot bring them back, so pass them again on each upgrade
(a values file kept outside git works well).

## 5. Upgrade safely

### Read the plan first

```console
$ karga upgrade demo shop --set image.tag=1.27.3-alpine --dry-run
Revision: 1 -> 2

  ~ WORKLOAD web (replace)

0 to create, 1 to update, 0 to delete, 3 unchanged. --verbose shows the changed fields.
```

`--verbose` lists every changed field (`old -> new`, `(added) -> new`, `old -> (removed)`; a Secret
value only as `(changed)`). The text in parentheses is the update strategy and, when a
workload asked for `replace` but cannot have it, why:

- **recreate**: the new container is created next to the old one, then the old one stops and the
  new one starts. A short outage, from the stop to the start; a container Docker refuses (a bad
  mount or option) leaves the old one running.
- **replace**: the new container starts next to the old one and takes over once healthy. Needs a
  health check and read-only volumes (see [section 7](#7-upgrades-without-downtime)).

A workload's container is named `<release>-<workload>-<hash>` (e.g. `demo-web-3f9a2c1b`), the hash
coming from the workload's spec: an update gives it a new name, an upgrade that changes nothing
keeps it. `karga status` shows the current one, and Docker finds it by label:
`docker ps --filter label=io.karga.release=demo --filter label=io.karga.name=web`.

`--strategy recreate|replace` overrides the packages' choice for one upgrade or rollback.

### Upgrade

```console
$ karga upgrade demo shop --set image.tag=1.27.3-alpine
[  0s] workload web: image nginx:1.27.3-alpine (if-not-present)
[  0s] workload web: starting demo-web-3f9a2c1b next to demo-web-8b1d04e7
[  5s] workload web: switching traffic to demo-web-3f9a2c1b
[  5s] workload web: waiting up to 10s for connections to demo-web-8b1d04e7 to finish
[  5s] workload web: stopping demo-web-8b1d04e7
Release "demo" upgraded (revision 2).
  update workload web (replace)
```

Images are pulled before anything running is touched. While Docker pulls one, every two seconds a
line says how far it got (the total grows as Docker learns the image's layers):

```console
$ karga install db examples/postgres --set database.password=s3cret-pw
[  0s] workload postgres: image postgres:17 (if-not-present)
[  2s] workload postgres: pulling postgres:17 (2s)
[  4s] workload postgres: pulling postgres:17: 57MB/165MB (4s)
[  6s] workload postgres: pulling postgres:17: 142MB/171MB (6s)
[  8s] workload postgres: pulling postgres:17: 171MB/171MB (8s)
[  9s] network postgres-network: create done
...
```

A tag that does not exist changes nothing:

```console
$ karga upgrade demo shop --reuse-values --set image.tag=does-not-exist
Error: Workload "web": image nginx:does-not-exist was not found in its registry; check the repository and tag (...)
revision 3 failed; nothing that was running changed and revision 2 is still serving
```

The same holds when a replaced workload's new container never becomes healthy. A recreate has
already replaced the old container: when the new one does not get ready, it is stopped (not
removed, so `karga logs` still works), the revision is marked failed, and the workload is down until
you roll back. A container that keeps crashing fails after 3 restarts, not at the timeout:

```console
$ karga upgrade demo shop --reuse-values --set image.tag=broken
Error: workload demo-web-9cc139f0 keeps restarting (3 restarts, last exit code 3)
revision 3 failed and workload web is down: its revision 2 container was replaced. Run 'karga rollback demo 2' to start revision 2 again
```

The rollback names its revision: `karga rollback demo` alone goes to the revision before 2.

### History, diff and rollback

```console
$ karga history demo --diff
REVISION  STATUS      PACKAGE  VERSION  CREATED                     DESCRIPTION       CHANGES
1         superseded  shop     0.1.0    2026-11-17 13:18:00 +03:00  Install complete  first
2         deployed    shop     0.1.0    2026-11-17 13:18:06 +03:00  Upgrade complete  +0 ~1 -0, 1 values
3         failed      shop     0.1.0    2026-11-17 13:18:21 +03:00  Upgrade failed: ...

$ karga diff demo 1
Release demo: revision 1 -> 2

Values:
  image.tag: "1.27-alpine" -> "1.27.3-alpine"

Resources:
  ~ WORKLOAD web
      spec.image: "nginx:1.27-alpine" -> "nginx:1.27.3-alpine"

$ karga rollback demo --dry-run
Revision: 2 -> 4
  ~ WORKLOAD web (replace)
```

A description longer than 60 characters is cut in the table (`...`); `-o json` has it whole.
`deployed` is the revision that runs (the newest one that deployed: a failed upgrade after it
leaves it running), `superseded` an earlier one a later deployment replaced, `failed` one that did
not deploy. Rollback goes back to superseded revisions, never to failed ones.

`karga rollback demo` returns to the last good revision before the current one; `karga rollback
demo 1` to a given one. A rollback is a new revision. It re-applies configuration; it cannot undo
changes your application made to its data (database migrations, for example). A rollback that
would delete a volume's data needs `--delete-volumes` (see
[section 8](#8-databases-and-other-stateful-services)).

## 6. Services and the gateway

A workload publishes nothing. Two things make it reachable:

- A **Service** gives it a name inside the release, on the networks the workload joins:

  ```yaml
  kind: Service
  metadata: {name: web}           # other workloads connect to web:80
  spec:
    workload: web
    ports:
      - {name: http, port: 80, targetPort: http}   # targetPort: the workload's port name or number
  ```

  A Service name resolves only on its release's networks: two releases can both have a Service
  `web` on port 80 without seeing each other. One release proxy serves all Services of a release, so
  a port number belongs to one Service of the release.
- An **exposure** puts a Service port on a listener of the host gateway. Listeners belong to the
  host's administrator, not to packages.

### Listeners and listen addresses (the administrator)

```console
$ karga gateway listener list
NAME   PORT  PROTOCOL  OPTIONS            SERVICES
http   80    http      redirect-https=on
https  443   https     -

$ karga gateway listener add pg --port 15432 --protocol tcp
listener pg added (tcp on port 15432)
```

- `http` and `https` listeners are shared: each exposure selects a Service by host name and path (`pathType` prefix or exact; below)
  prefix. A `tcp` listener belongs to one Service.
- Every listener binds the addresses of `karga gateway set listen`, by default `127.0.0.1` only:
  nothing is reachable from other machines until you say so.

  ```console
  $ karga gateway set listen 0.0.0.0          # every interface; or a list: 10.0.0.5,127.0.0.1
  gateway listens on 0.0.0.0
  ```

  Then make sure DNS points the host names at the server and the firewall lets the ports through.
- A listener's options bound what reaches the Services on it. They are the administrator's, shared
  by every route on the listener, and change without restarting the gateway. Give them to
  `listener add` or change them with `listener set` (options not given stay; `0` or `off` removes
  one):

  ```console
  $ karga gateway listener set http --timeout 30s --max-body 10Mi --rate-limit 100/s
  listener http: timeout=30s max-body=10Mi rate-limit=100/s redirect-https=on
  $ karga gateway listener set pg --proxy-protocol v2 --idle-timeout 8h
  listener pg: idle-timeout=8h proxy-protocol=v2
  ```

  | Option | Listeners | Effect |
  |--------|-----------|--------|
  | `--timeout` | http, https | Longest a request may take; the gateway answers 504 after it. Default: none. |
  | `--idle-timeout` | all | Closes a request, a WebSocket or a tcp connection that sends nothing either way for that long. Default: 5m (http), 1h (tcp). |
  | `--max-body` | http, https | Largest request body, at most `16Mi`; 413 beyond it. The gateway holds each request's body in memory, and its memory is 128Mi: a few large uploads at once can exhaust it. Larger uploads are limited by the application. |
  | `--rate-limit` | http, https | Requests per second or minute (`100/s`, `600/m`) for the whole listener (an https listener counts each host on its own); 429 beyond it. |
  | `--proxy-protocol` | tcp | Sends the client's address first on every connection (`v1` or `v2`). The backend must expect it (PostgreSQL does not; HAProxy, nginx `proxy_protocol` and most mail servers do). |
  | `--redirect-https` | http | Answers plain requests for the hosts an https listener serves (and this one does not) with a redirect to that listener's port. The default `http` listener has it. |
- A listener a Service uses cannot be removed:

  ```console
  $ karga gateway listener remove pg
  Error: gateway listener "pg" is used by db/postgres; remove those exposures first
  ```

### Gateway settings as a document

The gateway settings are also a document you keep where you like (a git repository, `/etc`),
like a Kubernetes manifest. karga never writes it: it keeps the revisions applied (the last 10).

```console
$ karga gateway get > gateway.yaml          # the settings in force, to start from
```

```yaml
apiVersion: karga.io/v1alpha1
kind: Gateway
spec:
  listen: [127.0.0.1]
  accessLog: false
  listeners:
    - {name: http, port: 80, protocol: http, redirectHTTPS: true, timeout: 30s, rateLimit: 100/s}
    - {name: https, port: 443, protocol: https}
    - {name: pg, port: 15432, protocol: tcp, proxyProtocol: v2, idleTimeout: 8h}
```

```console
$ karga gateway diff -f gateway.yaml
~ listener http: timeout: - -> 30s, rateLimit: - -> 100/s
+ listener pg: port: 15432, protocol: tcp, proxyProtocol: v2, idleTimeout: 8h

$ karga gateway apply -f gateway.yaml
~ listener http: timeout: - -> 30s, rateLimit: - -> 100/s
+ listener pg: port: 15432, protocol: tcp, proxyProtocol: v2, idleTimeout: 8h
gateway settings applied (revision 4)

$ karga gateway history
REVISION        TIME                        ACTOR  SOURCE
2               2026-11-19 10:02:11 +03:00  alice  listener add pg
3               2026-11-19 10:14:40 +03:00  alice  apply
4 (in force)    2026-11-19 10:31:05 +03:00  alice  apply

$ karga gateway rollback          # the revision before; or: karga gateway rollback 2

$ karga gateway edit              # the settings in force, in $KARGA_EDITOR or $EDITOR (vi)
```

- The document is the whole desired state: a listener it does not name is removed. Fields it
  leaves out take their defaults (`listen: [127.0.0.1]`; no `listeners` at all: `http` on 80
  with `redirectHTTPS` and `https` on 443).
- `diff` and `apply` refuse a document with problems (exit 2): YAML that does not parse, an
  unknown field (a typo), another `apiVersion` or `kind`, a value that does not check (the option
  table above), a listener a Service uses removed or changed to another protocol, a port another
  process holds. `-f -` reads stdin.
- Before anything changes, Envoy itself checks the configuration in a container of its own (a
  certificate it cannot load, a value it refuses); what it refuses is a problem too. It cannot
  check that an address or port can be bound: that shows on apply (below).
- `edit` opens the document again with the problems on top until it has none; saving it without a
  change gives up and says where your edit is kept.
- `apply`, `rollback` and `edit` ask before listening beyond loopback or removing a listener (`--yes`).
- If the gateway does not serve the new revision (Envoy cannot bind an address, it does not become
  ready), the revision is dropped and the one before keeps serving:

  ```console
  $ karga gateway apply -f gateway.yaml --yes
  ~ listen: 127.0.0.1 -> 127.0.0.1,10.0.0.9
  Error: ... rejected listener http_10.0.0.9_80: cannot bind '10.0.0.9:80': Cannot assign requested address; the gateway kept revision 4
  ```
- The gateway commands above (`listener add|set|remove`, `set listen|access-log`) apply a revision
  the same way; `karga gateway get` shows the result to keep your document current.
- With `-o json` or `-o yaml`, `apply`, `rollback` and `edit` write one document: `plan` (what
  changes) and `revision` (the one applied); with nothing to change, the plan alone.

### Expose a Service (the package)

```yaml
spec:
  expose:
    - {port: http, listener: http, host: shop.local}       # path / (prefix): everything
    - {port: postgres, listener: pg}                         # tcp: no host
```

On http and https listeners `path` and `pathType` choose what a host sends to the Service:

| `path` | `pathType` | Takes | Not |
|--------|------------|-------|-----|
| `/` (default) | `prefix` (default) | everything on the host | |
| `/api` | `prefix` | `/api`, `/api/`, `/api/orders` (by segment; `/api/` is the same) | `/apix` |
| `/healthz` | `exact` | `/healthz` only | `/healthz/x` |

The most precise wins: exact paths first, then the longest prefix, then `/`. With `to`, a
prefix is replaced in what the Service receives, so an application written for
`/` serves under a sub-path; the removed prefix comes in `x-forwarded-prefix` (an application that
writes absolute links needs it, or links that are relative):

```yaml
    - {port: http, listener: http, host: shop.local, path: /api, to: /}                   # /api/orders -> /orders
    - {port: http, listener: http, host: shop.local, path: /old, to: /v2}                 # /old/x -> /v2/x
    - {port: http, listener: http, host: shop.local, path: /healthz, pathType: exact}     # /healthz only
```

`karga gateway status` shows a route with its `to` (`shop.local/api → /`) or `(exact)`.

The scaffold from `karga create` exposes its Service with `--set expose.host=...`; the examples with
`--set service.listener=...`:

```console
$ karga upgrade demo shop --reuse-values --set expose.host=shop.local --dry-run
  ~ SERVICE web

$ karga upgrade demo shop --reuse-values --set expose.host=shop.local
Release "demo" upgraded (revision 3).
  update service web

$ curl -s -o /dev/null -w "%{http_code}\n" -H "Host: shop.local" http://127.0.0.1/
200

$ karga gateway status
Gateway:    running, ready (Envoy 1.36.13)
Image:      envoyproxy/envoy:distroless-v1.36.13@sha256:...
Listen:     127.0.0.1
Access log: off

LISTENER  ROUTE        RELEASE  SERVICE  UPSTREAM
http:80   shop.local/  demo     web:80   10.213.0.2:80
```

The release proxy publishes no port: the gateway reaches it on the `karga-edge` network at the
release's address (UPSTREAM above; `curl 10.213.0.2:80` from the host reaches the proxy directly).
Exposing a port, or changing a host or path, changes nothing of the proxy. Adding or removing a
Service needs a new proxy container (Docker sets a container's aliases when it joins a network): it
starts next to the old one at the release's other address, the gateway switches to it, and the
old one drains (up to 30s) and goes; no request is lost. An exposure the gateway
cannot serve is refused before anything changes:

```console
$ karga install other shop --set expose.host=shop.local
Error: Service "web": expose http on listener http: shop.local/ is already routed to demo/web: port already in use
revision 1 failed; nothing was installed. Fix the problem and run 'karga install' again

$ karga install db examples/postgres --set database.password=s3cret-pw --set service.listener=pg
Error: Service "postgres": expose postgres on listener pg: no such gateway listener; the host's administrator adds it with 'karga gateway listener add pg --port <port> --protocol tcp|http|https'
```

Applications behind http and https listeners receive the client address in `x-forwarded-for`, and
only it: a value the client sent is replaced, so it cannot pass for another address (behind a CDN or
load balancer, the address is theirs). A client must send its request headers within 10 seconds,
and a connection without requests is closed after an hour; https takes TLS 1.2 or newer. tcp
backends receive it with the listener's `--proxy-protocol`. Unless the listener sets a `--timeout`,
requests may take as long as they need (uploads, server-sent events, long polls) and WebSocket
connections pass through; a request or WebSocket that sends nothing either way for 5 minutes (the
listener's `--idle-timeout`) is closed.

### TLS

Expose on an https listener (`https` on a new host) with a Secret that holds the PEM certificate
chain as `tls.crt` and the key as `tls.key`. Plain HTTP on an http listener with `redirectHTTPS`
(the default `http` has it) redirects to it, unless that listener serves the host itself; each host
goes to the port of the https listener serving it:

```yaml
kind: Secret
metadata: {name: blog-cert}
spec:
  values:
    tls.crt: {{ .Values.tls.cert | quote }}     # from a values file kept outside git
    tls.key: {{ .Values.tls.key | quote }}
---
# in the Service
  expose:
    - {port: http, listener: https, host: blog.local, tls: {secret: blog-cert}}
```

```console
$ curl -s -o /dev/null -w "%{http_code}\n" --resolve blog.local:443:127.0.0.1 --cacert cert.pem https://blog.local/
200
$ curl -s -o /dev/null -w "%{http_code} %{redirect_url}\n" -H "Host: blog.local" http://127.0.0.1/
301 https://blog.local/
```

Mark the certificate values `writeOnly` in the schema. To renew a certificate, upgrade with the new
one; the gateway serves it after the upgrade. There is no automatic certificate (ACME) yet.

### Gateway access log

```console
$ karga gateway set access-log on
$ karga logs --gateway -f
[2026-11-17T04:58:08.117Z] "GET /logged HTTP/1.1" 404 - 0 124 2 1 "192.0.2.57" "Go-http-client/1.1" ...
```

It is off by default to keep the log small; turning it on or off does not interrupt traffic. A
request shows up within about a second. Times are UTC (the `Z`).

## 7. Upgrades without downtime

Ask for replace in the workload:

```yaml
healthCheck:
  type: http        # or tcp, or exec with a command
  port: 80
  path: /
  interval: 5s
  startPeriod: 10s  # failures in the first 10s do not count
updateStrategy:
  type: replace
  drainTimeout: 10s # how long open connections may finish on the old container
```

A replace:

```console
$ karga upgrade demo shop --set image.tag=1.27.3-alpine
[  0s] workload web: starting demo-web-3f9a2c1b next to demo-web-8b1d04e7
[  2s] workload demo-web-3f9a2c1b: waiting for demo-web-3f9a2c1b to be ready (...)
[  5s] workload web: switching traffic to demo-web-3f9a2c1b
[  5s] workload web: waiting up to 10s for connections to demo-web-8b1d04e7 to finish
[  5s] workload web: stopping demo-web-8b1d04e7
Release "demo" upgraded (revision 2).
  update workload web (replace)
```

Every client reaches the workload through its Service, that is through the release proxy, from the
release's networks and through the gateway alike. The proxy sends traffic to the new container only
once it is healthy, and the old one keeps its open connections until they finish or `drainTimeout`
passes. If the new
container never becomes healthy it is stopped and kept for its logs, and nothing that was running
changed.

Rules to know:

- **Health checks.** `exec` runs a command inside the container, so the image needs the tool (curl,
  wget, pg_isready...). `tcp` and `http` are run by karga from the host and need nothing in the
  image. The workload's check is the only one: the release proxy and the gateway check nothing
  themselves. A `tcp` check opens a connection and closes it without a word; services that punish
  those (sshd since OpenSSH 9.8, anything behind fail2ban) need an `http` or `exec` check.
- **Volumes.** A workload that mounts a volume read-write, or keeps an image VOLUME in an anonymous
  volume (see [section 8](#what-happens-to-data-on-an-upgrade)), is always recreated: two
  containers writing the same data are unsafe.
- **Release proxy.** `karga status` shows it, and each Service, with the backend it routes to:

  ```
  Proxy    proxy  demo-karga-proxy-d41a7c90  1  applied  running  0  -> karga-web-r1
  Service  web    demo-karga-proxy           1  applied  routed      -> karga-web-r1
  ```

  Adding or removing a Service briefly interrupts the proxy's open connections on that network.
- A workload no Service targets receives no traffic; a replace just starts the new container and
  stops the old one once it is healthy.

## 8. Databases and other stateful services

Databases mount a volume read-write, so they upgrade with **recreate** (a short outage) and keep
their data in the volume across upgrades, rollbacks and reinstalls:

```console
$ karga install db examples/postgres --set database.password=s3cret-pw --set service.listener=pg
Release "db" installed (revision 1).
  create network postgres-network
  create volume postgres-data
  create secret postgres-credentials
  create workload postgres
  create service postgres
  create proxy proxy

$ docker port db-postgres-a71c3e05         # the database publishes nothing
$ docker port db-karga-proxy-6e2b9f14      # nor does the proxy: the gateway reaches it on karga-edge
```

Other workloads of the release connect to `postgres:5432`; outside clients to the `pg` listener
(`127.0.0.1:15432` until `karga gateway set listen` says otherwise).

Volumes have `retention: retain` by default: `uninstall` keeps them and a later install of the same
release name reuses them, and a volume removed from the package stays in Docker. `retention: delete`
removes the volume when the release no longer has it, after a renamed volume, a removed one, or a
rollback to a revision without it. Such an upgrade or rollback stops before changing anything
unless you confirm with `--delete-volumes`:

```console
$ karga rollback s 1 --dry-run
Warning: this rollback deletes volume cache and its data; it needs --delete-volumes
  - VOLUME cache (its data is deleted)

$ karga rollback s 1
Error: this rollback deletes volume cache and its data (retention: delete); nothing was changed. Rerun with --delete-volumes to go ahead

$ karga rollback s 1 --delete-volumes
Release "s" rolled back (revision 3).
  delete volume cache (its data is deleted)
```

### What happens to data on an upgrade

- A Volume is named `<release>-<volume>` in Docker (`db-postgres-data`), whatever the revision.
  The new container mounts the same volume, so upgrades, rollbacks and reinstalls under the same
  release name see the same data. Changing a Volume's settings is refused by the plan.
- The old container is stopped before the new one starts: it gets SIGTERM (or the image's stop
  signal) and `stopTimeout` (default 10s) to shut down before it is killed. Databases that write a
  checkpoint on shutdown need more; `examples/postgres` sets `stopTimeout: 60s`. The same time
  applies to `uninstall`, `volume backup`, `volume restore` and `docker stop`.
- Data written anywhere else in the container is lost when it is recreated, unless the image
  declares that path as a `VOLUME`. Docker then keeps it in an anonymous volume, which karga moves
  to the new container on every upgrade. It is not a karga Volume: `uninstall` deletes it and
  `volume backup` cannot reach it, so karga warns:

  ```console
  $ karga install cache ./cache
  Warning: workload redis keeps image VOLUME /data in an anonymous volume: upgrades keep it, but uninstall deletes it and it is not in volume backups. Mount a Volume there.
  ```

  Mount a Volume on that path to keep the data like any other.
- Upgrades cannot undo what the new version does to the data (a database migration, a new file
  format); back up before upgrading across such versions.

### Back up and restore a volume

A consistent copy of an application that writes (a database) needs it stopped: the backup stops
the workloads writing to the volume, and those that depend on them, for the time of the copy.

```console
$ karga volume backup db postgres-data db.tar.gz
volume postgres-data of db written to db.tar.gz (postgres was down for 10.6s)

$ karga volume restore db postgres-data db.tar.gz
This stops the workloads using the volume and those that depend on them, replaces the files of volume postgres-data of db with db.tar.gz; its current files are kept as a restore point (karga volume undo-restore). Continue? [y/N] y
[  0s] workload postgres: stopping db-postgres-f83189ca
[  0s] volume postgres-data: keeping its files as a restore point
[  0s] workload postgres: removing db-postgres-f83189ca
[  0s] volume postgres-data: creating db-postgres-data again, empty
[  0s] workload postgres: creating db-postgres-f83189ca
[  1s] volume postgres-data: writing db.tar.gz
[  1s] workload postgres: starting db-postgres-f83189ca
[  3s] workload db-postgres-f83189ca: waiting for db-postgres-f83189ca to be ready (health starting, 2s)
...
[ 11s] workload postgres: start done
volume postgres-data of db restored from db.tar.gz (postgres was down for 10.9s)
Its previous files are kept: 'karga volume undo-restore db postgres-data' puts them back.

$ karga volume undo-restore db postgres-data
volume postgres-data of db is back as it was before its last restore (postgres was down for 10.7s)
```

- **What stops.** A backup stops the workloads that write to the volume; a restore every workload
  mounting it, read-only ones too. Both stop what depends on those through `dependsOn`, at any
  depth (an application whose database is backed up), dependents first. They start again in
  dependency order, each once those before it are ready (their health checks, as in a deployment),
  and the command returns when all are: the downtime it reports is from the first stop to the last
  ready, and `karga events` shows it (`down 10.6s`). A workload without a health check counts as
  ready once it runs.
- **Backups.** The copy goes to a temporary file under the server's `KARGA_HOME` (as large as the
  backup); the workloads start again, then the file is sent, so a slow download does not lengthen
  the downtime. `--live` copies while the workloads run, with no downtime, but an application that
  writes may leave the copy inconsistent: for a database use the default, or its own dump
  (`karga exec db postgres -- pg_dump -U app app > app.sql`). The file is mode 0600, must not exist
  yet, and names its release and volume. If a workload does not get ready again, the backup is
  complete and kept, and the command fails (exit 1) saying so.
- **Restores replace.** The volume ends up with exactly the backup's files: the containers mounting
  it are removed, the volume is created again, the backup written, and the same containers created
  again (their anonymous volumes kept). `--merge` writes the backup over the files instead, keeping
  those it does not have: in a database's volume that can mix newer files with an older backup.
  Restoring a backup of another volume or release needs `--from-other-volume`. The backup is read
  to its end before anything changes: a file cut short or damaged is refused, and the volume is
  untouched. If writing it fails anyway (Docker, a full disk), the volume may be empty or half
  written: its workloads stay stopped and the release is paused. Restore again or `karga volume
  undo-restore`, then `karga resume`.
- **Restore points.** Before it changes anything, a restore keeps the volume's files as its restore
  point in `KARGA_HOME/restore-points/<release>/<volume>.tar.gz` (one per volume, each restore
  replaces it); `karga volume undo-restore` puts them back the same way. Without room for it under
  `KARGA_HOME` the restore changes nothing and says so; `--no-restore-point` restores without one
  (the current files are then lost). `uninstall --delete-volumes` removes restore points with the
  volumes.
- **Interruptions.** A restore asks first (`--yes` in scripts), receives the whole backup and keeps it
  under `KARGA_HOME` until it is done: it goes to the end if the CLI goes away, and if karga-server
  stops in the middle it finishes the restore when it starts again (without taking the restore point
  again). If the kept backup is gone, the release is paused so that its workloads are not started on
  a half-written volume: restore again or `karga volume undo-restore`, then `karga resume`.

## 9. Secrets

There are two kinds of secret input:

- **Secret resources** become files mounted read-only in the container (e.g.
  `POSTGRES_PASSWORD_FILE: /run/secrets/postgres/password`), or environment variables of a
  workload with `environmentFrom`.
- **Values marked `"writeOnly": true`** in the package schema.

An application that reads its secrets only from the environment gets them from a Secret:

```yaml
kind: Workload
metadata: {name: app}
spec:
  image: shop:1.4
  environment:
    DATABASE_USER: app
  environmentFrom:
    - {name: DATABASE_PASSWORD, secret: db-credentials, key: password}
```

karga stores and shows only the reference; the value is set when the container is created, and a
new value in the Secret recreates the container (`update` in the plan). The value is visible to
anyone who can inspect the container through Docker (`docker inspect`): the host's administrators,
who can read the mounted files too. Prefer a mounted file where the image supports one (`*_FILE`);
`karga lint` and the plans of `install` and `upgrade` warn about every variable set this way.

What karga keeps:

| Where | Secret values |
|-------|---------------|
| Output, logs, `get values`, `get manifest`, `diff` | `<redacted>` or `(changed)` |
| State files (`$KARGA_HOME/state`) | HMAC digests keyed by `$KARGA_HOME/state.key`, to detect changes |
| `$KARGA_HOME/releases/<release>/<revision>/` | the files mounted into the container (directory 0700) |

`karga template` shows secrets only with `--show-secrets`: Secret values and `writeOnly` values are
`<redacted>` wherever they occur. A `writeOnly` value may only be used in a Secret (mount it, and
point the application at the file, e.g. `POSTGRES_PASSWORD_FILE`): a package that puts one in a
Workload's environment or a Config is refused, because karga stores and shows those. Values that
are not marked `writeOnly` are stored as written, so mark every password and token in your schemas.
`writeOnly` is followed under `additionalProperties`, `patternProperties` and `items`.

## 10. Resources and hardening

Defaults are written into every workload; set them in the package to change them:

```yaml
resources:
  memory: 512Mi     # also the swap limit: the container cannot swap beyond it
  cpu: 0.5
  pids: 1024        # default; 0 removes the limit
logging:
  maxSize: 10m      # default: the container log rotates at 10m, 3 files kept
  maxFiles: 3
security:
  noNewPrivileges: true          # default
  readOnlyRootFilesystem: true   # the image becomes read-only; /tmp stays writable (64m tmpfs)
  capDrop: [ALL]
  capAdd: [NET_BIND_SERVICE]
  user: "1000:1000"
```

A container that runs out of memory is killed and restarted; `karga status` shows it:

```
Workload  worker  shop-worker-6d2e9041  3  applied  running  4  healthy, oom-killed
```

An image that needs setuid (e.g. one calling `sudo`) needs `noNewPrivileges: false`. Upgrading a
release installed by an earlier karga applies its workloads once to add these defaults; the plan
lists the new fields.

## 11. Operate and troubleshoot

| Look at | Command |
|---------|---------|
| Desired versus actual state, health, restarts | `karga status <release> [--watch]` |
| Logs | `karga logs <release> [workload...] [-f] [--tail N]`, `--proxy`, `--gateway` |
| A shell or a command inside | `karga exec <release> <workload> [-- command]` |
| What changed | `karga history <release> --diff`, `karga diff <release> <rev>` |
| What karga and karga-server did | `karga events [release] [-f]` |
| The gateway | `karga gateway status` |

Tables are colored on a terminal (`--color never`, `NO_COLOR=1` or `TERM=dumb` turns it off).

### Output and exit codes for scripts

Every command takes `-o text` (the default, for people), `-o json` or `-o yaml`. Streams
(`status --watch`, `events -f`) write one JSON document per line, or YAML documents separated by
`---`. `logs` always writes the containers' lines. The JSON and YAML shapes are the API's views:
fields are only added within an API major version, never renamed or removed, so scripts may rely
on them.

Commands that only ask karga-server something give up after `--request-timeout` (default `30s`;
`0` waits for ever). Deployments, `uninstall`, `reconcile`, `logs`, `exec`, volume backup, restore
and undo-restore, `--watch`/`-f`, and the gateway commands that open an editor or ask first (`gateway
edit`, `apply`, `rollback`, `set listen`, `listener remove`) are not bounded: a timeout would end an
edit or run while you answer.

Commands that delete data or expose services ask first on a terminal: `uninstall`, `volume
restore`, `volume undo-restore`, `gateway set listen` with an address other than loopback, `gateway listener remove`, and
`upgrade`/`rollback` with `--delete-volumes`. In a script (stdin is not a terminal) they need
`--yes` (`-y`); `--dry-run` shows what they would do. karga-server refuses these requests unless
the client says the user agreed, so no client skips the question.

| Exit code | Meaning |
|---|---|
| 0 | Done; `karga status`: the release is deployed and healthy |
| 1 | Any other failure (`karga lint`: a package has errors; `karga reconcile`: a repair failed; a question answered no) |
| 2 | Wrong arguments, flags or subcommand, a confirmation needs `--yes`, or karga-server refused the request as given (an invalid name, package or values) |
| 3 | karga-server is not running, did not answer in time, went away, or is too old for the request (another API major, or a field or endpoint it does not know) |
| 4 | The release, revision or resource does not exist |
| 5 | Another operation holds the release, or it exists already |
| 6 | `karga status`: the release is not deployed and healthy (failed, missing, unhealthy or starting, or a Service routed nowhere) |
| 7 | karga stopped waiting (Ctrl-C) after its request went out: what it asked for goes on in karga-server (`karga events <release> -f` follows an operation) |
| other | `karga exec`: the command's own exit code |

### Common errors

| Error | Meaning and fix |
|-------|-----------------|
| `values do not match values.schema.json` | A value has the wrong type or range; fix the `--set` or file. |
| `no such gateway listener` | The Service exposes on a listener the host does not have; the administrator adds it (`karga gateway listener add`) or the package uses another one. |
| `... is already routed to R/S` / `the listener is used by R/S` | Another release's Service holds that host and path, or that tcp listener. |
| `...:N is taken by another process on this host` | Another program holds a listener's port on a listen address. |
| `is a tcp listener and takes no host` / `needs a host` / `needs tls.secret` | The exposure does not fit the listener's protocol. |
| `image ... was not found in its registry` | Wrong repository or tag; nothing running changed. |
| `workload X is unhealthy: http check on ... failed` | The application did not answer its health check; `karga logs` the stopped container. |
| `workload X keeps restarting (N restarts, last exit code C)` | The container crashes as it starts; `karga logs` it. |
| `... copied, but workload W did not get ready` / `... restored, but ...` | The volume operation did its work (the backup file is complete and kept, the files are in place); W failed its health check after starting again: `karga logs` it. |
| `unreadable` in `karga list` / `.../release.yaml: yaml: ...` or `is incomplete` | The release's state file under `$KARGA_HOME/state` is damaged (a disk error, a hand edit); `karga status R` names it. Put it back from a backup of `$KARGA_HOME`; the other releases are not affected. |
| `copy volume V: write .../tmp/karga-backup-...: no space left on device` | A backup is copied under the server's `KARGA_HOME` first, as large as the archive, wherever its file goes: free space there, or use `--live` (no copy there). |
| `... is cut short or damaged` | The backup file is incomplete (a copy that stopped early) or corrupt; nothing was changed. |
| `... may be empty or half written, so ... stay stopped and R is paused` | The restore failed writing the volume; restore again or `karga volume undo-restore`, then `karga resume R`. |
| `volume V is missing in Docker, so its files are gone` | The volume was removed outside karga or its data lost: `karga reconcile R --empty-volumes`, then `karga volume restore R V <backup>`. |
| `not enough space under ... for a restore point` | Free space under `KARGA_HOME`, or restore with `--no-restore-point`; nothing was changed. |
| `volume V of R has no restore point` | `undo-restore` needs a restore earlier: a restore keeps one. |
| `... and workload W is down: its revision N container was replaced` | A recreate failed after the old container went: run the `karga rollback <release> N` it names. |
| `another operation is running on this release` | Another karga command works on the release; wait for it. |
| `a previous operation on this release was interrupted` | An operation was left by a process that is gone (karga-server finishes its own when it starts); check `karga status`, then rerun with `--force`. |
| `gateway: proxy does not route to ... : <reason>` | The gateway did not accept or reach the new backend; the reason (and Envoy's own message, if it refused the configuration) follows. |
| `services ... need karga's gateway and release proxy (Linux with a local, rootful Docker)` | karga runs on Linux with a local, rootful Docker daemon. |
| `karga speaks API N, karga-server API M` | The CLI and the server are of incompatible releases; upgrade the one named. |
| `karga-server is not running` | Start it: `sudo systemctl start karga-server`; see `journalctl -u karga-server`. |
| `permission denied` on `/run/karga/karga.sock` | Your user is not in the `karga` group (or has not logged in again since). |
| `a Workload no longer exposes ports; a Service does` | The package uses the removed `hostPort`, `hostIP` or `routes`; move them to a Service. |

### Repairs: karga-server keeps releases as declared

karga-server brings what runs back to each release's current revision, on its own: after Docker
reports a container died, was stopped, removed or became unhealthy, and every 30 seconds.

| It sees | It does |
|---------|---------|
| A container, network or release proxy is gone | Creates it again from the current revision |
| A volume is gone | Nothing: its files are gone, and its workloads would start empty. The repair fails and says so (below) |
| A container was recreated by hand from other settings | Recreates it from the current revision |
| A container is stopped | Starts it |
| A container is unhealthy (Docker's check, or karga's tcp/http check failing `retries` times) | Restarts it |
| Something is exposed and the gateway is down | Starts the gateway |

```console
$ docker rm -f demo-web
$ docker ps --filter name=demo-web --format "{{.Names}} {{.Status}}"
demo-web Up 1 second

$ karga events demo
TIME                        RELEASE  OPERATION  REVISION  USER          STATUS     WHAT
2026-11-17 19:01:09 +03:00  demo     install    1         alice         succeeded  create network net, create workload web, create service web, create proxy proxy
2026-11-17 19:01:17 +03:00  demo     reconcile  1         karga-server  succeeded  create workload web

$ karga status demo
...
Repaired: 2026-11-17 19:01:17 +03:00, succeeded
```

What it never does:

- It does not change the revision, the values or the package: an upgrade or rollback is yours.
- It leaves a **failed** release alone (after a failed recreate the release waits for your
  rollback or upgrade, D2), and a release an operation is working on.
- It does not delete containers it does not know (`karga status` lists them under "Not tracked by
  karga").
- It does not create a volume that is gone (removed by hand, Docker's data lost, `$KARGA_HOME` moved
  to another host). What mounts it waits too, and `karga status` says why:

  ```console
  $ karga status cache
  ...
  Repaired: 2026-11-24 00:14:48 +03:00, failed: Volume redis-data: create: volume redis-data is missing in Docker, so its files are gone; karga does not create it empty on its own. 'karga reconcile cache --empty-volumes' creates it, then 'karga volume restore cache redis-data <backup>' puts a backup back
  ```

  `karga reconcile <release> --empty-volumes` creates it, empty, with what mounts it; `karga volume
  restore` then puts a backup back:

  ```console
  $ karga reconcile cache --empty-volumes
  Release "cache" repaired:
    create volume redis-data: done
    create workload redis: done
  ```

A repair that fails is retried with growing pauses (10 seconds doubling to 5 minutes); so is one
needed again within 10 minutes (a container that crashes right after each start), which counts as
failed. After 5 failures in 10 minutes karga-server gives up on it; `karga status` says so and why.
It tries again once those failures are 10 minutes old. `karga reconcile <release>` repairs now and
forgets the failures:

```console
$ karga reconcile demo
Release "demo" repaired:
  start workload web: done
```

For maintenance, stop the repairs of a release, and start them again afterwards:

```console
$ karga pause demo
Release "demo" paused: karga-server does not repair it.
$ docker stop demo-web                    # stays stopped
$ karga resume demo
Release "demo" resumed: karga-server will repair it.
```

`karga events [release] [-f]` lists every command and repair, newest last; `journalctl -u
karga-server` shows the server's side: each operation's start and end, a failed one at warning
level (`journalctl -u karga-server -p warning`).

## 12. Back up and move a host

A complete backup is `$KARGA_HOME` (the state, its key, the gateway settings and the revisions'
Config and Secret files) and the volumes:

```console
$ sudo systemctl stop karga-server
$ sudo tar -C /var/lib -czf ~/backup/karga-home.tar.gz karga
$ sudo systemctl start karga-server
$ karga volume backup db postgres-data ~/backup/db-postgres-data.tar.gz
```

Restore on a new host, in this order:

1. Install karga and Docker.
2. Stop karga-server, unpack the backup into `/var/lib` (owner `karga`) and start it again.
   `karga list` and `karga history` now show your releases. karga-server does not create their
   volumes on its own (they would be empty): until step 3, `karga status` shows the repair failing.
3. For each release, upgrade from its package; the plan creates everything missing on this host:

   ```console
   $ karga upgrade demo shop -f values.yaml
   Release "demo" upgraded (revision 3).
     create network net
     create workload web
   ```

4. Restore the volumes with `karga volume restore`.

Without the old `$KARGA_HOME` (a fresh host), install each release and restore its volumes; only
the history is lost.

Without `state.key`, karga creates a new one and each release applies its Secrets once more on its
next upgrade. Keep the backup safe: it holds your values and Secrets.

## 13. Write a package

```
shop/
  Package.yaml          name, version, runtime.supported: [docker]
  values.yaml           defaults, with comments
  values.schema.json    types, ranges, "writeOnly": true for secrets
  templates/web.yaml    resources, separated by --- (a Workload, its Service, ...)
  templates/_helpers.tpl named templates ({{ define "url" }} ... used with include)
  NOTES.txt             shown after install and upgrade
  files/                read with .Files.Get "name"
```

In `templates/`, files whose name starts with `_` only hold named templates, and files whose name
starts with `.` are ignored (macOS `._*` and `.DS_Store`, editor files).

Templates use Go templates with Sprig. The context is `.Values`, `.Release.Name`,
`.Release.Revision`, `.Package.Name`, `.Package.Version` and `.Files`; `include`, `tpl`,
`required`, `toYaml` and `fromYaml` are there. Functions that read the host or are not
deterministic (`env`, `now`, `randAlpha`, `genPrivateKey`, ...) fail the render, so a package and
its values always give the same result.

Check your work without touching Docker:

```console
$ karga lint shop                      # metadata, schema, render, every resource
$ karga template demo shop -f my.yaml  # what it renders
$ karga install demo shop --dry-run    # what it would do
```

A resource that does not decode names its file, document and line in the rendered output; look at
that line with `karga template`. It renders revision 1 of the release (`--revision N` another, as
`.Release.Revision`) from the values given, not the release's stored ones: `karga upgrade <release>
<package> --dry-run -o yaml` shows what an upgrade would deploy. The package format is
`karga.io/v1alpha1` and may still change.

A workload that needs another one at start (an application and its database) waits for its
Service with `dependsOn`:

```yaml
kind: Workload
metadata: {name: app}
spec:
  image: shop:1.4
  networks: [backend]
  dependsOn: [postgres]          # Services of the release
  environment:
    DATABASE_URL: postgres://app@postgres:5432/app
```

karga starts `app` only once the `postgres` workload is ready (its health check passes) and the
release proxy routes to it; if `postgres` never gets ready, `app` is not started. Upgrades and
repairs follow the same order. Changing `dependsOn` alone changes no container, and a cycle is
refused (`circular dependency between ...`). Your application should still retry its connections:
`dependsOn` orders the start, it cannot keep a database up later.

Give every workload other workloads or users talk to a Service, and expose it through a values
setting (a listener name, a host) rather than a fixed one: the listeners are the host's, and the
person installing the package knows them (`karga gateway listener list`).

## 14. Uninstall and clean up

```console
$ karga uninstall db --dry-run          # what would be removed
$ karga uninstall db
This removes release db and its containers. Continue? [y/N] y
Release "db" uninstalled.
$ docker volume ls --filter label=io.karga.release=db --format '{{.Name}}'
db-postgres-data
```

- Volumes are kept (reinstalling `db` reuses them); `--delete-volumes` removes them.
- `--keep-history` keeps the release's revisions in the state.
- The release's operations stay, as a record of what was done and by whom: `karga events db` lists
  them, the uninstall included, also after `db` is installed again (newest 200). `karga status`
  of a release installed again shows its own repairs only.
- The release proxy goes with the release; the gateway goes when nothing is exposed anymore. The
  gateway's listeners and listen addresses stay: they are the host's settings.

## 15. Put your own application online

This section takes one application from nothing to a domain: an HTTP API
([PostgREST](https://postgrest.org), a public image that reads its database address from the
environment) and its PostgreSQL database, on one host. Replace the API's image with your own; the
rest stays the same.

### The package

```
notes/
  Package.yaml
  values.yaml
  values.schema.json
  templates/notes.yaml
  NOTES.txt
```

`Package.yaml`:

```yaml
apiVersion: karga.io/v1alpha1
name: notes
version: 1.0.0
description: A REST API (PostgREST) over PostgreSQL
runtime:
  supported: [docker]
```

`values.yaml` holds what changes between installs; the password has no default:

```yaml
# The application's image.
image: postgrest/postgrest:v12.2.3
# Where it is served: a host name on a gateway listener (karga gateway listener list).
host: notes.example.com
listener: http
database:
  # Required: --set database.password=... or a values file kept out of git.
  password: ""
```

`values.schema.json` checks them and marks the password `writeOnly`, so karga never stores or
shows it (section 9):

```json
{
  "type": "object",
  "properties": {
    "image": {"type": "string", "minLength": 1},
    "host": {"type": "string", "minLength": 1},
    "listener": {"type": "string", "minLength": 1},
    "database": {
      "type": "object",
      "properties": {"password": {"type": "string", "minLength": 8, "writeOnly": true}},
      "required": ["password"]
    }
  },
  "required": ["image", "host", "listener", "database"]
}
```

`templates/notes.yaml`:

```yaml
apiVersion: karga.io/v1alpha1
kind: Network
metadata: {name: backend}
---
apiVersion: karga.io/v1alpha1
kind: Secret
metadata: {name: db}
spec:
  values:
    password: {{ .Values.database.password | quote }}
    uri: "postgres://app:{{ .Values.database.password | urlquery }}@postgres:5432/app"
---
apiVersion: karga.io/v1alpha1
kind: Volume
metadata: {name: db-data}
spec: {retention: retain}
---
apiVersion: karga.io/v1alpha1
kind: Workload
metadata: {name: postgres}
spec:
  image: postgres:17
  stopTimeout: 60s
  environment:
    POSTGRES_DB: app
    POSTGRES_USER: app
    POSTGRES_PASSWORD_FILE: /run/secrets/db/password
  networks: [backend]
  ports:
    - {name: postgres, containerPort: 5432}
  mounts:
    - {secret: db, mountPath: /run/secrets/db}
    - {volume: db-data, mountPath: /var/lib/postgresql/data}
  healthCheck:
    type: exec
    command: [pg_isready, -U, app, -d, app]
    interval: 5s
    timeout: 5s
    retries: 5
---
apiVersion: karga.io/v1alpha1
kind: Service
metadata: {name: postgres}
spec:
  workload: postgres
  ports:
    - {name: postgres, port: 5432}
---
apiVersion: karga.io/v1alpha1
kind: Workload
metadata: {name: api}
spec:
  image: {{ .Values.image | quote }}
  dependsOn: [postgres]
  networks: [backend]
  environment:
    PGRST_DB_ANON_ROLE: app
    PGRST_DB_SCHEMAS: public
    PGRST_ADMIN_SERVER_PORT: "3001"
  environmentFrom:
    - {name: PGRST_DB_URI, secret: db, key: uri}
  ports:
    - {name: http, containerPort: 3000}
    - {name: admin, containerPort: 3001}
  healthCheck:
    type: http
    port: 3001
    path: /ready
    interval: 5s
    timeout: 2s
    retries: 3
  updateStrategy:
    type: replace
    drainTimeout: 10s
---
apiVersion: karga.io/v1alpha1
kind: Service
metadata: {name: api}
spec:
  workload: api
  ports:
    - {name: http, port: 80, targetPort: http}
  expose:
    - {port: http, listener: {{ .Values.listener }}, host: {{ .Values.host }}}
```

What it does:

- **Secret `db`** holds the password and the connection URI built from it. Postgres reads the
  password from a mounted file (`POSTGRES_PASSWORD_FILE`); the API takes the URI as a variable
  (`environmentFrom`), because it reads it only from the environment.
- **Volume `db-data`** keeps the database across upgrades and uninstalls (`retention: retain`).
- **Service `postgres`** is how the API reaches the database (`postgres:5432` in the URI);
  nothing outside the release can reach it.
- **Workload `api`** starts only once Postgres is ready (`dependsOn`). Its health check is karga's
  (`type: http`, the image has no shell), so upgrades replace it without downtime (section 7).
- **Service `api`** exposes it on the `web` listener for the host name in `host`.
- `NOTES.txt` is printed after each install and upgrade:
  `{{ .Release.Name }} serves http://{{ .Values.host }}/ through the gateway listener {{ .Values.listener }}.`

### The host

Once per host, as a member of the `karga` group:

```console
$ karga gateway listener list
NAME   PORT  PROTOCOL  OPTIONS            SERVICES
http   80    http      redirect-https=on
https  443   https     -
$ karga gateway set listen 0.0.0.0        # every interface; the default 127.0.0.1 is local only
```

Point the domain's DNS record (`A` or `AAAA`) at the host and let port 80 (and 443 for https)
through its firewall.

### Install

Keep the password in a values file outside the package and outside git:

```console
$ cat > notes-secrets.yaml <<'YAML'
database:
  password: "correct-horse-battery"
YAML
$ chmod 600 notes-secrets.yaml

$ karga lint notes -f notes-secrets.yaml
1 package(s) linted, 0 failed
$ karga install notes notes -f notes-secrets.yaml --dry-run
Deployment Plan

Release:  notes
Revision: 1

  + NETWORK backend
  + VOLUME db-data
  + SECRET db
  + WORKLOAD postgres
  + SERVICE postgres
  + WORKLOAD api
  + SERVICE api
  + PROXY proxy

8 to create, 0 to update, 0 to delete, 0 unchanged.

$ karga install notes notes -f notes-secrets.yaml
[  0s] workload postgres: image postgres:17 (if-not-present)
[  0s] workload api: image postgrest/postgrest:v12.2.3 (if-not-present)
[ 25s] workload postgres: creating notes-postgres
[ 28s] workload notes-postgres: waiting for notes-postgres to be ready (health starting, 2s)
[ 31s] workload api: waiting for postgres
[ 31s] workload api: creating notes-api
[ 33s] workload notes-api: waiting for notes-api to be ready (GET /ready: 503 Service Unavailable, 2s)
[ 36s] gateway karga-gateway: updating the host gateway
Release "notes" installed (revision 1).
  ...

NOTES:
notes serves http://notes.example.com/ through the gateway listener http.

$ karga status notes
KIND      NAME      RUNTIME NAME                REVISION  DESIRED  ACTUAL   RESTARTS  HEALTH/BACKEND
Network   backend   notes-backend               1         applied  present
Proxy     proxy     notes-karga-proxy-6e2b9f14  1         applied  running  0         -> karga-api-r1, karga-postgres-r1
Secret    db        notes-db                    1         applied  file
Service   api       notes-karga-proxy           1         applied  routed             -> karga-api-r1
Service   postgres  notes-karga-proxy           1         applied  routed             -> karga-postgres-r1
Volume    db-data   notes-db-data               1         applied  present
Workload  api       notes-api-5c07e19a          1         applied  running  0         healthy
Workload  postgres  notes-postgres-a71c3e05     1         applied  running  0         healthy
```

Create a table and use the API (PostgREST reloads its schema on `NOTIFY pgrst`):

```console
$ karga exec notes postgres -- psql -U app -d app -c "create table notes (id serial primary key, body text not null); notify pgrst, 'reload schema';"
CREATE TABLE
NOTIFY
$ curl -s -H "Content-Type: application/json" -d '{"body":"hello"}' -w "%{http_code}\n" http://notes.example.com/notes
201
$ curl -s http://notes.example.com/notes
[{"id":1,"body":"hello"}]
```

### Upgrade, look back, roll back

Upgrade with the same values file and what changes; read the plan first:

```console
$ karga upgrade notes notes -f notes-secrets.yaml --set image=postgrest/postgrest:v12.2.8 --dry-run
Release:  notes
Revision: 1 -> 2

  ~ WORKLOAD api (replace)

0 to create, 1 to update, 0 to delete, 7 unchanged. --verbose shows the changed fields.
$ karga upgrade notes notes -f notes-secrets.yaml --set image=postgrest/postgrest:v12.2.8
Release "notes" upgraded (revision 2).
  update workload api (replace)

$ karga history notes
REVISION  STATUS      PACKAGE  VERSION  CREATED                     DESCRIPTION
1         superseded  notes    1.0.0    2026-11-18 12:00:42 +03:00  Install complete
2         deployed    notes    1.0.0    2026-11-18 12:02:36 +03:00  Upgrade complete
$ karga rollback notes 1
Release "notes" rolled back (revision 3).
  update workload api (replace)
```

The data stays: the volume is not part of the change, and the API is replaced next to the old one.
A new package version (a new `version` in `Package.yaml`) is upgraded the same way.

### Day to day

- `karga status notes` (exit code 6 when something is not as declared) and `karga logs notes -f`.
- karga-server repairs what goes missing (section 11, "Repairs").
- Back up the database volume (section 8) and `$KARGA_HOME` (section 12).
- HTTPS: add an exposure on `https` with a certificate Secret (section 6, "TLS").
- Remove it all: `karga uninstall notes` keeps the volume; `--delete-volumes` removes it too.

## 16. Limits to know

- karga needs Linux with a local Docker daemon.
- Services carry TCP only; a release's Service ports must be distinct numbers.
- Workloads on the same network of a release can reach each other by IP (and Docker resolves
  container names); there is no network policy.
- The gateway runs as root (with only the capabilities to bind ports) and is a single point for
  outside traffic; local users can reach the loopback ports behind it.
- Images are pulled anonymously; private registries are not supported yet.
- Packages come from local directories or archives; there is no repository or dependency support.
- There are no lifecycle tasks (e.g. running migrations before an upgrade) yet.
- Volume backups are files you keep: karga takes none on a schedule and keeps no history of them
  (one restore point per volume). A consistent backup stops the volume's workloads: there are no
  backup hooks (a database's own dump without stopping it) or filesystem snapshots yet.
- A workload is one container; there are no replicas.
- An upgrade that recreates a workload stops the old container before the new one starts: if the
  new one does not get ready, the workload is down (stopped, so its logs can be read) until you upgrade again or
  `karga rollback`. The `replace` strategy avoids this where it applies (section 7).
- TLS certificates come from Secrets: you get and renew them yourself (renew by upgrading with the
  new one); there is no ACME.
- A request takes at most 1000 `--set`, 100 `-f` files of 4 MiB each, 10000 events and 100000 log
  lines per container (`--tail`).

The full list, with what is planned, is in karga-server's [docs/roadmap.md](https://github.com/hasankaradmir/karga-server/blob/master/docs/roadmap.md).

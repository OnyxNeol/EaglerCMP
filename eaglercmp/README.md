# EaglerCMP — EaglerCraft with Mods and Performance

A small Go daemon that turns an Eaglercraft web client into a desktop app. It
installs the client locally, serves it from a loopback HTTP server, and opens
it in a dedicated window running the **Sodium HX Graphics (NaOHX)**
performance profile. There is no account, no Microsoft login and no Mojang
download. Singleplayer works fully offline, and multiplayer uses the client's
own WebSocket (`ws://` / `wss://`) connections to Eaglercraft servers.

> **Sodium HX Graphics / NaOHX** is a software brand name for EaglerCMP's
> window performance profile. It has no chemical meaning. The name nods to
> [Sodium by CaffeineMC](https://github.com/CaffeineMC/sodium); EaglerCMP
> contains no CaffeineMC code. See [NOTICE.md](NOTICE.md).

## How it works

```
eaglercmp launch
  ├─ verify client/ against .eaglercmp-client.json (SHA-256 per file)
  ├─ serve client/ on http://127.0.0.1:47262/ (loopback only)
  │    └─ entry page gets /__eaglercmp/naohx.js injected into <head>
  │         (window title, configured servers + eaglercraftXOpts overrides)
  └─ open a Chromium-engine app window (Edge / Chrome / Chromium / Brave)
       ├─ dedicated profile in <instance>/profile (worlds live in its IndexedDB)
       ├─ NaOHX flags: GPU rasterisation, ignore GPU blocklist, zero-copy,
       │  no background/occlusion throttling
       └─ page console streamed to the terminal as [client/err] lines
```

Eaglercraft 26.2 is a TeaVM WebAssembly-GC build, so the window needs a
browser engine with Wasm-GC support. EaglerCMP uses an installed
Chromium-based browser in `--app` mode, which gives a chrome-less window
without CGO, so the Go binary still cross-compiles from any OS. Every Windows
10/11 machine has Edge. If no Chromium-based browser is found, EaglerCMP opens
the default browser instead (Firefox 120+ and Safari 18.2+ also run Wasm-GC).

## The Eaglercraft client

The default client is Eaglercraft 26.2 from the upstream repo,
[`eymenwsmc/playopspt.github.io`](https://github.com/eymenwsmc/playopspt.github.io),
folder `262/`. That folder's `index.html` redirects to the current
self-contained single-file build, and the importer follows the redirect.

- **Release builds** (GitHub Actions) fetch that client once per run and
  embed it in the binary (`-tags=bundled`, about 54 MB compressed). The first
  launch unpacks it with no network access.
- **Plain builds** (`scripts/build.sh`) embed nothing. On first launch they
  download the client from the upstream repo.
- `eaglercmp update` re-downloads the client from the upstream repo, which
  picks up a new build when the repo's redirect changes.

To use a different build, import it; it is then remembered as `clientSource`:

```bash
eaglercmp import ~/Downloads/eaglercraft-26.2.html   # single-file offline build
eaglercmp import ./my-eaglercraft-build/             # static folder (index.html + assets)
eaglercmp import ./client.zip                        # .zip / .tar.gz of such a folder
eaglercmp import -sha256 <hex> https://example.org/eaglercraft.html
```

To bundle a client into a local build, do what CI does:

```bash
go run . import -dir /tmp/c https://raw.githubusercontent.com/eymenwsmc/playopspt.github.io/main/262/index.html
tar -czf bundle/client.tar.gz -C /tmp/c/client --exclude=.eaglercmp-client.json .
echo "upstream 262/" > bundle/source.txt
GOFLAGS=-tags=bundled scripts/build.sh --target windows/amd64
```

On Windows, `eaglercmp.exe` is built as a GUI app: double-clicking it opens
only the game window, with no terminal. Launcher output goes to
`%APPDATA%\EaglerCMP\logs\`, and errors are shown in a dialog box. Closing the
game window stops EaglerCMP. If no client can be installed (a plain build with
no network, for example), it opens a file picker. Run from a terminal, the
exe still prints to that terminal (`cmd` returns to the prompt immediately;
PowerShell's `Start-Process -Wait` or Git Bash wait for it).

The importer picks the page that boots the game (an HTML file defining
`eaglercraftXOpts`): `index.html` if it is one, else the largest such file.
Launcher sites and menus are never used; an installed client that is not a
game page is replaced with the bundled one. It
follows pages that only `<meta refresh>` to another http(s) URL, up to 3 hops.
The import replaces the old client atomically and is recorded as
`clientSource` in the config, so `eaglercmp init` can reinstall it later.

## Build and run

You need Go 1.22 or newer. The code uses only the standard library.

```bash
# Linux / macOS
scripts/build.sh                          # vet, test, build ./bin/eaglercmp
scripts/build.sh --init ./client.html     # ...then import a client
scripts/build.sh --all                    # cross-compile into ./dist (linux/windows/darwin × amd64/arm64)

# Windows (PowerShell)
.\scripts\build.ps1 -Init -Source .\client.html
```

```bash
eaglercmp                     # same as `eaglercmp launch`
eaglercmp launch -dry-run     # print the window command
eaglercmp launch -no-window   # only serve; open http://127.0.0.1:47262/ yourself
eaglercmp status              # client, verification state, browser, servers
eaglercmp update              # re-download the client (clientSource or upstream repo)
eaglercmp credits
```

Flags: `-dir <path>` (instance directory; or set `EAGLERCMP_HOME`),
`-port`, `-browser <path|system>`, `-skip-verify`, `-verbose` (show all
browser-engine log lines, not just page console output).

Default instance directory: Linux `~/.local/share/eaglercmp`, macOS
`~/Library/Application Support/EaglerCMP`, Windows `%APPDATA%\EaglerCMP`.

## Real Java client mods (`eaglercmp java-client`)

Runs the actual Minecraft 26.2 desktop client with Fabric Loader in a local JVM, so unmodified Fabric client mods (Sodium, Physics Mod, ...) work. It needs Java 25+ on PATH (or `javaClient.java`).

- First run downloads Minecraft 26.2, Fabric Loader, assets and Fabric API (Modrinth) into `<instance>/javaclient/`.
- Drop Fabric client `.jar` files in `<instance>/javaclient/game/mods/`. Fabric Loader discovers them there itself; they must not be put on the JVM classpath by hand. Fabric API is installed there automatically.
- **Offline session:** `-username Name` (3-16 letters/digits/`_`, default `Player`) with a deterministic offline UUID and a dummy token; no Microsoft login. This only works against offline-mode servers such as the local Velocity/Fabric stack (`127.0.0.1:25565`, or `javaClient.server`). Use your own licensed copy of Minecraft; the client is fetched from Mojang's public download servers.
- `-dry-run` prepares the files and prints the JVM command without starting the game. Config: `javaClient` in `eaglercmp.json` (`version`, `username`, `server`, `java`, `jvmArgs`, `dir`).
- Release archives include `java-client.bat` / `java-client.sh`.

## Configuration (`<instance>/eaglercmp.json`)

```json
{
  "clientSource": "/home/me/Downloads/eaglercraft-26.2.html",
  "port": 47262,
  "browser": "",
  "width": 1280,
  "height": 720,
  "fullscreen": false,
  "performance": true,
  "servers": [
    { "name": "My Server", "addr": "wss://play.example.net" }
  ],
  "clientOptions": { "allowBootMenu": false }
}
```

- `port` must stay the same. The browser engine stores worlds, settings and
  resource packs per origin (`http://127.0.0.1:<port>`), so a new port starts
  with an empty profile. A busy port also means EaglerCMP is already running.
- `browser`: path to a Chromium-based browser, `"system"` for the default
  browser, or empty for auto-detect. You can also set `EAGLERCMP_BROWSER`.
  `browserArgs` adds extra flags.
- `performance` turns the NaOHX profile on or off.
- `servers` is put in front of `eaglercraftXOpts.servers`, and
  `clientOptions` keys are merged into `eaglercraftXOpts`, before the client
  reads them. Builds that ignore a given option are unaffected.

## Layout

```
main.go              CLI (launch / import / init / status / credits)
config/              eaglercmp.json schema, instance layout, NaOHX branding + credits
downloader/          HTTP + SHA-256 verification, safe archive unpacking, client import + manifest
manager/             loopback asset server + bootstrap injection, browser discovery,
                     window process supervisor + log streaming
scripts/             build.sh, build.ps1, package.sh

<instance>/
  eaglercmp.json  naohx.json
  client/         installed web client + .eaglercmp-client.json
  profile/        dedicated browser-engine profile (IndexedDB worlds, localStorage)
  cache/          downloaded client sources
  logs/           launcher.log, client-<timestamp>.log
```

## Security notes

- The server binds to `127.0.0.1` only and rejects any request whose `Host`
  is not `127.0.0.1:<port>` or `localhost:<port>`, which blocks DNS
  rebinding. It serves only GET/HEAD, with no directory listings, and keeps
  the manifest hidden.
- Archives are unpacked with path-traversal and symlink-escape checks.
- Every launch re-hashes the client against the manifest (about 50 ms for a
  72 MB build), unless you pass `-skip-verify`.

## Releases (CI)

`.github/workflows/release.yml` runs on pushes to `main`, `v*` tag pushes and manual dispatch:

1. `test` job: `gofmt`, `go vet`, `go test`. `client` job: fetches the Eaglercraft client from the
   upstream repo (or the `client_source` input) into `bundle/client.tar.gz`.
2. `build` matrix: linux/amd64 (Ubuntu), windows/amd64 (Windows, via `scripts/build.ps1`),
   darwin/amd64 and darwin/arm64 (macOS, via `scripts/build.sh`). Each target is built as
   `bin/eaglercmp[.exe]` with `CGO_ENABLED=0`, `-tags=bundled` and the tag stamped into the version,
   smoke-tested (including installing the bundled client) on
   its native runner where possible, then packaged by `scripts/package.sh` with `README.md` and
   `NOTICE.md` (`.zip` for Windows, `.tar.gz` otherwise) and uploaded as a workflow artifact.
3. `release` job (tags only): attaches all archives plus `SHA256SUMS.txt` to a GitHub Release.
   Tags containing `-` (e.g. `v0.2.0-rc.1`) are marked as pre-releases.

Release archives contain the launcher (with the embedded client), `README.md` and `NOTICE.md`.
Every run also uploads the archives as workflow artifacts, so a push to `main` gives you test
builds without cutting a release.

```bash
git tag v0.2.0 && git push origin v0.2.0
```

Manual runs build and upload artifacts only; they never publish a release.

To reproduce one target locally:
`VERSION=v0.2.0 scripts/build.sh --target darwin/arm64 && scripts/package.sh darwin arm64 v0.2.0`.

## Development

```bash
go vet ./... && go test ./...
```

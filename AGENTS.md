# qbox

Windows 10/11 x64 VPN/proxy client. Go 1.27.4, native [MyGo](https://github.com/egoist/mygo) UI (`v0.10.0`, no webview), and sing-box `v1.14.4` linked in-process. Module `qbox`; the app is a single root package.

## Commands

Run these from PowerShell on Windows. The scripts set the required sing-box build tags (`with_gvisor,with_quic,with_utls,with_grpc`); build, test, check, and installer also set `CGO_ENABLED=0`. Builds or tests without those tags lack the mixed TUN stack, Hysteria2/TUIC, and uTLS/REALITY support.

- `.\scripts\test.ps1` — full check: `gofmt`, all packages except `internal/app`, UI tests compiled to `build/test`, `go vet ./...`, `go tool mygo vet .`.
- `.\scripts\build.ps1` — `build/windows-amd64/qbox.exe`.
- `.\scripts\check.ps1` — `staticcheck` and `govulncheck`.
- `.\scripts\dev.ps1` — MyGo dev mode.
- `.\scripts\installer.ps1` — NSIS installer in `build/windows-amd64/`; version comes from `mygo.json`.

For focused tests, set the tags first:

```powershell
$env:GOFLAGS = "-tags=with_gvisor,with_quic,with_utls,with_grpc"
$env:CGO_ENABLED = "0"
go test -count=1 ./internal/subscription -run TestParseVLESS
go test -count=1 ./internal/boxconf
```

UI tests get special handling because Windows Defender can block newly linked test executables in `%TEMP%`:

```powershell
go test -c -o build/test/app.test.exe ./internal/app
.\build\test\app.test.exe -test.v -test.run TestViewAndStart
```

CI runs `test.ps1` → `build.ps1` → `check.ps1` on `windows-latest`.

## Architecture

- `main.go` creates the config store, engine, rule-set manager, proxy manager, and `app.Deps`, then starts MyGo. It enforces a single instance. TUN mode relaunches the process elevated with `--elevated`, `--autostart`, and `--wait-pid=`; preserve those flags.
- `internal/app` is the native UI plus orchestration. Engine, network, and ping work runs in goroutines; UI state must only be changed through `a.post` / `win.Update`. Ping and fetch jobs are cancelable and generation-guarded. Window close hides to the tray; `BeforeQuit`/`Shutdown` must stop the engine and restore the system proxy.
- `internal/core` owns exactly one sing-box instance and serializes `Start`/`Stop`/`Restart`/`Close` with `opMu`. Ping uses short-lived sing-box instances tracked so `Close` can stop them. Node and routing-profile changes restart the core and drop connections; other routing edits apply on the next explicit restart.
- `internal/boxconf` converts config into sing-box JSON (not option structs), including DNS, TUN/mixed inbound, rule-sets, and route rules. Rule order is: sniff → qbox process direct → adblock → LAN → custom proxy lists → custom direct lists → RU bypass → final. Direct and RU-bypass domains resolve through the system resolver; other traffic uses remote DNS. Update `boxconf` tests when route behavior changes.
- `internal/config` is the versioned persisted schema (`Version = 3`) with `Normalize`/migration logic, atomic save, and `config.json.bak` fallback. `config.json` contains subscription URLs and tokens; do not log them or surface them beyond the existing read-only config viewer.
- `internal/subscription` parses URI lists, base64 lists, and sing-box JSON only; Clash YAML is intentionally unsupported. Keep `core.canonicalizeNodes` validation: it rejects unknown/unsafe outbound types, Shadowsocks plugins, TLS file paths, and duplicate tags.
- `internal/platform` holds Windows-only elevation and WinINET proxy backup/restore. The `!windows` implementations are no-ops so tests can run elsewhere; TUN and system-proxy behavior still require manual verification on Windows.
- `internal/rulesets` downloads cached `.srs` files from SagerNet repos using ETag.

## Conventions and gotchas

- User-facing text, errors, and runtime log messages are Russian; code comments and docs are English.
- Before UI changes, read `.agents/skills/mygo-maintenance/SKILL.md`. Do not store `ui.Element` or widget parts in app state; store handles and persistent model fields. Rendering can run several times per event, so side effects belong in handled actions or guarded jobs.
- Reuse `baseButton`, `baseInput`, `baseCard`, `switchRow`, `hintText`, and the other primitives in `internal/app/components.go`; layout/theme/type tokens live in `internal/app/tokens.go`. Prefer `.OnClick` over querying `.Clicked()` for navigation, close, and other side effects.
- Use `go tool mygo` (pinned via the `tool` directive in `go.mod`); do not install a separate MyGo CLI.
- Bump `config.Version` and add migration/normalization when changing the on-disk schema incompatibly.

# Development

Technical notes for building, testing and releasing Kimai Tracker. For design
decisions see [architecture.md](architecture.md).

Stack: Go, [Wails v3](https://v3.wails.io), Svelte 5, TypeScript, Tailwind CSS.
whisper.cpp is compiled into the app (cgo); llama.cpp is downloaded at runtime.

## Build from source

### Linux (Ubuntu 24.04 or newer)

Requirements: Go ≥ 1.25, Node.js ≥ 20, GTK 4 / WebKitGTK 6.0 development files,
CMake and Git.

```bash
sudo apt install build-essential pkg-config cmake git libgtk-4-dev libwebkitgtk-6.0-dev

git clone https://github.com/klein-it-solutions/kimai-desktop-app.git
cd kimai-desktop-app
make deps      # wails3 CLI, frontend packages, whisper.cpp (static library)
make build     # → ./bin/kimai-tracker
./bin/kimai-tracker
```

The Makefile finds `wails3` in `$(go env GOPATH)/bin` automatically.

**Ubuntu 22.04** has no GTK 4 / WebKitGTK 6.0. Use the GTK 3 stack:

```bash
sudo apt install build-essential pkg-config cmake git libgtk-3-dev libwebkit2gtk-4.1-dev
make build GTK3=1
```

### Windows 10/11

cgo needs a MinGW-w64 gcc; the easiest way is [MSYS2](https://www.msys2.org).
Install Go and Node.js for Windows, then in the **MSYS2 MINGW64** shell:

```bash
pacman -S --needed make git mingw-w64-x86_64-gcc mingw-w64-x86_64-cmake \
  mingw-w64-x86_64-ninja mingw-w64-x86_64-nsis
export PATH="$PATH:/c/Program Files/Go/bin:/c/Program Files/nodejs:$(cygpath "$(go env GOPATH)")/bin"

make deps
make build     # → bin/kimai-tracker.exe
make package   # → bin/kimai-tracker-amd64-installer.exe (NSIS, per-user install)
```

The binary is linked statically (only Windows system DLLs) and needs the
WebView2 runtime, which is part of Windows 10/11; the installer adds it if missing.

## Make targets

| Command | Description |
| ------- | ----------- |
| `make dev` | Hot-reload development mode (Go + Vite) |
| `make build` | Production binary in `./bin` |
| `make test` | Go unit tests and frontend type check |
| `make lint` | `go vet` and `gofmt` check |
| `make package` | Linux: `.deb`, Windows: installer |
| `make notices` | Regenerate `THIRD_PARTY_NOTICES.md` (after dependency changes) |
| `make appimage` | AppImage for local testing only – not for distribution (see Licensing) |
| `make version` | Version that would be built (from the latest `v*` tag) |
| `make kimai-up` / `make kimai-down` | Local Kimai test instance in Docker |
| `make test-integration` | API tests against a real Kimai |
| `make help` | All targets |

## Testing against a local Kimai

`make kimai-up` starts Kimai 2 + MariaDB via Docker Compose, creates an API token
and seeds customers, projects and activities:

```
Web UI:     http://localhost:8001  (admin / changeme123)
API token:  kt_dev_token_0123456789abcdef0123456789
```

Enter both in the app, or run the integration tests, which start, update and
stop a real timesheet:

```bash
KIMAI_TEST_URL=http://localhost:8001 \
KIMAI_TEST_TOKEN=kt_dev_token_0123456789abcdef0123456789 make test-integration
```

`make kimai-down` removes the instance including its data. The integration tests
also work against your own Kimai – use a test account.

## Releasing

Every release gets its own version and Git tag. The version is compiled into the
app (*Settings → Info*) and used for the package names.

- **From GitHub:** *Actions → Release → Run workflow*, choose `patch`, `minor` or
  `major`. The workflow takes the latest `v*` tag, increases it (`v0.1.0` →
  `v0.1.1`; the first release is `v0.1.0`), creates the tag, runs the tests on
  Linux and Windows, builds the packages and publishes a GitHub release with
  checksums and generated release notes.
  For a specific version, enter it in the *version* field (e.g. `1.0.0`) instead.
- **From the command line:** `git tag v1.2.0 && git push origin v1.2.0` builds
  exactly that version. Tags with a suffix (`v1.2.0-rc.1`) become pre-releases.

Release assets:

| File | Platform |
| ---- | -------- |
| `Kimai-Tracker-<version>-windows-x64-setup.exe` | Windows installer (per user, no admin rights) |
| `Kimai-Tracker-<version>-windows-x64-portable.exe` | Windows, single executable |
| `kimai-tracker_<version>_amd64.deb` | Ubuntu 24.04+ / Debian 13+ |
| `SHA256SUMS` | Checksums |

The Windows executables are not code-signed yet, so SmartScreen warns on first
start. Signing can be added with `wails3 task windows:sign` once a certificate
is available.

Local builds are versioned via `git describe` (e.g. `0.1.0-3-gabc123`); override
with `make package VERSION=1.2.3`. Every push and pull request runs the CI
workflow (lint, tests and build on Linux and Windows).

## Configuration and data

| | Linux | Windows |
| - | ----- | ------- |
| Settings | `~/.config/kimai-tracker/config.json` | `%APPDATA%\kimai-tracker\config.json` |
| Last selection | `~/.config/kimai-tracker/state.json` | `%APPDATA%\kimai-tracker\state.json` |
| Cached lists | `~/.cache/kimai-tracker/` | `%LOCALAPPDATA%\kimai-tracker\` |
| Models, llama.cpp | `~/.local/share/kimai-tracker/` | `%APPDATA%\kimai-tracker\` |
| Token | Secret Service (GNOME Keyring, KWallet …) | Windows Credential Manager |
| Start at login | `~/.config/autostart/kimai-tracker.desktop` | `HKCU\…\CurrentVersion\Run` |

If the keyring cannot be used, the app offers to store the token unencrypted in
`secrets.json` next to the settings (mode 0600). Diagnose keyring problems with:

```bash
kimai-tracker --check-keyring
```

### Environment variables

| Variable | Purpose |
| -------- | ------- |
| `KIMAI_TRACKER_MODEL_URL` | Mirror for whisper models (`ggml-*.bin`) |
| `KIMAI_TRACKER_LLAMA_URL` | Mirror for llama.cpp release archives |
| `KIMAI_TRACKER_LLM_MODEL_URL` | Mirror for GGUF language models |
| `KIMAI_TRACKER_WINDOW_STYLE` | Force window buttons: `mac`, `windows`, `gnome`, `kde` |

### Command-line options

```bash
kimai-tracker --hidden   # start in the tray (used by autostart)
kimai-tracker --toggle   # show/hide the running app's window
kimai-tracker --timer    # start/stop the timer with the last selection
```

## Global shortcuts per platform

| Platform | Implementation |
| -------- | -------------- |
| X11 | Key grab on the root window (all Num/Caps Lock combinations, auto-repeat filtered) |
| Wayland | XDG *GlobalShortcuts* portal (KDE Plasma 6, GNOME 48+); otherwise the `--toggle`/`--timer` fallback |
| Windows | `RegisterHotKey`; key release for push-to-talk is polled with `GetAsyncKeyState` |

## Project structure

```
.
├── .github/workflows/      # CI and release builds
├── main.go                 # Wails application & window setup
├── internal/
│   ├── app/                # Services exposed to the frontend
│   ├── config/             # Configuration, keyring, JSON state files
│   ├── desktop/            # Window controls, transparency, tray check, autostart
│   ├── hotkey/             # Global shortcut (X11, Wayland portal, Windows)
│   ├── kimai/              # Kimai REST client with tests
│   ├── voice/              # Recording (miniaudio), whisper.cpp, OpenAI API, model download
│   ├── llm/                # Local LLM: llama.cpp runtime install, llama-server, bullet points
│   └── fetch/              # Safe downloads (temp file, progress, type check)
├── frontend/               # Svelte + TypeScript + Tailwind UI
│   ├── bindings/           # Generated TypeScript bindings for Go services
│   └── src/
├── build/                  # Wails build assets (icons, .desktop, nfpm, NSIS)
├── dev/kimai/              # Docker Compose for the local Kimai test instance
├── scripts/                # Test-instance provisioning and seeding
├── docs/                   # Architecture and development notes, screenshots
├── Makefile
└── Taskfile.yml            # Used internally by the wails3 CLI
```

## Licensing

Kimai Tracker is source-available under its own license (`LICENSE`): free for a
single person, a commercial license for teams, white-labelling and redistribution.
Keep this in mind when adding dependencies:

- **Only permissive licenses** (MIT, BSD, ISC, Apache-2.0, Zlib, public domain,
  OFL for fonts) for code that is compiled in or shipped. GPL/LGPL/AGPL code must
  not be linked or bundled.
- **Notices:** every bundled component must appear in `THIRD_PARTY_NOTICES.md`.
  Extend `scripts/third-party-notices.sh` and run `make notices`; CI fails if the
  file is outdated. The texts are embedded in the app (*Settings → Über →
  Lizenz*), installed with the `.deb` (`/usr/share/doc/kimai-tracker/`) and the
  Windows installer.
- **No AppImage releases:** an AppImage bundles the system's GTK/WebKitGTK stack,
  including GPL-only libraries (`libjbig0`, `liblzo2-2`). `make appimage` remains
  for local testing.
- **Downloads at runtime** (llama.cpp, Whisper and LLM models) are not shipped;
  users fetch them from the publishers under their licenses, which the app shows
  next to each model.
- **Contributions** are accepted under § 5 of the license (rights granted to
  Klein IT Solutions), see `CONTRIBUTING.md`.

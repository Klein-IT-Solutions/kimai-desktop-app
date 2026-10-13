# Architecture notes

## Wails v3 instead of v2

Kimai Tracker is a tray-first application: it lives in the background, shows the
running timer in the tray menu and is toggled via hotkey. Wails v3 ships a native,
cross-platform system tray (`app.SystemTray`), multi-window support and window
hooks (e.g. cancel *close* → hide). Wails v2 has no tray API; it would need a
third-party library such as `fyne.io/systray` or `getlantern/systray`, which run
their own main loop and clash with GTK's main loop on Linux.

Wails v3 is still tagged *beta* (pinned: `v3.0.0-beta.28`), but its API has been
stable for many releases and is used in production by a number of apps. The version
is pinned in `go.mod`, `frontend/package.json` (`@wailsio/runtime`) and the
`Makefile` (`wails3` CLI) – upgrade all three together.

### Linux GTK stack

Wails v3 uses **GTK 4 + WebKitGTK 6.0** on Linux by default (Ubuntu 24.04+,
Debian 13+, Fedora 39+). The legacy **GTK 3 + WebKit2GTK 4.1** stack is available
via the `gtk3` build tag (`make build GTK3=1`) for Ubuntu 22.04; Wails plans to drop
it in v3.1.

## Window

- Frameless, transparent window of 392 × 512 px. The visible card is 360 × 480; the
  16 px transparent gutter holds the CSS drop shadow (`--shadow-gutter` in
  `frontend/src/app.css`). The shadow must fit inside the gutter, otherwise it is
  clipped at the window edge and shows up as a translucent box. The surface is
  nearly opaque because Linux offers no blur behind windows. Rounded corners are drawn by CSS, so they look identical
  on X11 and Wayland.
- The drag region is the title bar (`--wails-draggable: drag`); interactive children
  opt out with `no-drag`.
- **GTK4 transparency workaround:** Wails v3 beta.28 makes the webview transparent
  but leaves the GTK theme's window background in place (`setTransparent()` is a
  no-op on GTK4), which shows as an opaque rectangle around the rounded card.
  `internal/desktop` installs a small GTK CSS provider that clears the background of
  frameless windows. Remove it once Wails handles this itself.
- Linux has no portable "blur behind window" API (KDE offers one, GNOME doesn't).
  The frosted look is therefore a tinted, semi-transparent surface plus a subtle
  noise texture. On macOS the native translucent backdrop is enabled. On Windows the
  window is transparent (DirectComposition) and draws the same CSS surface; the
  native frameless decorations (Aero shadow, Windows 11 rounded corners) are
  disabled, because they would outline the transparent shadow margin.
- Windows: global shortcuts use `RegisterHotKey` on a locked OS thread with its own
  message loop; push-to-talk polls `GetAsyncKeyState` for the key release.
  Autostart is the per-user `Run` registry key. whisper.cpp and miniaudio are built
  with MinGW-w64 and linked statically (`-static`), so the .exe needs only system
  DLLs. Default shortcuts are Ctrl+Alt+T / Ctrl+Alt+V, because Windows reserves
  most Win+Shift combinations.
- Window buttons follow the platform (`internal/desktop.DetectWindowControls`):
  macOS traffic lights on the left; Windows 11 caption buttons on the right; on Linux
  Adwaita-style buttons (GNOME and others) or Breeze-style (KDE, via
  `XDG_CURRENT_DESKTOP`). The side, order and visible buttons come from the desktop's
  `org.gnome.desktop.wm.preferences button-layout` (XDG portal, fallback `gsettings`),
  so the GNOME default `appmenu:close` shows only a close button. Settings/Done sit on
  the opposite side. `KIMAI_TRACKER_WINDOW_STYLE=mac|windows|gnome|kde` forces a style
  for development. The corner radius follows the platform too.
- Single instance: a second launch focuses the running window. This is also the
  basis for the Wayland hotkey fallback (bind a desktop shortcut to the binary).

## Dark mode

The frontend follows `prefers-color-scheme`. On Linux, WebKitGTK does not always pick
up the desktop preference, so the Go side additionally reads the XDG desktop portal
(`org.freedesktop.appearance color-scheme`) and emits `theme:changed` events.

## Frontend ↔ backend

Go services in `internal/app` are bound via Wails; TypeScript bindings are generated
into `frontend/bindings` by `wails3 generate bindings` (run automatically during
`make build` / `make dev`).

## Configuration and secrets

- `internal/config` stores non-secret settings in `~/.config/kimai-tracker/config.json`
  (honours `XDG_CONFIG_HOME`), written atomically with mode `0600`. A corrupt file is
  moved to `config.json.broken` and the app starts with defaults.
- The API token lives in the system keyring via `zalando/go-keyring`
  (service `kimai-tracker`, key `kimai-api-token`). The token is never sent back to
  the frontend; the UI only learns whether one is stored.
- go-keyring ignores a dismissed Secret Service prompt and reports success without
  storing anything. `config.Keyring.Set` therefore reads the value back and fails
  loudly. On failure the UI offers an explicit opt-in fallback, `config.FileSecrets`
  (`secrets.json`, mode 0600, unencrypted), selected via `tokenStorage: "file"`.
  `--check-keyring` prints the Secret Service provider and does a round trip.

## Kimai client

`internal/kimai` is a small, dependency-free client for the Kimai 2.x REST API
(Bearer token auth). Notes from testing against Kimai 2.69:

- Relations come as plain IDs from collection endpoints but as nested objects from
  `/api/timesheets/active`; `kimai.Ref` accepts both.
- Timestamps are returned as `2026-10-13T09:15:00+0200` and must be sent as
  `2026-10-13T09:15:00` in the **user's Kimai time zone**. The time zone is a top-level
  field of `/api/users/me` (older versions: a `timezone` preference). The client formats
  `begin` in that zone, so desktop and Kimai time zones may differ.
- `/api/activities?project=ID` returns project-specific and global activities.
- Errors are mapped to sentinels (`ErrUnauthorized`, `ErrForbidden`, `ErrNotFound`,
  `ErrOffline`, `ErrCertificate`, `ErrNotHTTPS`, `ErrUnexpectedResponse`); the app layer
  turns them into German UI messages (`app.DescribeError`).

Unit tests use `httptest`; `integration_test.go` (build tag `integration`) runs the
full start → update → stop flow against a real instance (`make kimai-up`).

## Services

- `AppService` – app info, dark mode, opening links.
- `SettingsService` – settings, token, connection test; emits `connection:changed`.
- `TrackerService` – lists, start/stop, description, sync; emits `tracker:changed`.
  A background loop reconciles the running timer with Kimai every 60 s (stopped in
  the web UI, started elsewhere) and re-checks an offline connection. Kimai errors that
  indicate connection problems trigger an immediate connection check.
- `Connection` (not bound) – shared config/secret access and the Kimai client factory.

## Tracking data

- `Catalog` caches customer/project/activity lists in `~/.cache/kimai-tracker/cache.json`
  (fresh for 10 minutes, used as stale fallback when offline) and keeps recently used
  entries plus the last selection and draft description in
  `~/.config/kimai-tracker/state.json`. Both are bound to the Kimai URL; switching
  instances starts clean.
- Lists show recently used entries first (most recent first, max. 5), the rest
  alphabetically.
- The description of a running timer is PATCHed ~0.7 s after typing stops; without a
  running timer it is kept as draft for the next start.

## Background operation

- `Shell` (not bound) owns the tray icon, close-to-tray, the global hotkey,
  notifications and command-line actions; `ShellService` exposes the settings to the UI.
- Tray: Wails' StatusNotifierItem implementation. `desktop.TrayAvailable()` checks for
  `org.kde.StatusNotifierWatcher`; without it, closing minimises instead of hiding, and
  `--hidden` is ignored so the app never becomes unreachable. The menu shows the
  running time at minute resolution (refreshed every 5 s while running).
- Hotkey (`internal/hotkey`): X11 key grab via `jezek/xgb` (pure Go, also grabbed with
  Caps/Num Lock variants); on Wayland the XDG GlobalShortcuts portal. Fallback for all
  desktops: `kimai-tracker --toggle` / `--timer`, forwarded to the running instance via
  Wails single-instance.
- Notifications: Wails notifications service (freedesktop on Linux), sent on every
  running/stopped transition, including timers stopped in the Kimai web UI.
- Autostart: XDG autostart entry with `--hidden`; the AppImage path is used when running
  from an AppImage (local test builds only, AppImages are not released).
- A window created hidden is centred after it is first shown (GTK ignores positioning
  before mapping; on Wayland the compositor places windows).

Untested so far: the GlobalShortcuts portal flow on a real KDE/GNOME Wayland session
(the container has no portal backend for it).

## Branding

Publisher details live in `frontend/src/lib/brand/brand.ts`. Put the official logo
next to it as `logo.svg`, `logo.webp` or `logo.png` (optionally `logo-dark.*` for dark
mode; without it the logo sits on a light badge in dark mode). Until then a text
wordmark is shown.

## Voice input

- Recording in Go with `gen2brain/malgo` (miniaudio, vendored C, no system libraries;
  PipeWire/PulseAudio/ALSA on Linux, CoreAudio, WASAPI) at 16 kHz mono – the format
  Whisper expects. Max. 2 minutes per recording. `Recorder.Stop` waits until the samples
  cover the recorded time (≤ 3 s), because some sound servers deliver capture data late;
  otherwise the end of a sentence would be cut off.
- `voice.Local` (optional external program) runs `whisper-cli -m MODEL -f FILE -l LANG -t N -nt -np` and strips
  markers such as `[BLANK_AUDIO]`. `voice.API` posts multipart form data to
  `{url}/audio/transcriptions` (`model`, `language`, `response_format=json`).
- Models (quantised, multilingual) are downloaded from the whisper.cpp Hugging Face
  repository to `~/.local/share/kimai-tracker/models`, written to a temp file and only
  accepted if complete and starting with the ggml magic bytes.
- Push-to-talk uses the hotkey manager with press/release callbacks: X11 key release with
  auto-repeat filtering; on Wayland the portal's `Activated`/`Deactivated` signals.
- The frontend appends `voice:result` text to the description, which then goes through the
  normal debounced description save.

Tested in the container with a PulseAudio null sink (speech synthesised with espeak-ng),
a fake OpenAI-compatible server and whisper.cpp built from source with its test model.
Real transcription quality could not be tested there (model downloads are blocked).

## Local AI (bullet points)

- `internal/llm` downloads llama.cpp's official CPU release build for the platform
  (`llama-<tag>-bin-ubuntu-x64.tar.gz` etc., names from llama.cpp's `release-publish.yml`;
  built with `RPATH=$ORIGIN`, so it runs unpacked). llama.cpp publishes **pre-releases**,
  so `/releases/latest` must not be used (it points to an ancient release with other file
  names). The app asks the GitHub releases API for the newest release that contains the
  platform's archive; if the API is unavailable or rate-limited it reads the Atom feed and
  checks with HEAD requests which archive exists. Archives are extracted with path
  clamping and only relative symlinks.
- whisper.cpp is **compiled into the app** (`voice.Embedded`, cgo against the static
  libraries). `make whisper` clones the pinned release (`WHISPER_VERSION` in the Makefile)
  into `third_party/` and builds it for the host OS only (`GGML_NATIVE=OFF`: AVX2/FMA
  baseline on x64, no OpenMP), so builds are portable and no runtime download of a program
  is needed. The loaded model stays in memory between recordings and is freed after
  5 minutes. An external `whisper-cli` (`voice.Local`) is used only when configured.
- `llm.Server` starts `llama-server -m MODEL --host 127.0.0.1 --port FREE -c 4096 -np 1`
  on demand, waits for `/health`, uses the OpenAI-compatible `/v1/chat/completions` (the
  server applies the model's chat template) and stops after 5 minutes idle. On Linux the
  child gets `Pdeathsig`, so it never outlives the app.
- A strict German system prompt asks for 1–5 bullets without invention;
  `ParseBullets` normalises answers (drops introductions, unifies markers, max. 6).
- Models are GGUF files (Qwen2.5 0.5B/1.5B Instruct, Apache-2.0; Llama 3.2 3B Instruct,
  Llama 3.2 Community License), validated by the `GGUF` magic.

Tested in the container with llama.cpp built from source and packed like a release, served
from a local mirror, and a tiny random-weight qwen2 GGUF (real Qwen tokenizer) – this
exercises download, unpacking, server start, chat template and parsing, but not the quality
of real models (Hugging Face is blocked there). The Hugging Face file names of the models
could not be verified from the container either.

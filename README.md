# zer0-waymarks

[![CI](https://github.com/andrey-losikhin/zer0-waymarks/actions/workflows/ci.yml/badge.svg)](https://github.com/andrey-losikhin/zer0-waymarks/actions/workflows/ci.yml)
[![Go 1.24](https://img.shields.io/badge/Go-1.24-00ADD8?logo=go)](https://go.dev/)
[![License: Apache-2.0](https://img.shields.io/badge/License-Apache--2.0-blue.svg)](LICENSE)
[![Platform: Linux](https://img.shields.io/badge/platform-Linux-lightgrey?logo=linux)](docs/SUPPORT.md)

A local-first, browser-independent bookmark launcher for Wayland and
[Noctalia](https://noctalia.dev/). Waymarks keeps one user-owned bookmark
collection, provides instant keyboard search, and opens every bookmark in the
browser and profile you choose.

> [!IMPORTANT]
> The Noctalia plugin and local helper are usable today. Firefox synchronization
> is experimental, and Chromium synchronization is planned. Waymarks never
> edits browser profile databases directly.

## Features

- Search titles, URLs, notes, and tags from the Noctalia launcher or panel.
- Add, edit, organize, and delete bookmarks without opening a browser.
- Use tags as sections such as `personal`, `work`, or `project`.
- Search and reuse existing tags while adding or editing a bookmark.
- Detect common Linux browsers, including Firefox, Chromium, Brave, Zen, and
  Helium, then persist the default browser.
- Open an individual bookmark with a different browser/profile without changing
  the default.
- Navigate the complete panel by keyboard.
- Store data locally as versioned append-only JSON events plus an atomic
  snapshot—no account, cloud service, database, or telemetry.
- Open URLs with fixed argument arrays; no shell command interpolation.

## Status

| Component | Status |
| --- | --- |
| Go helper and local store | Ready for local use |
| Noctalia launcher and panel | Ready for local use |
| Firefox Native Messaging bridge | Experimental; temporary extension workflow |
| Chromium bridge | Planned |
| Cross-device synchronization | Out of scope for the first release |

## Requirements

- Linux with a Wayland session
- [Noctalia](https://noctalia.dev/) with plugin API 24 or newer
- Go 1.24 or newer
- Node.js for JavaScript tests only
- Firefox only when testing the experimental browser bridge

## Quick start

Clone the repository and install the helper into a directory on `PATH`:

```sh
git clone https://github.com/andrey-losikhin/zer0-waymarks.git
cd zer0-waymarks
go install ./cmd/zer0-waymarks-helper
```

Add this checkout as a local Noctalia plugin source and enable Waymarks:

```sh
noctalia msg plugins source add waymarks path "$PWD/noctalia-plugin"
noctalia msg plugins enable zer0/waymarks
```

Open the panel:

```sh
noctalia msg panel-toggle zer0/waymarks:panel
```

The launcher provider is available under `/wm`. If `go install` placed the
helper outside your `PATH`, add `$(go env GOPATH)/bin` to the environment used
to start Noctalia.

## Keyboard workflow

| Shortcut | Action |
| --- | --- |
| Type | Filter bookmarks immediately |
| `Up` / `Down` | Select a bookmark |
| `Enter` | Open the selected bookmark |
| `Ctrl+1` | Bookmarks |
| `Ctrl+2` or `Ctrl+N` | Add bookmark |
| `Ctrl+3` | Browser settings |
| `Ctrl+T` | Open searchable tag picker |
| `Ctrl+Shift+T` | Clear the active tag filter |
| `Ctrl+Up` / `Ctrl+Down` | Change section |
| `Ctrl+Enter` | Save the add/edit form |
| `Escape` | Return to bookmarks or close the panel |

## Local data and configuration

Waymarks follows the XDG Base Directory specification:

- data: `$XDG_DATA_HOME/zer0-waymarks` (normally `~/.local/share/zer0-waymarks`)
- configuration: `$XDG_CONFIG_HOME/zer0-waymarks/config.json` (normally
  `~/.config/zer0-waymarks/config.json`)

Browser executables and profile arguments are configured outside the UI and are
never accepted as free-form commands. See [configuration](docs/CONFIGURATION.md)
and [import/export](docs/IMPORT.md).

## Development

Run the complete local verification suite:

```sh
scripts/test-local.sh
```

Build the helper, Native Messaging host, and temporary Firefox extension:

```sh
scripts/build-local.sh
```

Outputs are written to `dist/local/`. The test suite does not access a real
browser profile. See [local development](docs/LOCAL_DEVELOPMENT.md) for the
disposable Firefox workflow.

## Architecture and security

The Go helper owns the canonical store. The Noctalia plugin and optional browser
extensions communicate through typed commands; URLs, browser IDs, and profile
IDs are passed as separate arguments. Only `http` and `https` URLs are allowed
by default.

- [Architecture](docs/ARCHITECTURE.md)
- [Protocol](docs/PROTOCOL.md)
- [Security model](docs/SECURITY.md)
- [Support matrix](docs/SUPPORT.md)
- [Architecture decisions](docs/decisions/)

## Contributing

Bug reports, focused feature proposals, documentation fixes, and tested pull
requests are welcome. Read [CONTRIBUTING.md](CONTRIBUTING.md) and the
[Code of Conduct](CODE_OF_CONDUCT.md) before contributing. Please report
security issues using [SECURITY.md](SECURITY.md), not a public issue.

## License

Licensed under the [Apache License 2.0](LICENSE).

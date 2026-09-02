# Contributing to zer0-waymarks

Thank you for helping improve Waymarks. Small, focused changes with a clear user
benefit are easiest to review and maintain.

## Before opening an issue

- Search existing issues first.
- Use the bug report or feature request template.
- Do not include bookmark data, browser history, profile paths, tokens, or other
  private information in logs and screenshots.
- Report security vulnerabilities privately as described in [SECURITY.md](SECURITY.md).

## Development setup

Requirements are Go 1.24+, Node.js, and a POSIX shell. Clone your fork, create a
topic branch, and run the full verification suite before submitting a change:

```sh
git clone https://github.com/YOUR-USER/zer0-waymarks.git
cd zer0-waymarks
git switch -c fix/short-description
scripts/test-local.sh
```

No network access or real browser profile is required by the tests.

## Project layout

- `cmd/zer0-waymarks-helper/` — CLI used by Noctalia
- `cmd/zer0-waymarks-native-host/` — Firefox Native Messaging entry point
- `internal/` — store, protocol, browser discovery, and bridge logic
- `noctalia-plugin/waymarks/` — Noctalia plugin package
- `browser-extension/` — shared extension code and browser-specific manifests
- `docs/` — architecture, security, configuration, and protocol documentation
- `scripts/` — local build, installation, and verification helpers

## Change guidelines

- Keep the helper Linux-only, standard-library-first, and free of cgo.
- Do not add a dependency without documenting the architectural reason.
- Never execute URLs, titles, browser names, or profile values through a shell.
- Do not read or modify browser profile databases directly.
- Test browser behavior only with disposable profiles or fixtures.
- Keep changes scoped; avoid unrelated formatting or refactoring.
- Add or update tests for behavior changes and update user-facing documentation.
- Use clear commit subjects in the imperative mood, for example
  `Add tag filtering to the panel`.

## Pull requests

Before opening a pull request:

1. Run `scripts/test-local.sh`.
2. Run `git diff --check`.
3. Explain the user-visible change and its security impact.
4. Include screenshots for UI changes when practical.
5. Call out anything that was not tested.

By contributing, you agree that your contribution is licensed under the
Apache License 2.0 used by this repository.

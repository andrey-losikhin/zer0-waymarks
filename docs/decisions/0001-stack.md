# ADR 0001: локальный Go helper и WebExtension bridge

## Статус

Принято для MVP 2026-08-24; storage и browser semantics требуют spike.

## Решение

- Go stdlib-first helper без cgo.
- Append-only JSON events и атомарный JSON snapshot вместо SQLite.
- Plain JavaScript WebExtension core без npm runtime dependencies.
- Отдельные Firefox и Chromium manifests.
- Native Messaging вместо localhost HTTP/daemon.
- Luau для Noctalia Plugin API 24+.

## Почему

Это минимизирует runtime dependencies и сохраняет данные в переносимом,
проверяемом пользователем формате. WebExtension необходим, потому что только
browser bookmarks API предоставляет поддерживаемые CRUD-события. Прямое
изменение browser profile DB запрещено как хрупкое и опасное.

## Не добавлять без ADR

SQLite, Electron/Tauri, Node runtime, npm framework, localhost server, systemd
daemon, прямой browser-profile parser или облачный backend.

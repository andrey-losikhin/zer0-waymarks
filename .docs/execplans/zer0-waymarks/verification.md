# Verification Plan: zer0-waymarks

## Mandatory gates

- Go: `gofmt`, `go test ./...`, `go vet ./...`, build с `CGO_ENABLED=0`.
- Store: crash recovery, partial-write, lock contention, deterministic replay.
- Security: no shell, URL scheme allowlist, native origin allowlist, input limits.
- Browser E2E: только disposable profiles; create/update/move/remove/offline/restart.
- Sync: echo-loop, duplicate, concurrent edit, tombstone, managed-folder deletion.
- Noctalia: API 24 argv-table, manifest/community validation.
- Packaging: clean Arch build, install/uninstall, data preservation.

## Последняя проверка Milestone 4 — 2026-08-28

- `go test -buildvcs=false ./...` — пройдено.
- `go test -buildvcs=false -race ./internal/store ./internal/nativehost` — пройдено.
- `go vet -buildvcs=false ./...` — пройдено.
- `CGO_ENABLED=0 go build -buildvcs=false` для helper и native host — пройдено.
- Node test/check, Firefox assembly и JSON parse обоих manifests — пройдено.
- Synthetic framed request к отдельному native-host binary во временном XDG data
  dir — пройдено (`changes.pull`, один `root.created`, cursor 1).
- Firefox disposable-profile E2E и применение bookmarks ещё не выполнялись.
- Local bundle/install проверен во временных `HOME`/`XDG_DATA_HOME`: permissions,
  абсолютный manifest path и framed request к установленному host прошли.
- Bookmark-tree tests покрывают точечный candidate filtering, nested mapping,
  unsafe URL/title и limits 5000 nodes/32 depth; Firefox options bundle собран.
- `reconcile.preview` проверен unit/race и synthetic Native Messaging smoke;
  JS↔Go fingerprint fixtures включают `&<>` и U+2028, expired tokens очищаются.
- Двухфазный reconcile, mapping CAS, crash recovery initial push и mapped
  create/update/remove проверены Go tests; malformed mapping отклоняется без panic.
- Firefox background mock-тесты проверяют local create, remote create без echo и
  restart после helper commit до сохранения browser mapping.
- Firefox VM-тесты дополнительно проверяют options resume→prepare→commit,
  mapped update, move-out→remove и сохранение cursor/mapping; Native handler tests
  проверяют полное декодирование push/apply, bridge tests — prepare pagination,
  mapped retry и stale-revision conflict.
- `scripts/test-local.sh` объединяет полный локальный Go/JS/race/vet gate; запуск
  после добавления parent/child page-boundary, preview decode и marker assertions
  пройден полностью.
- Firefox 153.0.4 запущен headless с disposable профилем в `/tmp`; Marionette
  подтвердил localhost listener. Загрузка временного unsigned extension и UI E2E
  не автоматизированы: локально отсутствуют `web-ext` и `geckodriver`.
- Независимое correctness/skeptic review выполнено; подтверждённые findings по
  durable intents, marker recovery, conflicts, reset/root/move boundaries,
  operation reuse и stale preview исправлены, после чего gates повторены.
- Локальный bundle и user installer повторно проверены в полностью временных
  `HOME`/`XDG_DATA_HOME`; manifest mode `0600`, binaries `0755`, extension `0644`.

## Synthetic data

Использовать только `example.test`, фиктивные titles и временные XDG/profile dirs.
Не копировать реальные browser profiles или личные URL.

## Критическая матрица

| Сценарий | Ожидание |
| --- | --- |
| Добавление в Firefox `Waymarks` | Появляется в store и Chromium после pull |
| Изменение вне `Waymarks` | Игнорируется |
| Browser закрыт | Догоняет изменения после запуска |
| Remote event применён в browser | Не возвращается бесконечным echo |
| Одновременное изменение | Conflict report, без silent loss |
| Удаление | Tombstone распространяется идемпотентно |
| Helper crash при записи | Предыдущие подтверждённые events читаются |
| URL `javascript:`/leading option | Отклоняется |
| Поддельный extension origin | Native host отказывает |
| Uninstall package | Пользовательская база сохраняется |

## Definition of Done MVP

- Milestones 1–7 закрыты обязательными gate-проверками.
- Нет high/critical findings.
- Cross-browser E2E не затрагивает закладки вне managed folder.
- Документация честно описывает eventual sync и отсутствие mobile/cloud.
- Backup, dry-run reconcile и uninstall behavior проверены.

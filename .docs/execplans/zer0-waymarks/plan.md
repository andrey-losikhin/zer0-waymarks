# План реализации: zer0-waymarks

## Milestone 1: compatibility и protocol spike

### Цель
Подтвердить browser APIs, Native Messaging, Noctalia API и event model.

### Области
`docs/PROTOCOL.md`, `docs/SECURITY.md`, `docs/decisions/`.

### Шаги
1. Создать disposable Firefox/Chromium profiles.
2. Проверить CRUD/events выделенной папки и различия manifests.
3. Проверить Native Messaging framing, limits, paths и extension allowlists.
4. Спроектировать versioned event envelope, UUID, revision, tombstone и conflict.
5. Проверить Noctalia API 24 argv-table и community packaging.
6. Зафиксировать supported browsers/versions и минимальную Go version.

### Acceptance criteria
Есть подтверждённые protocol, event ordering, echo suppression и support matrix.

### Validation
Synthetic extension prototypes, captured JSON, restart/offline/event-order matrix.

### Риски
Различия MV2/MV3, service-worker suspension и fork-specific manifest paths.

### Stop rule
Не писать store до утверждения event/identity/conflict contracts.

## Milestone 2: helper store и CRUD

### Цель
Реализовать локальный event store, snapshot и безопасный CLI.

### Области
`cmd/zer0-waymarks-helper/`, `internal/store/`, `internal/protocol/`.

### Шаги
1. Реализовать schema validation и URL policy.
2. Добавить append-only events, lock, fsync/atomic snapshot и recovery.
3. Реализовать list/get/add/update/remove/export.
4. Добавить tombstones, conflict records и deterministic projection.
5. Ограничить размеры и redacted logging.

### Acceptance criteria
Crash/restart не повреждает подтверждённые events; CRUD идемпотентен; unsafe schemes отклоняются.

### Validation
`gofmt`, `go test ./...`, `go vet ./...`, crash/fuzz/property tests, `CGO_ENABLED=0 go build`.

### Риски
Partial writes, lock races, unbounded log и metadata leakage.

### Stop rule
Не подключать браузеры при потере событий или недетерминированной projection.

## Milestone 3: browser opener и import/export

### Цель
Открывать URL выбранным браузером и безопасно переносить существующие закладки.

### Области
`internal/browser/`, `docs/CONFIGURATION.md`, `docs/IMPORT.md`.

### Шаги
1. Добавить allowlisted browser/profile config с argv arrays.
2. Реализовать open без shell.
3. Реализовать Netscape HTML dry-run/import/export.
4. Добавить duplicate report и rollback snapshot.

### Acceptance criteria
Нет option/shell injection; import никогда не удаляет данные; dry-run совпадает с apply.

### Validation
Fake browsers, malicious URL/title fixtures, round-trip import/export.

### Риски
Profile CLI различается между forks; HTML dialects неодинаковы.

### Stop rule
Не подключать UI при injection или необратимом import.

## Milestone 4: Firefox bridge

### Цель
Синхронизировать папку `Waymarks` Firefox ↔ canonical store.

### Области
`browser-extension/shared/`, `browser-extension/firefox/`, native manifests.

### Шаги
1. Реализовать native protocol client и explicit extension ID.
2. Создать/найти managed folder и browser mapping.
3. Обработать create/update/move/remove и suppress echo.
4. Добавить initial dry-run reconcile, pull-on-start и periodic pull.
5. Покрыть offline/restart/conflict/remove races.

### Acceptance criteria
Двусторонние изменения сходятся без циклов и не затрагивают закладки вне managed folder.

### Validation
Disposable Firefox profile E2E и permission/native-host negative tests.

### Риски
Import bursts, folder deletion и смена extension ID.

### Stop rule
Не добавлять Chromium до идемпотентного Firefox E2E.

## Milestone 5: Chromium bridge

### Цель
Переиспользовать core с отдельным MV3 manifest и Chromium semantics.

### Области
`browser-extension/chromium/`, shared compatibility adapter, native manifests.

### Шаги
1. Добавить MV3 service worker и `chrome.*` adapter.
2. Настроить `allowed_origins` и fork-aware install docs.
3. Проверить alarms/startup catch-up и worker suspension.
4. Выполнить Firefox ↔ store ↔ Chromium E2E.

### Acceptance criteria
Закладка из одного браузера появляется в другом при следующем pull без duplicate loop.

### Validation
Disposable Chromium profile, restart/suspend/offline/cross-browser matrix.

### Риски
MV3 lifecycle и отличия Chromium forks.

### Stop rule
Не подключать Noctalia при cross-browser data loss/high findings.

## Milestone 6: Noctalia plugin

### Цель
Добавить `/wm`, panel, CRUD и выбор browser/profile.

### Области
`noctalia-plugin/waymarks/`.

### Шаги
1. Создать `plugin.toml` с API 24 и helper dependency.
2. Реализовать metadata search и argv-table actions.
3. Добавить panel, errors, empty/missing-helper states.
4. Добавить ручное добавление URL без чтения active tab.

### Acceptance criteria
Noctalia ищет общую базу и открывает выбранный browser без shell command strings.

### Validation
Community lint, disable/reload, malicious metadata and missing dependency tests.

### Риски
API beta и UI state divergence.

### Stop rule
Не пакетировать при shell invocation или невалидном manifest.

## Milestone 7: Arch packaging и release candidate

### Цель
Поставлять helper, native manifests, extensions и Noctalia plugin воспроизводимо.

### Области
`packaging/arch/`, release docs, licenses.

### Шаги
1. Создать PKGBUILD: Go только makedepends.
2. Устанавливать manifests без изменения browser profiles.
3. Документировать extension installation/uninstall и data backup.
4. Пройти clean build, fresh-user E2E и security/skeptic review.

### Acceptance criteria
Install/uninstall не удаляет пользовательскую базу; Firefox/Chromium/Noctalia E2E зелёный.

### Validation
Clean chroot build, package inspection, disposable-user smoke, checksums.

### Риски
Extension distribution/signing и пути manifests у forks.

### Stop rule
Не публиковать с high/critical findings или неподтверждённым rollback.

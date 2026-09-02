# Архитектура zer0-waymarks

## Компоненты

### `zer0-waymarks-helper`

Go CLI и Native Messaging host:

```text
list [--query <query>] --json
get <id> --json
add --url <url> ... --operation-id <id> --json
add-folder --title <title> ... --operation-id <id> --json
update <id> ... --base-revision <n> --operation-id <id> --json
remove <id> --base-revision <n> --operation-id <id> --json
open <id> --browser <id> [--profile <id>] --json
conflicts list|get|resolve ... --json
export-snapshot --json
import-html <path> --dry-run|--apply --json
export-html <path> --json
native-host (режим CLI helper для диагностики)
zer0-waymarks-native-host (отдельный Firefox entrypoint; принимает только manifest path)
```

Точный контракт задан в [`PROTOCOL.md`](PROTOCOL.md). CLI и native protocol
используют fixed argv/JSON, лимиты входа и versioned envelopes.

### Каноническое хранилище

Расположение по XDG:

```text
$XDG_DATA_HOME/zer0-waymarks/events/<device-id>/<sequence>.json
$XDG_DATA_HOME/zer0-waymarks/snapshot.json
$XDG_STATE_HOME/zer0-waymarks/browser-maps/<instance-id>.json
```

Events неизменяемые; snapshot строится атомарно через temporary file + fsync +
rename с последующим fsync каталога. Запись сериализуется advisory lock. Browser ID
не является каноническим ID и хранится только в instance mapping. Удаление — tombstone.

Основная модель node: `id`, `type`, `parent_id`, `title`, `url`, `note`, `position`,
`revision`, `updated_at`, `deleted_at`. Merge использует optimistic base revision;
несовпадение сохраняется как explicit conflict record.

### Browser bridges

Общий JavaScript core слушает bookmark events только внутри папки `Waymarks`.
Firefox и Chromium имеют отдельные manifests и native-host allowlists. Расширение:

1. отправляет локальные события helper-у;
2. периодически запрашивает события после своей подтверждённой store sequence;
3. применяет изменения и подавляет echo по operation ID;
4. хранит browser bookmark ID ↔ canonical UUID mapping.

Native Messaging выбран вместо localhost HTTP: нет открытого порта, токена или
постоянного daemon. Host запускается браузером по запросу.

### Noctalia plugin

`zer0/waymarks`, Plugin API 24+, launcher `/wm` и panel. Плагин получает только
bookmark metadata и вызывает helper через argv-table. Строковый shell API запрещён.

### Browser opener

Конфигурация содержит allowlisted executable и массив фиксированных profile args.
URL передаётся отдельным argv. Произвольная command string не поддерживается.

## Sync semantics MVP

- область: только корневая папка `Waymarks`;
- локальная база — источник истины;
- первичная привязка требует dry-run и явного подтверждения;
- одинаковые URL не объединяются автоматически;
- concurrent edits создают conflict record, а не silent last-write-wins;
- offline browser догоняет изменения при следующем запуске;
- full reconcile идемпотентен и имеет backup/rollback instructions.

## Зависимости

Runtime helper-а не требует БД или daemon. Go — build dependency. Browser
extensions и Noctalia устанавливаются отдельно. Для открытия используется сам
выбранный браузер. Git не является dependency MVP.

## Риски

- различия Firefox/Chromium manifests и bookmark tree semantics;
- MV3 service worker может приостанавливаться;
- циклы событий при применении remote change;
- rename/move/delete races;
- утечка URL metadata в backups или будущий Git remote;
- браузерные форки могут искать Native Messaging manifest в других путях.

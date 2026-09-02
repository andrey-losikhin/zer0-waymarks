# Protocol v1

## Назначение

Один контракт используется CLI, Noctalia-плагином и Native Messaging host.
Версия `1` покрывает локальный MVP: поиск, CRUD, открытие закладки и синхронизацию
управляемой папки `Waymarks`. Межустройственная синхронизация не входит в v1.

## CLI

Плагин запускает helper только массивом аргументов:

```text
zer0-waymarks-helper list [--query TEXT] [--tag TAG] [--limit N] --json
zer0-waymarks-helper tags --json
zer0-waymarks-helper get ID --json
zer0-waymarks-helper add --url URL [--title TITLE] [--note NOTE] [--tags CSV] [--parent ID] --operation-id ID --json
zer0-waymarks-helper add-folder --title TITLE [--parent ID] --operation-id ID --json
zer0-waymarks-helper update ID [--url URL] [--title TITLE] [--note NOTE] [--tags CSV] [--parent ID] --base-revision N --operation-id ID --json
zer0-waymarks-helper remove ID --base-revision N --operation-id ID --json
zer0-waymarks-helper open ID --browser BROWSER [--profile PROFILE] --json
zer0-waymarks-helper browsers --json
zer0-waymarks-helper set-default-browser BROWSER --json
zer0-waymarks-helper export-snapshot --json
zer0-waymarks-helper import-html FILE --dry-run|--apply --json
zer0-waymarks-helper export-html FILE --json
zer0-waymarks-helper conflicts list [--limit N] --json
zer0-waymarks-helper conflicts get ID --json
zer0-waymarks-helper conflicts resolve ID --choose current|proposed --operation-id ID --json
```

`list` выполняет case-insensitive поиск по `title`, `url`, `note` и тегам. `--tag`
добавляет точный case-insensitive фильтр раздела. Пустой запрос возвращает
недавно изменённые активные закладки. Результаты сортируются по fuzzy score,
затем `updated_at` descending и `id` ascending. `limit` по умолчанию равен 50,
максимум — 100; общий stdout ограничен 512 KiB. Если подходящих записей больше,
`data.truncated` равен `true`; launcher уточняет query вместо загрузки всех записей.
Tombstones, folders и conflict records не попадают в обычную выдачу.

Каждая мутация требует созданный клиентом случайный `operation_id`. Helper хранит
нормализованный request и результат. Повтор того же ID с тем же request возвращает
первоначальный результат без нового event; reuse ID с другим request возвращает
`invalid_request`.

Каждый вызов пишет в stdout ровно один UTF-8 JSON document и ничего больше.
Диагностика идёт в stderr. Успех:

```json
{"protocol_version":1,"ok":true,"data":{"bookmarks":[],"truncated":false}}
```

Ошибка:

```json
{"protocol_version":1,"ok":false,"error":{"code":"invalid_url","message":"URL scheme is not allowed"}}
```

Стабильные коды v1: `invalid_request`, `invalid_url`, `not_found`, `conflict`,
`limit_exceeded`, `browser_not_allowed`, `profile_not_allowed`, `busy`,
`stale_preview`, `storage_error`, `unsupported_version`. Сообщение предназначено
человеку и не используется для ветвления. Ошибка `conflict` дополнительно содержит
`details.conflict_id`.

HTML import требует ровно один режим: `--dry-run` ничего не импортирует, `--apply`
создаёт backup перед первым event. Точные URL-дубликаты пропускаются. Export HTML
атомарно записывает только активные nodes; подробности — в [`IMPORT.md`](IMPORT.md).

## Модель данных

```json
{
  "id": "018f0f4d-4f36-7b95-8c07-f59cf2f77c2b",
  "type": "bookmark",
  "parent_id": null,
  "title": "Example",
  "url": "https://example.test/docs",
  "note": "Internal reference",
  "tags": ["Работа", "Проект"],
  "position": 0,
  "revision": 3,
  "updated_at": "2026-08-27T10:00:00Z",
  "deleted_at": null
}
```

Node имеет `type: bookmark|folder`. Bookmark требует URL и может содержать локальную
заметку `note` до 4096 байт; folder имеет `url: null` и не содержит заметку.
Bookmark может иметь до 16 уникальных case-insensitive тегов длиной до 64 байт;
folder тегов не имеет. Теги служат пользовательскими разделами и остаются локальными.
Браузерные API не имеют общего поля заметки, поэтому bridge сохраняет canonical
`note`, но не отправляет её в браузер и не удаляет при browser-originated update.
Корневой managed folder создаёт helper, его ID стабилен в store. Parent обязан быть
активным folder; node нельзя сделать своим потомком. Удаление непустого folder
возвращает `invalid_request`; recursive delete в v1 отсутствует.

ID — непрозрачная UUID-строка, создаваемая helper-ом. `revision` начинается с 1 и
увеличивается при каждом подтверждённом изменении записи. Update/remove требуют
`base_revision`; несовпадение создаёт conflict record и возвращает `conflict`, не
перезаписывая текущее значение.

Conflict содержит `id`, `node_id`, `current`, `proposed`, `created_at`, `resolved_at`
и `resolution`. `current` и `proposed` — полные node snapshots. Resolve записывает
новый event от выбранной версии, но только если current revision не изменился;
иначе создаётся новый conflict.

Browser/profile выдаются командой `browsers` и передаются обратно только по ID:

```json
{"browsers":[{"id":"firefox","name":"Firefox","profiles":[{"id":"work","name":"Work"}]}]}
```

Executable и аргументы профиля находятся только в конфигурации helper-а и никогда
не принимаются из UI.

## Event envelope

Append-only log содержит один JSON event на файл:

```json
{
  "protocol_version": 1,
  "event_id": "device-id:42",
  "device_id": "device-id",
  "sequence": 42,
  "operation_id": "opaque-operation-id",
  "kind": "node.updated",
  "node_id": "018f0f4d-4f36-7b95-8c07-f59cf2f77c2b",
  "base_revision": 2,
  "revision": 3,
  "occurred_at": "2026-08-27T10:00:00Z",
  "payload": {
    "node": {
      "id": "018f0f4d-4f36-7b95-8c07-f59cf2f77c2b",
      "type": "bookmark",
      "parent_id": null,
      "title": "Example",
      "url": "https://example.test/docs",
      "note": "Internal reference",
      "position": 0,
      "revision": 3,
      "updated_at": "2026-08-27T10:00:00Z",
      "deleted_at": null
    }
  }
}
```

Kinds v1: `root.created`, `node.created`, `node.updated`, `node.removed`, `conflict.recorded`,
`conflict.resolved`. Каждый node event несёт полный post-operation node snapshot;
remove snapshot содержит `deleted_at`, а conflict events — полный conflict record.
Под одной блокировкой helper проверяет base revision,
назначает монотонный `sequence`, сохраняет event и обновляет projection.
Повторный `event_id` или `operation_id` идемпотентен.

Удаление записывает tombstone. Оно не удаляет историю и не переиспользует ID.
При несовпадающей base revision helper сохраняет обе версии в conflict record;
автоматического last-write-wins нет.

## Native Messaging

Browser extension передаёт JSON envelope с `protocol_version`, `request_id`,
`method` и `params`. Ответ повторяет `request_id` и использует тот же `ok/data/error`
contract. Методы v1: `changes.pull`, `changes.push`, `reconcile.preview` и
`reconcile.apply`.

`changes.pull` request:

```json
{"protocol_version":1,"request_id":"r1","method":"changes.pull","params":{"browser_instance_id":"firefox-work","after_sequence":41,"limit":100}}
```

Response data содержит `events` в порядке sequence ascending, `through_sequence`
(sequence последнего event, либо входной cursor для пустой страницы) и `has_more`.
Helper фиксирует верхнюю границу страницы в начале запроса, поэтому события,
добавленные параллельно, попадут в следующий pull. Extension атомарно сохраняет
browser mappings и acknowledged `through_sequence` только после применения всей
страницы; при сбое повторяет прежний `after_sequence`.

`changes.push` принимает `browser_instance_id` и batch полных node operations. Во
время initial reconcile запрос содержит fingerprint и действующий preview token;
после commit token/fingerprint пусты, а helper авторизует IDs и parents по
постоянному instance mapping. Response также возвращает текущую mapping revision.
Batch содержит
уникальными operation IDs и base revisions; ответ содержит результат каждого
элемента в исходном порядке. Весь batch валидируется до записи, но элементы
применяются независимо: retry безопасен по operation ID. Operation ID сохраняется
в browser mapping и подавляет echo применённого remote change.

Operation имеет `kind: create|update|remove`. Create передаёт `type`, `title`,
`url`, canonical `parent_id` и возвращает созданный canonical node. Update передаёт
полный node state, `node_id` и `base_revision`; remove — `node_id` и
`base_revision`. Batch содержит 1..200 элементов и полностью валидируется до
первой записи; результаты применяются независимо и возвращаются в исходном порядке.
Одинаковые operation IDs внутри batch запрещены. Payload push дополнительно
ограничен 512 KiB, чтобы полный response гарантированно помещался в Native frame.
Session token можно повторять для безопасного retry; он привязан к fingerprint и
ожидаемой store sequence, продвигается после batch и будет поглощён commit-фазой.

Первичная синхронизация всегда начинается с `reconcile.preview`. Request включает
полный managed-folder snapshot и его browser fingerprint. Response содержит report,
store sequence, mapping revision, browser fingerprint и одноразовый preview token.
Apply имеет две фазы. `prepare` принимает исходный snapshot, `offset` и `limit`
1..50 и возвращает canonical-only nodes parent-first. Extension сохраняет mapping
после каждого browser create. `commit` принимает свежий полный snapshot и полный
mapping; helper повторно проверяет store sequence, mapping revision и структуру
обоих деревьев, создаёт backup и атомарно фиксирует mapping. Любое расхождение
возвращает `stale_preview`; повтор того же commit возвращает прежний результат.

Текущий формат preview request:

```json
{
  "browser_instance_id": "firefox-local",
  "browser_fingerprint": "sha256-hex",
  "snapshot": {
    "root_browser_id": "toolbar-folder-id",
    "nodes": [{"browser_id":"toolbar-folder-id","parent_browser_id":null,"type":"folder","title":"Waymarks","url":null,"position":0}],
    "unsupported": 0
  }
}
```

Fingerprint — lowercase SHA-256 от compact JSON snapshot с nodes, отсортированными
по UTF-8 bytes `browser_id`; JSON использует Go-compatible escapes для
`<`, `>`, `&`, U+2028 и U+2029. Preview ничего не изменяет. Report содержит количества
`create_in_browser`, `browser_only`, `unsupported`; token хранится с mode `0600` в
`$XDG_STATE_HOME/zer0-waymarks/browser-maps/previews` и действует 10 минут.
До появления mapping initial preview не объединяет записи по URL: одинаковый URL
сам по себе не доказывает идентичность закладок.

## Ordering и восстановление

В рамках одного локального store порядок задаёт `sequence`. Projection полностью
воспроизводима сортировкой по sequence; timestamp не определяет победителя.
Неполный последний event не подтверждается и игнорируется recovery до помещения
в quarantine/report. Durable write: записать temporary event, fsync файла,
атомарно переименовать, fsync каталога events и только затем ответить успехом.
Snapshot использует ту же последовательность с fsync родительского каталога после
rename.

# AI Context: zer0-waymarks

## Что строим

Системный launcher и собственную базу закладок для Wayland/Noctalia. Пользователь
ищет закладки независимо от браузера, выбирает браузер/профиль и открывает URL.
Опциональные WebExtensions синхронизируют специальную папку `Waymarks` со
встроенными закладками Firefox и Chromium.

## Зачем

Пользователь работает примерно с восемью браузерами. Их встроенная синхронизация
несовместима между семействами и часто привязана к облачным аккаунтам. Нужен один
локальный источник данных, принадлежащий пользователю.

## Ключевые решения

1. Проект называется `zer0-waymarks`; Noctalia ID планируется как `zer0/waymarks`.
2. Каноническая база принадлежит helper-у, браузерные базы напрямую не изменяются.
3. MVP синхронизирует только выделенную папку `Waymarks`, а не всё дерево браузера.
4. Автоматический bridge — собственный WebExtension + Native Messaging без сервера и аккаунта.
5. Helper — Go stdlib-first, Linux-only, без cgo.
6. Хранилище — versioned append-only JSON events + атомарный snapshot под XDG data dir.
7. Открытие браузера выполняется fixed argv без shell.
8. Разрешены по умолчанию только `https` и `http`; другие schemes требуют явной политики.
9. Удаления представлены tombstone; конфликт не должен молча терять данные.
10. Межустройственная Git-синхронизация не входит в первый MVP.

## Проверенные внешние факты на 2026-08-24

- Firefox WebExtensions API предоставляет CRUD и события закладок при permission `bookmarks`.
- Firefox и Chromium поддерживают Native Messaging через JSON stdin/stdout.
- Native host manifests и allowlist расширений различаются между Firefox и Chromium.
- Noctalia уже имеет community Bookmarks plugin, но он хранит пользовательский список shell-команд и не является cross-browser sync backend.

## Security boundaries

- Не исполнять URL, browser name, profile или title через shell.
- Не читать и не изменять SQLite/JSON-файлы профилей браузеров напрямую.
- Native host принимает сообщения только от allowlisted extension IDs.
- Ограничивать размер, глубину, URL length и batch size Native Messaging запросов.
- Не отправлять историю посещений, cookies и содержимое страниц.
- Логи не должны содержать query strings URL по умолчанию.
- Перед destructive reconcile создавать snapshot/backup и dry-run report.

## Целевая структура

```text
zer0-waymarks/
├── cmd/zer0-waymarks-helper/
├── internal/{store,protocol,browser}/
├── browser-extension/{shared,firefox,chromium}/
├── noctalia-plugin/waymarks/
├── packaging/arch/
├── docs/
└── .docs/execplans/zer0-waymarks/
```

## Правила для AI

1. Выполнять по одному milestone.
2. Сначала обновлять `status.md`, после — mandatory gates.
3. Не добавлять dependency без ADR.
4. Не тестировать на реальном профиле браузера: только disposable profiles/fixtures.
5. Не обещать мгновенную синхронизацию: неактивный браузер применит изменения при запуске/следующем pull.
6. Не добавлять full-tree sync, mobile или network service в MVP.

# Спецификация: zer0-waymarks

## Цель

Сделать независимую локальную базу закладок, системный поиск и безопасную
двустороннюю синхронизацию выделенной папки между Firefox и Chromium.

## Scope

- Go helper/store/protocol;
- Noctalia launcher и panel;
- Firefox/Chromium WebExtensions и Native Messaging host manifests;
- ручной CRUD, HTML import/export, выбор browser/profile;
- event sync, tombstones, conflicts, backups, dry-run;
- Arch packaging.

## Non-goals MVP

- пароли, формы, весь bookmark tree, mobile, cloud/server, Git sync;
- чтение browser profile files;
- автоматическое объединение дублей;
- поддержка произвольных URL schemes или shell commands.

## Constraints

- только synthetic/disposable browser profiles в тестах;
- данные под XDG directories с правами пользователя;
- Native Messaging origin allowlist обязателен;
- HTTP/HTTPS allowlist по умолчанию;
- закрытый browser синхронизируется только после запуска;
- Noctalia использует API 24+ argv-table.

## Открытые вопросы

- точный author/extension IDs и public namespace;
- какие Firefox/Chromium forks входят в первый support matrix;
- default conflict policy для одновременного title/move;
- формат Netscape HTML import edge cases;
- нужно ли шифровать будущий межустройственный remote или достаточно private SSH remote.

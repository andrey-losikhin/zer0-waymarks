# Netscape Bookmarks HTML

## Предварительный просмотр

Импорт всегда начинается с dry-run:

```text
zer0-waymarks-helper import-html bookmarks.html --dry-run --json
```

Команда парсит файл, проверяет структуру, title и URL policy, но не создаёт nodes
или import events. Report содержит `would_create` и точные URL, уже существующие в
активной базе. Дубликаты определяются по полному URL и автоматически пропускаются;
разные URL не объединяются.

## Применение

После проверки report:

```text
zer0-waymarks-helper import-html bookmarks.html --apply --json
```

До первого import event helper сохраняет внутренний snapshot в
`$XDG_DATA_HOME/zer0-waymarks/backups/import-<UTC timestamp>.json`. Импорт только
добавляет папки и закладки: существующие nodes не изменяются и не удаляются.
Operation IDs детерминированы по содержимому файла и позиции записи, поэтому retry
после прерванного запуска не создаёт повтор той же импортированной записи.

Backup содержит store revision/idempotency state для диагностики и будущего
rollback tooling. Простая замена текущего `snapshot.json` backup-файлом не является
rollback: более новые append-only events будут воспроизведены снова.

## Экспорт

```text
zer0-waymarks-helper export-html waymarks.html --json
```

Экспорт включает активные папки и закладки под managed root, не включает
tombstones/conflicts и атомарно заменяет целевой файл с mode `0600`.

## Поддерживаемый диалект

- Netscape `DL`/`H3`/`A HREF` с одной папкой или ссылкой на строку;
- quoted `HREF` (`"` или `'`) и HTML entities;
- вложенные папки;
- UTF-8, максимум 16 MiB и максимум 1 MiB на строку;
- только `http`/`https`, без userinfo и управляющих символов.

Файлы реальных browser profiles не читаются: тесты и примеры используют только
synthetic HTML и `example.test`.

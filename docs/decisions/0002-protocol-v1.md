# ADR 0002: локальный protocol v1

- Статус: accepted
- Дата: 2026-08-27

## Контекст

Noctalia UI, CLI и browser bridges должны работать с одной канонической базой.
Без стабильного формата search/add/open невозможно реализовать UI независимо от
store, а строковые shell-команды создают injection boundary.

## Решение

Использовать versioned JSON envelope и фиксированные CLI argv из
[`docs/PROTOCOL.md`](../PROTOCOL.md). Noctalia требует plugin API 24 и запускает
helper только argv-table. Записи используют opaque UUID, optimistic base revision,
tombstones и explicit conflict records. Локальная sequence определяет replay.

## Последствия

- Launcher и panel могут независимо реализовать поиск и добавление.
- Несовместимая эволюция требует protocol version 2.
- Межустройственный merge намеренно отложен; v1 не делает timestamp победителем.
- Реализация должна добавить golden contract tests до подключения UI.


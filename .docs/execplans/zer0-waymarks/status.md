# Статус: zer0-waymarks

- Статус: Milestone 4 в работе. Native Messaging, initial reconcile и постоянный
  Firefox mapping реализованы; обычные create/update/move/remove и periodic pull
  подключены в background script.
- Текущий slice: двусторонняя синхронизация и обязательные gates завершены;
  browser API покрыт VM/mock E2E, disposable Firefox запускается headless.
- Следующий этап: автоматизировать загрузку unsigned temporary extension через
  Marionette либо добавить локальный `web-ext` harness, затем выполнить полный UI E2E.

## Решения

- 2026-08-24: отдельный проект рядом с `zer0-waypass`.
- 2026-08-24: автоматический sync требует собственного WebExtension/native bridge.
- 2026-08-24: MVP ограничен папкой `Waymarks`.
- 2026-08-24: Go stdlib-first + JSON event store, без SQLite/daemon/cloud.
- 2026-08-24: Firefox реализуется раньше Chromium.
- 2026-08-27: актуальная документация Noctalia v5 подтверждает `plugin.toml`,
  `[[launcher_provider]]`, `[[panel]]` и безопасный `noctalia.runAsync(argv, callback)`;
  argv-table требует `plugin_api = 24`.
- 2026-08-27: community plugin должен иметь ID вида `<author>/<plugin>`, объявлять
  внешние команды в `dependencies` и документировать launcher prefix и panel command.
- 2026-08-27: принят protocol v1 для search/add/CRUD/open и Native Messaging;
  поиск и добавление через Noctalia panel обязательны для Milestone 6.
- 2026-08-27: helper реализует append-only events, atomic snapshot/replay,
  стабильные device/root IDs, idempotent operations и optimistic conflicts.
- 2026-08-27: Noctalia plugin API 24 реализован через argv-table; `/wm` ищет и
  открывает закладки, panel выполняет CRUD/search и выбирает browser/profile.
- 2026-08-27: HTML import ограничен 16 MiB, требует explicit dry-run/apply,
  пропускает exact URL duplicates и создаёт backup до import events.
- 2026-08-27: до выбора публичного namespace Firefox-каркас использует явно
  временный development ID `waymarks@zer0.local`; distribution не заявлена.
- 2026-08-27: Firefox native manifest запускает отдельный binary без аргументов;
  CLI helper сохраняет `native-host` только как диагностический режим.
- 2026-08-27: публикация extension не входит в задачу; локальная сборка фиксирует
  development ID `waymarks@zer0.local` и локальный Native Messaging allowlist.
- 2026-08-27: local bundle собирает оба binary и Firefox extension без cgo;
  user installer генерирует Native Messaging manifest с абсолютным путём.
- 2026-08-27: options UI не обходит всё дерево: кандидаты ищутся точечно, при
  неоднозначности выбор явный; snapshot читает только выбранное subtree.
- 2026-08-27: initial preview не объединяет записи по URL без mapping; report
  показывает обе стороны, а token хранится приватно под XDG state.
- 2026-08-28: initial reconcile двухфазный и возобновляемый; prepare пагинирует
  parent-first, commit проверяет полный свежий mapping и создаёт backup.
- 2026-08-28: post-reconcile push авторизуется постоянным mapping; Firefox хранит
  revisions/operation journal, применяет pull при запуске и раз в минуту и не
  продвигает cursor вслед за push.
- 2026-08-28: основной Luau-плагин `zer0/waymarks` локально подключён к Noctalia
  как path source `zer0-waymarks`; helper и конфигурация установлены в пользовательские XDG-пути,
  панель назначена на свободное сочетание `SUPER+W`.
- 2026-08-28: для локальной установки `~/.local/bin` добавлен в окружение Hyprland,
  чтобы запущенная из сессии Noctalia находила `zer0-waymarks-helper` по `PATH`.
- 2026-08-28: закладки получили необязательное локальное поле `note` до 4096 байт;
  оно хранится в canonical store, участвует в поиске и редактируется из панели
  Noctalia, но не синхронизируется в браузерные закладки.
- 2026-08-28: начат keyboard-first/category slice: helper обнаруживает известные
  локальные браузеры (включая Helium), хранит выбранный default, а разделы
  реализуются пользовательскими тегами canonical bookmarks.

## Блокеры

- Не подтверждена первая browser/fork support matrix.
- Полный disposable-profile Firefox UI E2E ещё не выполнен: `web-ext`/`geckodriver`
  отсутствуют. Headless Firefox/Marionette запуск и browser API VM-тесты пройдены.

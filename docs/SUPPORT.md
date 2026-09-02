# Support matrix v1

Проверено по публичной документации 2026-08-27. Точные минимальные версии
браузерных форков будут закреплены только после disposable-profile E2E.

| Компонент | Статус MVP | Контракт |
| --- | --- | --- |
| Noctalia v5 plugin API | Подтверждён | `plugin.toml`, launcher provider, panel |
| Noctalia plugin API 24 | Подтверждён | argv-table в `runAsync`; строковый shell не используется |
| Firefox WebExtensions | Foundation собран | permissions `bookmarks`, `nativeMessaging`, `storage`, `alarms`; disposable E2E впереди |
| Chromium MV3 | Подтверждён API | bookmarks и Native Messaging; suspension E2E впереди |
| Firefox/Chromium forks | Не заявлены | Добавляются только после проверки manifest paths и disposable E2E |

Плагин Noctalia имеет ID `zer0/waymarks`, launcher prefix `/wm`, panel entry
`zer0/waymarks:panel` и dependency `zer0-waymarks-helper`.

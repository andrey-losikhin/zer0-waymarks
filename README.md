# zer0-waymarks

`zer0-waymarks` — независимое локальное хранилище и системный поиск закладок
для Wayland/Noctalia. Одна база используется Firefox-, Chromium-подобными
браузерами и системным launcher-ом.

## Что хотим получить

- `SUPER+B` или `/wm` открывает поиск по общей базе;
- закладка открывается в выбранном браузере и профиле;
- добавление/изменение в поддерживаемом браузере попадает в остальные браузеры;
- данные принадлежат пользователю и не требуют облачного аккаунта;
- Git-синхронизацию между устройствами можно включить позже.

## Важная граница

Системное приложение не может надёжно получать события встроенных закладок всех
браузеров. Для автоматической двусторонней синхронизации нужен собственный
минимальный WebExtension с разрешениями `bookmarks` и `nativeMessaging`.
Без расширения остаются поиск, ручное добавление, импорт и открытие URL.

## Стек

- Go helper, standard library, `CGO_ENABLED=0`;
- append-only JSON event store и атомарный snapshot, без БД;
- Luau-плагин Noctalia API 24+;
- общий JavaScript core и отдельные Firefox/Chromium manifests;
- Native Messaging через stdin/stdout JSON;
- Arch Linux — первый packaged target.

Реализованы Go helper и Noctalia UI: `/wm` ищет общую базу, а panel выполняет
CRUD/search, выбирает browser/profile и безопасно открывает URL. Browser bridges
ещё находятся в разработке.

## Документация

- [Замысел](docs/VISION.md)
- [Архитектура](docs/ARCHITECTURE.md)
- [Решение по стеку](docs/decisions/0001-stack.md)
- [Protocol v1](docs/PROTOCOL.md)
- [Security contract](docs/SECURITY.md)
- [Support matrix](docs/SUPPORT.md)
- [Конфигурация браузеров](docs/CONFIGURATION.md)
- [Импорт и экспорт HTML](docs/IMPORT.md)
- [Локальная сборка Firefox bridge](docs/LOCAL_DEVELOPMENT.md)
- [AI-контекст](AGENTS.md)
- [ExecPlan](.docs/execplans/zer0-waymarks/plan.md)

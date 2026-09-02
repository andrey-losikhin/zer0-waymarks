# Security contract

## Процессы и URL

- Noctalia вызывает helper через `noctalia.runAsync({argv...}, callback)`; строковая
  shell-команда запрещена.
- Helper открывает только executable и profile args из локальной allowlisted
  конфигурации. URL всегда является отдельным последним argv.
- По умолчанию разрешены только абсолютные `https` и `http` URL. Userinfo,
  управляющие символы, NUL и URL длиннее 8192 байт отклоняются.
- Title не исполняется и ограничен 1024 UTF-8 байтами; query — 1024 байтами.

## Native Messaging

- Host manifest разрешает только зафиксированные Firefox extension IDs или
  Chromium origins. ID не берётся из сообщения.
- Максимальный входной frame — 1 MiB, batch — 200 операций, JSON depth — 32,
  строка — 64 KiB. Неизвестные поля допустимы для совместимости, неизвестные
  методы и версии отклоняются.
- Request ID и operation ID ограничены 128 байтами. В текущем read-only
  `changes.pull` foundation browser instance ID проверяется только синтаксически;
  до добавления мутаций он будет регистрироваться через reconcile mapping.
- Расширение запрашивает `bookmarks`, `nativeMessaging`, `storage` и `alarms`;
  tabs/history, cookies и содержимое страниц не читаются.

## Данные и логи

- Data/state directories создаются с mode `0700`, файлы — `0600`.
- Логи по умолчанию содержат event/request IDs и error code, но не полный URL,
  query string, title или payload.
- Snapshot записывается во временный файл в том же каталоге, fsync-ится и
  атомарно переименовывается; после rename fsync-ится родительский каталог. Event
  append использует fsync файла и каталога и сериализован advisory lock.
- Reconcile apply требует свежий dry-run token и предварительный backup.
- Тесты используют только `example.test`, временные XDG dirs и disposable browser
  profiles. Реальные browser databases не читаются и не изменяются.

## Отказы

Невалидный ввод не меняет store. Conflict не перезаписывает текущую запись.
Закрытый браузер не считается ошибкой доставки: он выполняет pull при следующем
запуске. Uninstall пакета не удаляет пользовательский XDG data directory.

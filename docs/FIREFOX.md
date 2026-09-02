# Firefox bridge: development foundation

Extension проверяет доступность host, сохраняет статус в `storage.local` и имеет
локальную options-страницу для явного выбора папки `Waymarks`. Если папки нет,
пользователь может создать её кнопкой; при нескольких совпадениях автоматического
выбора нет. Snapshot пропускает separators и URL вне `http`/`https`.

Options UI выполняет server-side `reconcile.preview` и показывает dry-run report.
Preview ничего не изменяет. После явного Apply extension parent-first импортирует
browser-only записи, постранично создаёт canonical-only записи, проверяет свежий
snapshot и фиксирует постоянный mapping. Незавершённая сессия и create intent
хранятся в `storage.local`, поэтому reconcile можно продолжить после перезапуска.

После reconcile background script отправляет create/update/move/remove из managed
folder и применяет `changes.pull` при запуске и раз в минуту. Изменения вне mapping
игнорируются. Cursor продвигается только после полной страницы; локальные operation
IDs и remote create intent подавляют echo и позволяют безопасный retry.

## Сборка extension

```sh
sh browser-extension/firefox/build.sh /tmp/zer0-waymarks-firefox
```

Полученный каталог можно загружать только во временный disposable profile.
Локальный ID `waymarks@zer0.local` должен совпадать в extension и native host
manifest; публикация extension не требуется.
После загрузки открыть настройки extension, проверить Native Messaging и явно
выбрать либо создать managed folder.

## Native host

Firefox запускает отдельный executable `zer0-waymarks-native-host` и на Linux
передаёт ему абсолютный путь к native-host manifest. Host принимает только этот
единственный проверенный аргумент; ручной диагностический запуск работает без него.
Он читает и пишет little-endian length-prefixed JSON frames, ограниченные 1 MiB. Packaging
manifest ожидает его в `/usr/lib/zer0-waymarks/zer0-waymarks-native-host`; при
локальной разработке нужно сгенерировать отдельный manifest с абсолютным путём к
собранному disposable binary, не редактируя browser profile databases.

Реализованы `changes.pull` (limit 1..200), initial и mapping-authorized
`changes.push`, `reconcile.preview`, постраничный `reconcile.apply/prepare` и
идемпотентный `reconcile.apply/commit`. Commit создаёт backup и сохраняет
instance mapping в приватном XDG state каталоге.

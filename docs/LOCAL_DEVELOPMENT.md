# Локальная сборка и запуск

Публикация extension не требуется. Локальная сборка использует фиксированный ID
`waymarks@zer0.local`, совпадающий с Firefox native-host allowlist.

## Собрать bundle

```sh
scripts/build-local.sh
```

Результат появится в `dist/local`: CLI helper, отдельный Native Messaging host и
собранный Firefox extension. Go runtime и cgo не требуются.

## Проверить локально

```sh
scripts/test-local.sh
```

Команда без сети запускает Go unit/race/vet, JS contract tests и Firefox VM-тесты
reconcile/background sync. Реальный browser profile она не открывает.

## Установить для текущего пользователя

```sh
scripts/install-local-firefox.sh
```

Installer копирует файлы в `$XDG_DATA_HOME/zer0-waymarks/local` (по умолчанию
`~/.local/share/zer0-waymarks/local`) и создаёт user-level Firefox manifest в
`~/.mozilla/native-messaging-hosts/zer0.waymarks.json` с абсолютным путём к host.
Системные каталоги и browser profile databases он не изменяет.
`HOME` и заданный `XDG_DATA_HOME` должны быть абсолютными путями.

Затем открыть `about:debugging`, выбрать **This Firefox**, **Load Temporary
Add-on** и указать `manifest.json` из пути, напечатанного installer-ом. Временное
расширение нужно загружать заново после перезапуска Firefox.

В настройках загруженного extension можно проверить Native Messaging, выбрать
существующую папку `Waymarks` или явно создать новую. Несколько одноимённых папок
не объединяются автоматически.

Кнопка dry-run выполняет `reconcile.preview` и показывает отчёт без изменений.
Кнопка Apply выполняет первичный двусторонний reconcile и сохраняет mapping.
После него background script синхронизирует managed folder при запуске и раз в
минуту. Закрытый Firefox догонит изменения при следующем запуске; мгновенная
синхронизация не обещается.

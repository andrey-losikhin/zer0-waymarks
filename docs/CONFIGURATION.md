# Конфигурация helper-а

Helper читает браузеры из `$XDG_CONFIG_HOME/zer0-waymarks/config.json` (обычно
`~/.config/zer0-waymarks/config.json`). Пример с синтетическими профилями:

```json
{
  "default_browser_id": "firefox",
  "browsers": [
    {
      "id": "firefox",
      "name": "Firefox",
      "executable": "/usr/bin/firefox",
      "args": ["--new-window"],
      "profiles": [
        {
          "id": "work",
          "name": "Work",
          "args": ["--profile", "/tmp/example-firefox-profile"]
        }
      ]
    }
  ]
}
```

Helper дополняет явный список найденными известными Linux-браузерами из `PATH`:
Helium, Firefox, Zen, LibreWolf, Floorp, Waterfox, Chrome, Chromium, Brave,
Vivaldi, Opera, Edge и Thorium. Явная запись с тем же `id` всегда имеет приоритет.
Выбранный в панели default сохраняется атомарно в `default_browser_id`; launcher
использует его при следующем открытии закладки.

Плагин получает только `id` и отображаемые `name`. Executable и argv профиля не
принимаются из UI. URL валидируется helper-ом и добавляется отдельным последним
аргументом; shell не используется.

Данные находятся в `$XDG_DATA_HOME/zer0-waymarks`: `device-id`, `snapshot.json`
и append-only `events/<device-id>/<sequence>.json`. Удаление пакета не должно
удалять этот каталог.

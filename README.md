# ItmoSportBot

Бот для [my.itmo](https://my.itmo.ru): расписание физры в Telegram и автозапись по шаблону слота (`/add`, `/list`, `/remove`).

## Быстрый старт

1. Склонируй репозиторий и скопируй пример конфига:
   ```powershell
   copy config.example.json config.json
   ```
   В **`config.json`** (файл в `.gitignore`, в git не попадает) укажи:
   - **`telegram.bot_token`** — у [@BotFather](https://t.me/BotFather);
   - **`telegram.admin_chat_ids`** — числовые id чатов в Telegram, кому доступна `/admin` (свой id можно узнать у ботов вроде @userinfobot);
   - **`building_ids`** — id корпусов из DevTools → Network → `sign/schedule` (можно несколько);
   - опционально **`users[]`** — для одноразового импорта в SQLite при первом запуске (см. код `ImportFromConfig`); иначе оставь **`"users": []`** — пользователи добавятся через **`/link`** в боте.

2. Сборка и запуск:
   ```powershell
   go build -o pewatch.exe ./cmd/pewatch
   .\pewatch.exe -config config.json
   ```
   Другой путь к конфигу: переменная окружения **`PE_CONFIG`**.

3. Секреты и БД: см. **[SECURITY.md](SECURITY.md)**. Локальная база: **`pewatch.sqlite`** рядом с конфигом (или **`PE_SQLITE`**).

## Команды в Telegram

| Команда | Описание |
|--------|----------|
| `/start`, `/help` | Справка |
| `/schedule YYYY-MM-DD` | Расписание по корпусам |
| `/add`, `/list`, `/remove` | Шаблоны автозаписи |

Интервал опроса шаблонов — **`recurring_poll_ms`** в конфиге (по умолчанию 20 000 мс).

## Лицензия

MIT, см. [LICENSE](LICENSE).

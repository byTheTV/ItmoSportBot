# ItmoSportBot

Бот для [my.itmo](https://my.itmo.ru): расписание физры в Telegram и автозапись по шаблону слота (`/add`, `/list`, `/remove`).

## Быстрый старт

1. Склонируй репозиторий и скопируй пример конфига:
   ```powershell
   copy config.example.json config.json
   ```
   В **`config.json`** (файл в `.gitignore`, в git не попадает) укажи:
   - **`token_key`** — секрет для AES-шифрования refresh-токенов в SQLite: **64 hex** (например `openssl rand -hex 32`) или **любая строка от 32 символов**. В примере — заглушка: **замени на свой** случайный ключ. Альтернатива: переменная окружения **`PE_TOKEN_KEY`** (имеет приоритет над полем в файле).
   - **`telegram.bot_token`** — у [@BotFather](https://t.me/BotFather);
   - **`telegram.admin_chat_ids`** — числовые id чатов в Telegram, кому доступна `/admin` (свой id можно узнать у ботов вроде @userinfobot);
   - опционально **`users[]`** — для одноразового импорта в SQLite при первом запуске (см. код `ImportFromConfig`); иначе оставь **`"users": []`** — пользователи добавятся через **`/link`** в боте.

   **Здания (корпуса)** — не секрет, в репозитории **`buildings.json`**. Список можно переопределить **`PE_BUILDINGS`**, полем **`buildings_file`** или **`building_ids`** / **`building_id`** в **`config.json`**.

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

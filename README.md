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

3. Локальная база: **`pewatch.sqlite`** рядом с конфигом (или **`PE_SQLITE`**). Шаблоны `/add` хранятся в таблицах **`recurring_templates`** и **`recurring_signed`**; при обновлении старый JSON в `user_recurring.templates_json` при первом запуске переносится в эти таблицы.

## Команды в Telegram

| Команда | Описание |
|--------|----------|
| `/start`, `/help` | Справка |
| `/schedule YYYY-MM-DD` | Расписание по корпусам |
| `/add`, `/list`, `/remove` | Шаблоны автозаписи |
| `/lead` [часы] | Мин. запас до начала пары для автозаписи (по умолчанию 36 ч; `0` = без ограничения) |

**Автозапись (шаблоны):** горизонт **18** дней вперёд по МСК (поле **`recurring_horizon_days`**). Опрос: **~20 с** в окне **`recurring_fast_window_start`…`end`** по МСК (по умолчанию 23:50–00:15), **~60 с** в остальное время — **`recurring_fast_poll_ms`** / **`recurring_poll_slow_ms`**. Устаревшее **`recurring_poll_ms`** задаёт «медленный» интервал, если нет **`recurring_poll_slow_ms`**.

## Лицензия

MIT, см. [LICENSE](LICENSE).

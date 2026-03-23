# Безопасность

- **Не коммить** реальный `config.json` с токеном бота и refresh-токенами ITMO. В репозитории — `config.example.json`; локально: `cp config.example.json config.json` (файл в `.gitignore`).
- Если секреты попали в git — **ротируйте** токен в [@BotFather](https://t.me/BotFather) и refresh в my.itmo.
- **`PE_TOKEN_KEY`** — только на сервере / в секретах, не в репозитории.
- Очистка истории от случайных секретов: [git filter-repo](https://github.com/newren/git-filter-repo) или BFG.

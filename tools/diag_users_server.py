#!/usr/bin/env python3
import sqlite3
import sys

path = sys.argv[1] if len(sys.argv) > 1 else "/opt/itmosportbot/pewatch.sqlite"
c = sqlite3.connect(path)

print("users_total:", c.execute("SELECT COUNT(*) FROM users").fetchone()[0])
print(
    "users_with_token:",
    c.execute(
        "SELECT COUNT(*) FROM users WHERE refresh_token_enc IS NOT NULL AND length(refresh_token_enc) > 0"
    ).fetchone()[0],
)
print("users:")
for row in c.execute(
    "SELECT telegram_chat_id, COALESCE(telegram_username,''), priority, min_lead_hours, length(refresh_token_enc) FROM users ORDER BY priority ASC, id ASC"
):
    print("  ", row)
print("templates_by_chat:")
for row in c.execute(
    "SELECT telegram_chat_id, COUNT(*) FROM recurring_templates GROUP BY telegram_chat_id ORDER BY telegram_chat_id"
):
    print("  ", row)

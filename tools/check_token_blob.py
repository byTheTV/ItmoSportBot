#!/usr/bin/env python3
"""Один раз на сервере: как хранится refresh_token_enc (PLAIN1 vs AES-GCM)."""
import sqlite3
import sys

path = sys.argv[1] if len(sys.argv) > 1 else "/opt/itmosportbot/pewatch.sqlite"
c = sqlite3.connect(path)
row = c.execute(
    "SELECT length(refresh_token_enc), refresh_token_enc FROM users WHERE refresh_token_enc IS NOT NULL LIMIT 1"
).fetchone()
if not row:
    print("no token rows")
    sys.exit(0)
ln, blob = row
if isinstance(blob, memoryview):
    blob = blob.tobytes()
print("blob_length:", ln)
print("first7_hex:", blob[:7].hex() if blob else "")
plain_prefix = b"PLAIN1:"
is_plain = len(blob) >= len(plain_prefix) and blob[: len(plain_prefix)] == plain_prefix
print("storage:", "PLAINTEXT_WITH_PLAIN1_PREFIX" if is_plain else "likely_AES_GCM_BINARY")
sys.exit(0)

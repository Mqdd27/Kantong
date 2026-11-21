#!/usr/bin/env python3
# Cron wake-gate wrapper for the finance-email skill (cron only runs scripts under ~/.hermes/scripts/).
# Copied as finance-email-fetch-<account>.py it reads that Kantong account's mailbox instead
# (IMAP_*_<ACCOUNT>, FINANCE_API_TOKEN_<ACCOUNT> in ~/.hermes/.env).
import os, re, runpy
m = re.fullmatch(r"finance-email-fetch-(\w+)\.py", os.path.basename(__file__))
if m:
    os.environ["FINANCE_ACCOUNT"] = m.group(1)
runpy.run_path(os.path.expanduser("~/.hermes/skills/finance-email/scripts/fetch.py"), run_name="__main__")

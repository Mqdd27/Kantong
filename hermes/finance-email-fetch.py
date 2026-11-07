#!/usr/bin/env python3
# Cron wake-gate wrapper for the finance-email skill (cron only runs scripts under ~/.hermes/scripts/).
import os, runpy
runpy.run_path(os.path.expanduser("~/.hermes/skills/finance-email/scripts/fetch.py"), run_name="__main__")

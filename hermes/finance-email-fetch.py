#!/usr/bin/env python3
# Cron wake-gate wrapper for the finance-email skill (cron only runs scripts under <profile>/scripts/).
# Runs the copy of the skill in the same Hermes profile as this wrapper, so each profile reads its own mailbox.
import runpy
from pathlib import Path
runpy.run_path(str(Path(__file__).resolve().parents[1] / "skills/finance-email/scripts/fetch.py"), run_name="__main__")

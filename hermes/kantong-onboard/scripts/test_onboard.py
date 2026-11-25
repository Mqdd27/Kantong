"""python3 -I test_onboard.py: the clone loses every first-account secret, and the route lands before access."""
import os, sys, tempfile
from pathlib import Path
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import onboard

p = Path(tempfile.mkdtemp()) / ".env"
p.write_text("OPENAI_API_KEY=llm\nIMAP_USER=me@x\nIMAP_PASSWORD=secret\nFINANCE_API_TOKEN=mine\nFINANCE_OWNER_NAME=ME\n"
             "TELEGRAM_BOT_TOKEN=bot\nTELEGRAM_ALLOWED_USERS=1\nAPI_SERVER_KEY=k\n# note\n")
onboard.set_env(p, {"FINANCE_API_URL": "http://k", "FINANCE_API_TOKEN": "budi", "TELEGRAM_HOME_CHANNEL": "2"}, drop=onboard.OWNER_ONLY)
env = onboard.read_env(p)
assert env == {"OPENAI_API_KEY": "llm", "FINANCE_API_URL": "http://k", "FINANCE_API_TOKEN": "budi", "TELEGRAM_HOME_CHANNEL": "2"}, env
assert "# note" in p.read_text() and oct(p.stat().st_mode)[-3:] == "600"

a = Path(tempfile.mkdtemp()) / ".env"
a.write_text("X=1\nTELEGRAM_ALLOWED_USERS=1\n")
onboard.set_env(a, {"TELEGRAM_ALLOWED_USERS": ",".join(onboard.allowed(onboard.read_env(a)) + ["2"])})
assert onboard.read_env(a) == {"X": "1", "TELEGRAM_ALLOWED_USERS": "1,2"}

cfg = onboard.add_route({"model": "m"}, "budi", "2")
assert cfg["gateway"] == {"multiplex_profiles": True, "profile_routes": [{"name": "budi", "platform": "telegram", "user_id": "2", "profile": "budi"}]}
assert onboard.routed(cfg, "2") and not onboard.routed(cfg, "3") and not onboard.routed({}, "2")
cfg = onboard.add_route({"gateway": {"profile_routes": None}}, "ani", "3")
assert len(cfg["gateway"]["profile_routes"]) == 1
print("ok")

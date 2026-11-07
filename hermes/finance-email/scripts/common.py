"""Shared helpers for the finance-email skill (stdlib only). No secrets are stored here."""
import json, os, time, urllib.request, urllib.error
from datetime import datetime, timezone, timedelta
from pathlib import Path

HOME = Path(os.environ.get("HERMES_HOME", Path.home() / ".hermes"))
STATE_DIR = HOME / "finance-email"
SKIPPED = STATE_DIR / "skipped.json"
LOG = STATE_DIR / "finance-email.log"
WIB = timezone(timedelta(hours=7))
SKIP_TTL_DAYS = 7


def load_env():
    """Read ~/.hermes/.env (cron strips provider secrets from the child env)."""
    env = {}
    p = HOME / ".env"
    if p.exists():
        for line in p.read_text().splitlines():
            line = line.strip()
            if line and not line.startswith("#") and "=" in line:
                k, v = line.split("=", 1)
                env[k.strip()] = v.strip().strip("\"'")
    for k, v in os.environ.items():
        env.setdefault(k, v)
    return env


def log(msg):
    STATE_DIR.mkdir(parents=True, exist_ok=True)
    with open(LOG, "a") as f:
        f.write(f"{datetime.now(WIB).isoformat(timespec='seconds')} {msg}\n")


def load_skipped():
    """Return {message_id: iso_ts}, pruned of entries older than 7 days."""
    try:
        data = json.loads(SKIPPED.read_text())
    except (FileNotFoundError, ValueError):
        return {}
    cutoff = datetime.now(WIB) - timedelta(days=SKIP_TTL_DAYS)
    out = {}
    for k, v in data.items():
        try:
            if datetime.fromisoformat(v) >= cutoff:
                out[k] = v
        except ValueError:
            pass
    return out


def save_skipped(d):
    STATE_DIR.mkdir(parents=True, exist_ok=True)
    tmp = SKIPPED.with_suffix(".tmp")
    tmp.write_text(json.dumps(d, indent=1))
    tmp.replace(SKIPPED)


def api(env, method, path, body=None, timeout=20):
    """Returns (status, parsed_json_or_text). Never raises on HTTP errors."""
    url = env["FINANCE_API_URL"].rstrip("/") + path
    data = json.dumps(body).encode() if body is not None else None
    req = urllib.request.Request(url, data=data, method=method, headers={
        "Authorization": "Bearer " + env["FINANCE_API_TOKEN"],
        "Content-Type": "application/json",
    })
    try:
        with urllib.request.urlopen(req, timeout=timeout) as r:
            status, raw = r.status, r.read().decode("utf-8", "replace")
    except urllib.error.HTTPError as e:
        status, raw = e.code, e.read().decode("utf-8", "replace")
    try:
        return status, json.loads(raw)
    except ValueError:
        return status, raw

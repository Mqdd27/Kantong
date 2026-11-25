"""Shared helpers for the finance-email skill (stdlib only). No secrets are stored here."""
import json, os, urllib.request, urllib.error
from datetime import datetime, timezone, timedelta
from pathlib import Path

_here = Path(__file__).resolve()
# The Hermes profile this copy of the skill is installed in (<home>/skills/finance-email/scripts/), so a
# second profile's copy reads that profile's .env (its own mailbox and Kantong token), whatever the caller's env.
HOME = _here.parents[3] if _here.parents[2].name == "skills" else Path(os.environ.get("HERMES_HOME", Path.home() / ".hermes"))
STATE_DIR = HOME / "finance-email"
SKIPPED = STATE_DIR / "skipped.json"
PENDING = STATE_DIR / "pending.json"
LOG = STATE_DIR / "finance-email.log"
WIB = timezone(timedelta(hours=7))
SKIP_TTL_DAYS = 7


def load_env():
    """Read this profile's .env (cron strips provider secrets from the child env)."""
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


def load_skipped(path=SKIPPED):
    """Return {message_id: iso_ts}, pruned of entries older than 7 days."""
    try:
        data = json.loads(path.read_text())
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


def save_skipped(d, path=SKIPPED):
    STATE_DIR.mkdir(parents=True, exist_ok=True)
    tmp = path.with_suffix(".tmp")
    tmp.write_text(json.dumps(d, indent=1))
    tmp.replace(path)


def remember_fetched(ids):
    """fetch.py records the Message-IDs it hands the agent, in this profile's state."""
    d = load_skipped(PENDING)
    now = datetime.now(WIB).isoformat(timespec="seconds")
    d.update({i: now for i in ids})
    save_skipped(d, PENDING)


def fetched(message_id):
    """True when this profile's fetch handed out the Message-ID. post.py posts nothing else, so the
    agent running another profile's copy of post.py can't put one person's mail in the other's account."""
    return message_id in load_skipped(PENDING)


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

"""Shared helpers for the finance-email skill (stdlib only). No secrets are stored here."""
import json, os, re, urllib.request, urllib.error
from datetime import datetime, timezone, timedelta
from pathlib import Path

HOME = Path(os.environ.get("HERMES_HOME", Path.home() / ".hermes"))
STATE_DIR = HOME / "finance-email"
LOG = STATE_DIR / "finance-email.log"
WIB = timezone(timedelta(hours=7))
SKIP_TTL_DAYS = 7

# Extra mailboxes: each Kantong account reads its own mailbox and posts with its own API token.
# Account "budi" uses IMAP_HOST_BUDI, IMAP_USER_BUDI, ..., FINANCE_API_TOKEN_BUDI. No fallback to the
# unsuffixed values: a missing token must never post someone's mail into the first account.
ACCOUNT_KEYS = ("IMAP_HOST", "IMAP_USER", "IMAP_PASSWORD", "FINANCE_API_TOKEN", "FINANCE_FWD_FROM", "FINANCE_OWNER_NAME")


def suffix(account):
    return "_" + re.sub(r"\W", "_", account).upper() if account else ""


def for_account(env, account):
    """env with KEY_<ACCOUNT> values in place of KEY; the first account ("") is env unchanged."""
    if not account:
        return env
    out = dict(env)
    for k in ACCOUNT_KEYS:
        out[k] = env.get(k + suffix(account), "")
    return out


def state_file(name, account=""):
    """Per-account state: skipped.json / skipped-budi.json, pending.json / pending-budi.json."""
    return STATE_DIR / (f"{name}-{account}.json" if account else f"{name}.json")


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


def load_map(path):
    """Return {message_id: value} from a state file, pruned of entries older than 7 days.
    Values are an iso timestamp, or [iso timestamp, account] in pending files."""
    try:
        data = json.loads(path.read_text())
    except (FileNotFoundError, ValueError):
        return {}
    cutoff = datetime.now(WIB) - timedelta(days=SKIP_TTL_DAYS)
    out = {}
    for k, v in data.items():
        try:
            if datetime.fromisoformat(v[0] if isinstance(v, list) else v) >= cutoff:
                out[k] = v
        except (ValueError, TypeError, IndexError):
            pass
    return out


def save_map(path, d):
    STATE_DIR.mkdir(parents=True, exist_ok=True)
    tmp = path.with_suffix(".tmp")
    tmp.write_text(json.dumps(d, indent=1))
    tmp.replace(path)


def load_skipped(account=""):
    return load_map(state_file("skipped", account))


def save_skipped(d, account=""):
    save_map(state_file("skipped", account), d)


def remember_pending(ids, account):
    """fetch records which account each woken email belongs to; post.py routes by it, not by the LLM."""
    path = state_file("pending", account)
    d = load_map(path)
    now = datetime.now(WIB).isoformat(timespec="seconds")
    for i in ids:
        d[i] = [now, account]
    save_map(path, d)


def owner_of(message_id):
    """The account whose fetch woke the agent for this Message-ID, or None when unknown."""
    for path in STATE_DIR.glob("pending*.json"):
        v = load_map(path).get(message_id)
        if v:
            return v[1]
    return None


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

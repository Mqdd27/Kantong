#!/usr/bin/env python3
"""kantong-onboard: give another person a Kantong account and their own Hermes profile on the shared bot.

  onboard.py add <name> <telegram_user_id> [--dry-run]
  onboard.py email <name>          stdin: {"host","user","password","fwd_from"?,"owner_name"?}

Runs only from the default profile, and the new profile never gets this skill. Secrets are written to
files and never printed, except the temporary Kantong password, which the person changes in the app.
"""
import json, os, re, shutil, subprocess, sys, urllib.request, urllib.error
from pathlib import Path

HOME = Path(__file__).resolve().parents[3]  # <home>/skills/kantong-onboard/scripts/onboard.py
KANTONG_DIR = Path(os.environ.get("KANTONG_DIR", "/opt/Kantong"))
NAME_RE = re.compile(r"^[a-z0-9]{1,32}$")
TG_RE = re.compile(r"^\d{1,20}$")
# Keys the clone copies from the first account that must never reach another person's profile.
OWNER_ONLY = re.compile(r"^(IMAP_\w+|FINANCE_\w+|TELEGRAM_\w+|API_SERVER_\w+)=")


def die(msg):
    print("GAGAL: " + msg)
    sys.exit(1)


def read_env(path):
    env = {}
    for line in path.read_text().splitlines() if path.exists() else []:
        if "=" in line and not line.lstrip().startswith("#"):
            k, v = line.split("=", 1)
            env[k.strip()] = v.strip().strip("\"'")
    return env


def set_env(path, values, drop=None):
    """Rewrites a .env: drops lines matching `drop`, replaces or appends `values`, keeps everything else."""
    lines = path.read_text().splitlines() if path.exists() else []
    out, seen = [], set()
    for line in lines:
        k = line.split("=", 1)[0].strip() if "=" in line and not line.lstrip().startswith("#") else None
        if k in values:
            out.append(f"{k}={values[k]}")
            seen.add(k)
        elif not (drop and drop.match(line)):
            out.append(line)
    out += [f"{k}={v}" for k, v in values.items() if k not in seen]
    path.write_text("\n".join(out) + "\n")
    path.chmod(0o600)


def allowed(env):
    return [u.strip() for u in env.get("TELEGRAM_ALLOWED_USERS", "").split(",") if u.strip()]


def add_route(cfg, name, tg):
    """Adds the telegram user -> profile route to a parsed config.yaml dict; returns it."""
    gw = cfg.setdefault("gateway", {}) or {}
    cfg["gateway"] = gw
    gw["multiplex_profiles"] = True
    routes = gw.setdefault("profile_routes", []) or []
    gw["profile_routes"] = routes
    routes.append({"name": name, "platform": "telegram", "user_id": tg, "profile": name})
    return cfg


def routed(cfg, tg):
    return any(str(r.get("user_id")) == tg for r in ((cfg or {}).get("gateway") or {}).get("profile_routes") or [])


def hermes():
    return shutil.which("hermes") or str(HOME / "hermes-agent/venv/bin/hermes")


def run(cmd, **kw):
    r = subprocess.run(cmd, capture_output=True, text=True, **kw)
    if r.returncode != 0:
        die(f"{cmd[0]} {cmd[1] if len(cmd) > 1 else ''}: {(r.stderr or r.stdout).strip()[-300:]}")
    return r.stdout


def kantong_api(env, token, method, path, body):
    req = urllib.request.Request(env["FINANCE_API_URL"].rstrip("/") + path, method=method,
                                 data=json.dumps(body).encode(),
                                 headers={"Authorization": "Bearer " + token, "Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=20) as r:
            return r.status, r.read().decode()
    except urllib.error.HTTPError as e:
        return e.code, e.read().decode()
    except OSError as e:
        return 0, str(e)


def add(name, tg, dry):
    import yaml  # Hermes' own venv has it; SKILL.md runs this script with that python
    if HOME.parent.name == "profiles":
        die("hanya bisa dijalankan dari profile default")
    if not NAME_RE.match(name):
        die("nama harus huruf kecil/angka, 1-32 karakter")
    if not TG_RE.match(tg):
        die("Telegram user ID harus angka (dari @userinfobot)")
    prof = HOME / "profiles" / name
    env_path, cfg_path = HOME / ".env", HOME / "config.yaml"
    env = read_env(env_path)
    cfg = yaml.safe_load(cfg_path.read_text()) or {}
    if prof.exists():
        die(f"profile {name} sudah ada")
    if tg in allowed(env) or routed(cfg, tg):
        die("Telegram ID itu sudah punya akses ke bot (kamu sendiri, atau sudah didaftarkan)")
    if not env.get("FINANCE_API_URL"):
        die("FINANCE_API_URL belum diisi di .env")
    steps = [f"akun Kantong {name}", f"profile Hermes {name} (clone, tanpa mailbox/token kamu)",
             f"route Telegram {tg} -> {name}", f"izinkan {tg} di bot", "cron email", "restart gateway 20 detik lagi"]
    if dry:
        print("DRY RUN, akan:\n- " + "\n- ".join(steps))
        return

    out = run(["docker", "compose", "exec", "-T", "money", "/money-manager", "adduser", name], cwd=KANTONG_DIR)
    pw = re.search(r"^password:\s+(\S+)", out, re.M)
    tok = re.search(r"^api token:\s+(\S+)", out, re.M)
    if not (pw and tok):
        die("output adduser tidak terbaca")

    run([hermes(), "profile", "create", name, "--clone"])
    set_env(prof / ".env", {"FINANCE_API_URL": env["FINANCE_API_URL"], "FINANCE_API_TOKEN": tok.group(1),
                            "TELEGRAM_HOME_CHANNEL": tg}, drop=OWNER_ONLY)
    shutil.rmtree(prof / "skills/kantong-onboard", ignore_errors=True)
    (prof / "scripts").mkdir(exist_ok=True)
    shutil.copy(HOME / "scripts/finance-email-fetch.py", prof / "scripts/finance-email-fetch.py")
    run([hermes(), "-p", name, "cron", "create", "--name", "finance-email", "--script", "finance-email-fetch.py",
         "--skill", "finance-email", "--deliver", "telegram", "10m"])

    # route first, then allow: an allowed sender without a route lands in this (the first account's) profile
    shutil.copy(cfg_path, cfg_path.with_name("config.yaml.bak-onboard"))
    cfg_path.write_text(yaml.safe_dump(add_route(cfg, name, tg), sort_keys=False, allow_unicode=True))
    set_env(env_path, {"TELEGRAM_ALLOWED_USERS": ",".join(allowed(env) + [tg])})
    run(["systemd-run", "--user", "--on-active=20", f"--unit=kantong-onboard-{name}",
         "systemctl", "--user", "restart", "hermes-gateway.service"])

    st, _ = kantong_api(env, tok.group(1), "PUT", "/api/me/telegram", {"bot_token": "", "chat_id": tg})
    notif = "aktif (pesan tes terkirim)" if st == 200 else "belum: dia tekan Start di bot dulu, lalu isi chat ID di Akun & perangkat"
    print(f"""SELESAI: {name}
- Login Kantong: username {name}, password sementara {pw.group(1)}
  Minta dia ganti di Akun & perangkat setelah login, lalu hapus pesan ini.
- Chat dengan bot ini sekarang masuk ke Hermes milik {name} (setelah gateway restart ~20 detik).
- Notif transaksi Kantong: {notif}
- Email bank: belum. Jalankan `email {name}` dengan data IMAP-nya.""")


def email(name):
    prof = HOME / "profiles" / name
    if not NAME_RE.match(name) or not prof.exists():
        die(f"profile {name} tidak ada")
    try:
        d = json.load(sys.stdin)
    except ValueError:
        die("stdin harus JSON")
    if not all(d.get(k) for k in ("host", "user", "password")):
        die("butuh host, user, password")
    set_env(prof / ".env", {"IMAP_HOST": d["host"], "IMAP_USER": d["user"], "IMAP_PASSWORD": d["password"],
                            "FINANCE_FWD_FROM": d.get("fwd_from", ""), "FINANCE_OWNER_NAME": d.get("owner_name", "")})
    r = subprocess.run([sys.executable, str(prof / "skills/finance-email/scripts/fetch.py")], capture_output=True, text=True)
    log = prof / "finance-email/finance-email.log"
    last = log.read_text().splitlines()[-1][26:] if log.exists() and log.read_text().strip() else ""
    ok = '"wakeAgent"' in r.stdout and ("imap error" not in last and "config missing" not in last)
    print(f"Email {name}: " + ("tersambung, cron membaca tiap 10 menit" if ok else f"gagal ({last or r.stderr.strip()[-200:]})"))
    print("Hapus pesan chat yang berisi password email.")


if __name__ == "__main__":
    a = sys.argv[1:]
    if len(a) >= 3 and a[0] == "add":
        add(a[1], a[2], "--dry-run" in a)
    elif len(a) == 2 and a[0] == "email":
        email(a[1])
    else:
        die(__doc__)

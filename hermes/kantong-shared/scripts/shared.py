#!/usr/bin/env python3
"""kantong-shared: manage kantong bersama through the Kantong API only (never the database).

  shared.py list
  shared.py create <nama> [--kind cash] [--balance 0]   new kantong bersama
  shared.py share <nama aset>                            turn one of your assets into a kantong bersama
  shared.py invite <kantong> <username>
  shared.py remove <kantong> <username>                  creator only; ask for a yes first
  shared.py delete <kantong> --confirm <nama persis>     creator only; deletes its transactions for everyone
Runs only from the default (owner's) Hermes profile, with that profile's FINANCE_API_TOKEN.
"""
import json, os, sys, urllib.error, urllib.request
from pathlib import Path

HERE = Path(__file__).resolve()
HOME = HERE.parents[3] if HERE.parents[2].name == "skills" else Path.home() / ".hermes"


def env():
    e = dict(os.environ)
    p = HOME / ".env"
    if p.exists():
        for line in p.read_text().splitlines():
            if "=" in line and not line.lstrip().startswith("#"):
                k, v = line.split("=", 1)
                e[k.strip()] = v.strip().strip("\"'")
    return e


def api(e, method, path, body=None):
    req = urllib.request.Request(e["FINANCE_API_URL"].rstrip("/") + path, method=method,
                                 data=json.dumps(body).encode() if body is not None else None,
                                 headers={"Authorization": "Bearer " + e["FINANCE_API_TOKEN"], "Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=20) as r:
            return r.status, json.loads(r.read() or b"null")
    except urllib.error.HTTPError as err:
        body = err.read().decode("utf-8", "replace")
        try:
            body = json.loads(body).get("error", body)
        except ValueError:
            pass
        return err.code, body


def die(msg):
    sys.exit("✗ " + msg)


def ok(st, body, done):
    if st not in (200, 201, 204):
        die(f"{st} {body}")
    print("✓ " + done)


def find(e, name):
    st, ks = api(e, "GET", "/api/shared")
    if st != 200:
        die(f"{st} {ks}")
    k = next((k for k in ks if k["name"].lower() == name.strip().lower()), None)
    return k or die(f"kantong bersama '{name}' tidak ditemukan")


def main(argv, e):
    cmd, args = (argv[0], argv[1:]) if argv else ("list", [])
    opt = lambda flag, default: args[args.index(flag) + 1] if flag in args[:-1] else default
    if cmd == "list":
        st, ks = api(e, "GET", "/api/shared")
        if st != 200:
            die(f"{st} {ks}")
        for k in ks:
            people = ", ".join(m["username"] + (" (pembuat)" if m["role"] == "owner" else "") for m in k["members"])
            print(f"{k['name']} · kamu {('pembuat' if k['role'] == 'owner' else 'anggota')} · {people}")
        if not ks:
            print("belum ada kantong bersama")
    elif cmd == "create" and args:
        body = {"name": args[0], "kind": opt("--kind", "cash"), "balance": float(opt("--balance", 0))}
        st, k = api(e, "POST", "/api/shared", body)
        ok(st, k, f"kantong bersama {args[0]} dibuat")
    elif cmd == "share" and args:
        st, p = api(e, "GET", "/api/assets")
        a = next((a for a in (p["assets"] if st == 200 else []) if a["name"].lower() == args[0].lower() and not a.get("shared_id")), None)
        if not a:
            die(f"aset pribadi '{args[0]}' tidak ditemukan")
        st, k = api(e, "POST", "/api/shared", {"asset_id": a["id"]})
        ok(st, k, f"{a['name']} sekarang kantong bersama")
    elif cmd == "invite" and len(args) == 2:
        k = find(e, args[0])
        st, body = api(e, "POST", f"/api/shared/{k['id']}/members", {"username": args[1]})
        ok(st, body, f"{args[1]} diundang ke {k['name']}")
    elif cmd == "remove" and len(args) == 2:
        k = find(e, args[0])
        m = next((m for m in k["members"] if m["username"] == args[1]), None) or die(f"{args[1]} bukan anggota {k['name']}")
        st, body = api(e, "DELETE", f"/api/shared/{k['id']}/members/{m['user_id']}")
        ok(st, body, f"{args[1]} dikeluarkan dari {k['name']} (transaksinya tetap di kantong)")
    elif cmd == "delete" and args:
        k = find(e, args[0])
        st, body = api(e, "DELETE", f"/api/shared/{k['id']}", {"confirm": opt("--confirm", "")})
        ok(st, body, f"kantong bersama {k['name']} dan semua transaksinya dihapus")
    else:
        die(__doc__)


if __name__ == "__main__":
    if HOME.parent.name == "profiles":
        die("hanya bisa dijalankan dari profile default")
    main(sys.argv[1:], env())

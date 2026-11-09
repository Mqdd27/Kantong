# Kantong: setup guide for an AI agent

> **Human:** hand this file to your coding agent (Claude Code, Codex, Cursor, Hermes, …), e.g.
> *"Follow docs/AGENT-SETUP.md and set up Kantong on my server."*
> The agent does the work and stops to ask you whenever it needs a password, an account, or a decision.

---

## Your role (agent)

You are setting up **Kantong**, a self-hosted money manager, for the human you are talking to. It ends up as:

```
Bank email ─▶ mailbox (IMAP) ─▶ Hermes cron (every 10 min) ─▶ Kantong API ─▶ PWA + Telegram notification
Telegram: screenshot of a portfolio / receipt photo ─▶ Hermes skill ─▶ Kantong API
```

Work through the phases **in order**. Each phase ends with a **Check**: run it and show the result before moving on.
If a check fails, fix it or stop and ask. Never skip ahead with a broken phase.

Talk to the human in their language (the project's docs are in Indonesian). Keep each message short, and say which phase you're on.

### Ground rules

1. **Secrets never pass through you.** Don't ask the human to paste passwords, app passwords, or tokens into the chat.
   Don't print secret values in commands or output (`cat .env` is forbidden; use `grep -c '^KEY=.\+' .env` to check that a key is filled in).
   - Secrets you can generate (`API_TOKEN`, `SESSION_SECRET`): write them straight into the file with `openssl rand -hex 32` inside the command.
   - Secrets only the human has (IMAP app password, Telegram bot token, LLM API key): add the key with an empty value, then ask the human to fill it in with an editor (`nano <file>`), and wait.
2. **Ask before anything outward-facing or hard to undo:** opening ports, DNS changes, deleting volumes/data, sending messages, or creating accounts. The human creates accounts and app passwords themselves.
3. **Don't commit or push** to the Kantong repo. `.env` must never be committed.
4. **Don't put personal data** (email addresses, full name, account numbers) into files inside the repo. It goes only in `.env` files.
5. Prefer the commands in this file. When something differs on the human's server (another OS, Podman, an existing reverse proxy), adapt and say what you changed.

---

## Phase 0: Gather info (ask all at once)

Ask the human these questions in one message, then wait for the answers:

1. **Server:** how do you reach it (`ssh user@host`)? Which OS? Is Docker already installed? Is it reachable from your phone (same network, Tailscale, public IP)?
2. **HTTPS (needed to install the PWA on a phone):** do you have a domain? Do you prefer **Tailscale Serve** (private, easiest), **Cloudflare Tunnel** (a public domain without opening ports), or a reverse proxy you already run (Caddy/Nginx)?
3. **Bank emails:** which banks/brokers send notifications, and which mailbox receives them (Gmail? iCloud?)
4. **Telegram:** do you already have a Telegram bot for Hermes? (If not: create one with @BotFather; you'll do that yourself.)
5. **Hermes model:** which LLM provider/model will Hermes use? To read screenshots and receipts, it needs a model that can read images.
6. **Owner name:** your full name exactly as it appears on bank transfer receipts. It's used to detect transfers to your own accounts, and it's stored only in `~/.hermes/.env`.

---

## Phase 1: Server prerequisites

```sh
ssh <user@host>
docker --version && docker compose version
git --version
python3 --version   # Hermes scripts need Python 3.9+
```

If Docker is missing, install it with the official guide for the server OS (https://docs.docker.com/engine/install/) and add the user to the `docker` group. Ask before you do either.

**Check:** all four commands print a version, and `docker ps` runs without sudo.

---

## Phase 2: Install Kantong

```sh
sudo mkdir -p /opt/Kantong && sudo chown "$USER" /opt/Kantong
git clone https://github.com/Mqdd27/Kantong.git /opt/Kantong
cd /opt/Kantong
cp .env.example .env
chmod 600 .env
# generated secrets: written directly, never printed
sed -i "s/^API_TOKEN=.*/API_TOKEN=$(openssl rand -hex 32)/" .env
sed -i "s/^SESSION_SECRET=.*/SESSION_SECRET=$(openssl rand -hex 32)/" .env
```

Then:

- **`APP_PASSWORD`** (the login password for the web app): ask the human to set it with `nano /opt/Kantong/.env`.
- **`HOST_BIND`:** keep `127.0.0.1` when a reverse proxy or tunnel on the same server fronts the app. Set it to the server's Tailscale IP only if the phone will connect directly over the tailnet.
- **`COOKIE_SECURE`:** keep `true` (HTTPS). Set it to `false` only for a temporary plain-HTTP test.

On macOS the `sed -i` above needs `sed -i ''`.

```sh
docker compose up -d --build
set -a; . ./.env; set +a
curl -s "http://$HOST_BIND:$HOST_PORT/healthz"        # -> ok
curl -s -o /dev/null -w '%{http_code}\n' -H "Authorization: Bearer $API_TOKEN" "http://$HOST_BIND:$HOST_PORT/api/categories"   # -> 200
```

**Check:** `ok`, then `200`. `grep -c '^\(API_TOKEN\|SESSION_SECRET\|APP_PASSWORD\)=.\+' .env` prints `3`.

---

## Phase 3: HTTPS access (for the PWA)

Use the option the human picked in Phase 0. Ask before changing DNS or the tunnel/proxy config.

| Option | What to do |
|---|---|
| Tailscale Serve | `sudo tailscale serve --bg http://127.0.0.1:8080`. The app is then at `https://<machine>.<tailnet>.ts.net` (only on the tailnet). |
| Cloudflare Tunnel | Add a public hostname to the tunnel pointing to `http://127.0.0.1:8080`. |
| Caddy | `kantong.example.com { reverse_proxy 127.0.0.1:8080 }` |

**Check:** ask the human to open the HTTPS URL on their phone, log in with `APP_PASSWORD`, and use "Add to Home Screen". The login must succeed. If it loops back to the login page over HTTP, `COOKIE_SECURE` is the cause.

---

## Phase 4: Install Hermes

Hermes is the agent that reads the emails and talks to the human on Telegram: https://github.com/NousResearch/hermes-agent

1. Install it with the **official instructions in that repo** (don't guess the commands), for the same user that will run the cron.
2. Configure the model from Phase 0 (it must read images for Phase 6) and connect the Telegram bot by following Hermes' docs. The bot token goes into Hermes' own config, which the human fills in.
3. Find the `hermes` binary. If it isn't on `PATH`, note its full path (e.g. `~/.hermes/hermes-agent/venv/bin/hermes`) and use that from here on.

**Check:** `hermes --version` works, and the human sends "hi" to the bot and gets a reply.

---

## Phase 5: Bank email → Kantong (`finance-email` skill)

Full reference: [hermes-email.md](hermes-email.md). Summary of what you do:

1. **Mailbox (the human does this):**
   - Create an **app password** for the IMAP mailbox (iCloud: appleid.apple.com → App-Specific Passwords; Gmail: myaccount.google.com/apppasswords, needs 2-Step Verification).
   - If the bank emails land in Gmail but another mailbox is read, set up the Gmail forwarding filter from section 1 of hermes-email.md.
2. **Install the skill:**
   ```sh
   cd /opt/Kantong
   cp -R hermes/finance-email ~/.hermes/skills/
   mkdir -p ~/.hermes/scripts && cp hermes/finance-email-fetch.py ~/.hermes/scripts/
   ```
3. **Env:** append these keys to `~/.hermes/.env` (empty values where only the human knows them), then `chmod 600 ~/.hermes/.env`:
   ```sh
   IMAP_HOST=imap.mail.me.com          # or imap.gmail.com
   IMAP_USER=                          # human fills in
   IMAP_PASSWORD=                      # human fills in (the app password)
   FINANCE_API_URL=http://127.0.0.1:8080
   FINANCE_API_TOKEN=                  # copy without printing: see below
   FINANCE_FWD_FROM=                   # optional: address used to forward emails by hand
   FINANCE_OWNER_NAME=                 # human fills in
   ```
   Copy the token without printing it:
   ```sh
   t=$(grep '^API_TOKEN=' /opt/Kantong/.env | cut -d= -f2-) && sed -i "s/^FINANCE_API_TOKEN=.*/FINANCE_API_TOKEN=$t/" ~/.hermes/.env && unset t
   ```
   `FINANCE_API_URL` must be the address Kantong is reachable at from where Hermes runs (`http://<HOST_BIND>:<HOST_PORT>`).
4. **Banks:** compare the human's banks (Phase 0) with `ALLOW` in `~/.hermes/skills/finance-email/scripts/fetch.py` and the sections in `parsing-rules.md`. For a missing bank, follow section 4 of hermes-email.md (add the sender, then let Hermes draft the rules from real emails, with no account numbers or balances).
5. **Test:**
   ```sh
   python3 ~/.hermes/skills/finance-email/scripts/fetch.py | tail -n 1
   tail -n 20 ~/.hermes/finance-email/finance-email.log
   ```
   The last line is `{"wakeAgent": false}` (nothing new) or `{"wakeAgent": true, ...}` (new emails). Neither is an error. An IMAP or 401 error in the log is.
6. **Schedule:**
   ```sh
   hermes cron create --name finance-email --script finance-email-fetch.py --skill finance-email --deliver telegram 10m
   hermes cron list
   ```

**Check:** the human makes a small transaction (e.g. a Rp10.000 e-wallet top-up). Within 10 minutes it appears in the PWA. Wrong categories get fixed in **Review** with "Ingat pilihan" on.

---

## Phase 6: Assets & receipts from Telegram (optional)

```sh
cd /opt/Kantong
cp -R hermes/finance-assets hermes/finance-receipt ~/.hermes/skills/
python3 ~/.hermes/skills/finance-assets/scripts/assets.py      # prints assets, one JSON per line (empty on a new install)
python3 ~/.hermes/skills/finance-receipt/scripts/receipt.py    # prints categories and rules
```

Restart Hermes if it doesn't see the new skills. References: [hermes-assets.md](hermes-assets.md), [hermes-receipt.md](hermes-receipt.md).

**Check:** the human sends a receipt photo to the bot, and the transaction appears in the PWA (or the bot says "sudah tercatat").

---

## Phase 7: Telegram notification per transaction (optional)

Kantong sends its own short Telegram message for each transaction that comes from email.

1. Ask the human to fill in `TELEGRAM_BOT_TOKEN` and `TELEGRAM_CHAT_ID` in `/opt/Kantong/.env`. It can be the same bot as Hermes. The chat ID is the human's own chat with the bot.
2. Apply it: `cd /opt/Kantong && docker compose up -d --force-recreate`

**Check:** the next email transaction produces a notification. If none arrives: `docker logs kantong-money-1 2>&1 | grep -i telegram`

---

## Phase 8: Backups (recommended)

Kantong already keeps 14 daily backups inside its volume. To also keep a copy on the human's own computer, offer the commands in the README's **Backup** section, including the optional `crontab` line. Ask before adding a cron job.

---

## Phase 9: Hand-off

Finish with a short report to the human:

- PWA URL, plus where `APP_PASSWORD` lives (`/opt/Kantong/.env`). Don't print the password itself.
- What runs automatically: the email cron every 10 minutes, daily backups at 03:00 WIB.
- Which banks are covered, and which are still unverified.
- How to update: `cd /opt/Kantong && git pull && docker compose up -d --build`
- Anything you skipped or that still needs the human.

### Troubleshooting

| Symptom | Look at |
|---|---|
| `healthz` fails | `docker compose logs --tail=50`. A missing or too-short `SESSION_SECRET` (needs 32+ characters) stops the app. |
| 401 from scripts | `FINANCE_API_TOKEN` ≠ `API_TOKEN`. Copy it again with the command in Phase 5. |
| IMAP `AUTHENTICATIONFAILED` | An account password was used instead of an app password. |
| Bank emails never picked up | The sender isn't in `ALLOW`, or the forward filter isn't working. Only `INBOX`/`Junk` from the last 3 days are read. |
| PWA shows old UI after an update | The service worker cache. Close the app and open it again (or refresh twice). |

---
name: kantong-onboard
description: "Use when the owner asks in chat to give another person a Kantong account (their own data, Hermes profile and Telegram routing on this bot), or to connect that person's bank mailbox."
version: 1.0.0
platforms: [linux]
metadata:
  hermes:
    tags: [finance, kantong, onboarding]
---
# kantong-onboard

Sets up another person in one step:
- a Kantong account with its own data;
- their own Hermes profile (own memory and Kantong token; the LLM key is shared);
- a route on this bot so their chats go to that profile;
- their Kantong notifications through the server's bot;
- an email cron, which stays idle until their mailbox is connected.

This skill lives only in the default (first account's) profile, and the script refuses to run anywhere else.

## Rules
- Only for the owner of this Hermes. Use it only when the owner asks for a named person; never when the person asks for themselves.
- Run the script; never edit `config.yaml`, `.env` or profiles by hand. A wrong route sends someone's chats into the owner's profile.
- Never repeat a password or token, except passing the script's output on unchanged (it shows a temporary Kantong password once, which the person changes in the app).
- Profiles share this server's user, so only onboard people the owner trusts with the server: say so once if the owner seems unsure.

## Add a person
Ask for: a short name (lowercase letters/digits), and their Telegram user ID (they get it from @userinfobot). Ask them to press Start on this bot first.
```
${HERMES_SKILL_DIR}/../../hermes-agent/venv/bin/python ${HERMES_SKILL_DIR}/scripts/onboard.py add <name> <telegram_id> --dry-run
```
Show the plan, get a yes, then run it again without `--dry-run`. Reply with the script's output as is. The gateway restarts by itself about 20 seconds later; say the person can start chatting after that.

## Connect their bank mailbox
Ask for the IMAP host, address and app password (not the account password). Optionally ask for the forwarding address and their name as printed on bank transfers. Then:
```
echo '{"host":"imap.gmail.com","user":"x@gmail.com","password":"...","fwd_from":"","owner_name":""}' | ${HERMES_SKILL_DIR}/../../hermes-agent/venv/bin/python ${HERMES_SKILL_DIR}/scripts/onboard.py email <name>
```
Reply with the script's output, including its reminder to delete the chat message that held the password.

## Not covered
Removing a person, renaming one, and their own LLM key are manual; see `docs/hermes-email.md` section 8.

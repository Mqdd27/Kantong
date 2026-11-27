---
name: kantong-shared
description: "Use when the owner asks in chat to set up, create, update or delete a kantong bersama (a shared wallet several Kantong accounts use), or to invite or remove one of its members."
version: 1.0.0
platforms: [linux]
metadata:
  hermes:
    tags: [finance, kantong, shared]
---
# kantong-shared

Manages kantong bersama through the Kantong API with the owner's token. One kantong is one asset that
all its members see and edit; its balance counts once in each member's total. Transactions join it when
their account (Akun) is the kantong's exact name.

This skill lives only in the default (owner's) profile, and the script refuses to run anywhere else.
It only calls the API; never touch the database or `money.db`.

## Rules
- Only for the owner of this Hermes, about kantong they created. Members change balances and transactions in the app, not here.
- **Remove** and **delete** always need an explicit yes in chat first. For delete, say that every transaction in the kantong is gone for all members and can't be undone, then ask the owner to type the kantong's name; pass exactly what they typed as `--confirm`.
- Reply with the script's output line as is. A `✗` line is the server's answer (for example a name clash: the invited person already has an asset with that name and must rename it first).

## Commands
```
python3 ${HERMES_SKILL_DIR}/scripts/shared.py list
python3 ${HERMES_SKILL_DIR}/scripts/shared.py create "<nama>" --kind cash --balance 0
python3 ${HERMES_SKILL_DIR}/scripts/shared.py share "<nama aset pribadi>"
python3 ${HERMES_SKILL_DIR}/scripts/shared.py invite "<kantong>" <username>
python3 ${HERMES_SKILL_DIR}/scripts/shared.py remove "<kantong>" <username>
python3 ${HERMES_SKILL_DIR}/scripts/shared.py delete "<kantong>" --confirm "<nama persis>"
```
- **Setup / create:** `create` for a new one (`kind`: `cash`, `bank`, `ewallet`, …; `balance` = current amount), or `share` to turn an existing asset into a kantong bersama (its balance carries over).
- **Update:** members come and go with `invite` / `remove`. A removed member's past transactions stay in the kantong. Renaming or a new balance: in the app (Aset → the kantong → Edit), since every member may do that.
- **Delete:** `delete` with the confirmation above.

Usernames are Kantong account names (from `kantong-onboard`), not Telegram names.

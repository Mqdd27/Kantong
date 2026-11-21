"""python3 test_routing.py: an extra mailbox never borrows the first account's token, and post.py
routes by the Message-IDs fetch recorded."""
import os, sys, tempfile

os.environ["HERMES_HOME"] = tempfile.mkdtemp()
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
from common import for_account, remember_pending, owner_of, load_skipped, save_skipped

env = {"IMAP_USER": "me@x", "FINANCE_API_TOKEN": "mine", "FINANCE_API_URL": "http://k", "IMAP_USER_BUDI": "budi@x"}
assert for_account(env, "") is env
budi = for_account(env, "budi")
assert budi["IMAP_USER"] == "budi@x" and budi["FINANCE_API_URL"] == "http://k"
assert budi["FINANCE_API_TOKEN"] == "", "missing FINANCE_API_TOKEN_BUDI must not fall back to the first account's"

remember_pending(["<a>"], "")
remember_pending(["<b>"], "budi")
assert owner_of("<a>") == "" and owner_of("<b>") == "budi" and owner_of("<c>") is None

save_skipped({"<b>": "2099-01-01T00:00:00+07:00"}, "budi")
assert load_skipped("budi") and not load_skipped("")
print("ok")

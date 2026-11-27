"""python3 -I test_shared.py: names resolve to ids and delete sends the typed confirmation, against a fake API."""
import json, os, sys, threading
from http.server import BaseHTTPRequestHandler, HTTPServer
sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import shared

calls = []
class H(BaseHTTPRequestHandler):
    def handle_one(self, code, body):
        self.send_response(code)
        self.end_headers()
        if body is not None:
            self.wfile.write(json.dumps(body).encode())
    def do(self):
        n = int(self.headers.get("Content-Length") or 0)
        calls.append((self.command, self.path, json.loads(self.rfile.read(n)) if n else None))
        if self.path == "/api/shared" and self.command == "GET":
            return self.handle_one(200, [{"id": 4, "name": "Dapur", "role": "owner", "members": [
                {"user_id": 1, "username": "owner", "role": "owner"}, {"user_id": 2, "username": "budi", "role": "member"}]}])
        if self.path == "/api/shared/4" and calls[-1][2] != {"confirm": "Dapur"}:
            return self.handle_one(400, {"error": "confirm must be the kantong's exact name"})
        self.handle_one(204, None)
    do_GET = do_POST = do_DELETE = do
    def log_message(self, *a): pass

srv = HTTPServer(("127.0.0.1", 0), H)
threading.Thread(target=srv.serve_forever, daemon=True).start()
e = {"FINANCE_API_URL": f"http://127.0.0.1:{srv.server_port}", "FINANCE_API_TOKEN": "t"}

shared.main(["remove", "dapur", "budi"], e)
assert calls[-1] == ("DELETE", "/api/shared/4/members/2", None), calls[-1]
try:
    shared.main(["delete", "Dapur"], e)  # no --confirm: the server refuses, the script exits non-zero
    raise AssertionError("delete without confirm passed")
except SystemExit as x:
    assert "400" in str(x), x
shared.main(["delete", "Dapur", "--confirm", "Dapur"], e)
assert calls[-1] == ("DELETE", "/api/shared/4", {"confirm": "Dapur"}), calls[-1]
print("ok")

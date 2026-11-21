"""python3 -I test_profiles.py: each profile's copy uses its own .env and posts only what its own fetch found."""
import os, shutil, subprocess, sys, tempfile
from pathlib import Path

src = Path(__file__).resolve().parent
root = Path(tempfile.mkdtemp())
for home in ("a", "b"):
    shutil.copytree(src, root / home / "skills/finance-email/scripts")
    (root / home / ".env").write_text(f"FINANCE_API_TOKEN={home}\n")

probe = "import common; print(common.HOME.name, common.load_env()['FINANCE_API_TOKEN']); common.remember_fetched(['<x>'])"
env = {**os.environ, "HERMES_HOME": str(root / "a")}  # a caller env pointing at the wrong profile changes nothing
out = subprocess.run([sys.executable, "-c", probe], cwd=root / "b/skills/finance-email/scripts", env=env,
                     capture_output=True, text=True, check=True).stdout.split()
assert out == ["b", "b"], out

check = "import common; print(common.fetched('<x>'))"
for home, want in (("a", "False"), ("b", "True")):
    got = subprocess.run([sys.executable, "-c", check], cwd=root / home / "skills/finance-email/scripts",
                         capture_output=True, text=True, check=True).stdout.strip()
    assert got == want, (home, got)
print("ok")

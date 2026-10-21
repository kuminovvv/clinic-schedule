"""Poll published GitHub Releases and atomically deploy their verified site.tar.gz.

Installed on the server as a systemd timer running as unprivileged site-deploy.
No repository credentials or inbound deploy SSH access are required.
"""

from __future__ import annotations

import hashlib
import json
import os
import re
import shutil
import subprocess
import sys
import tarfile
import tempfile
from pathlib import Path
from urllib.error import HTTPError
from urllib.request import Request, urlopen


ROOT = Path("/var/www/kuminovvv.com")
RELEASES = ROOT / "releases"
CURRENT = ROOT / "current"
DEPLOYED = ROOT / ".deployed-tag"
API = "https://api.github.com/repos/Kuminovvv/personal-site/releases/latest"
MAX_BYTES = 50 * 1024 * 1024
TAG = re.compile(r"v(\d+)\.(\d+)\.(\d+)\Z")


def get_release() -> dict | None:
    request = Request(API, headers={"User-Agent": "personal-site-deployer", "Accept": "application/vnd.github+json"})
    try:
        with urlopen(request, timeout=20) as response:
            return json.load(response)
    except HTTPError as error:
        if error.code == 404:
            print("No published releases yet")
            return None
        raise


def download(url: str, target: Path, expected_hash: str) -> None:
    if not url.startswith("https://github.com/kuminovvv/personal-site/releases/download/"):
        raise ValueError("Unexpected release asset URL")
    digest = hashlib.sha256()
    size = 0
    request = Request(url, headers={"User-Agent": "personal-site-deployer"})
    with urlopen(request, timeout=60) as response, target.open("wb") as output:
        while block := response.read(1024 * 1024):
            size += len(block)
            if size > MAX_BYTES:
                raise ValueError("Release archive exceeds size limit")
            digest.update(block)
            output.write(block)
    if digest.hexdigest() != expected_hash:
        raise ValueError("GitHub Release SHA-256 mismatch")


def unpack(archive: Path, destination: Path) -> None:
    with tarfile.open(archive, "r:gz") as package:
        for member in package.getmembers():
            path = Path(member.name)
            if (not member.isfile() and not member.isdir()) or path.is_absolute() or ".." in path.parts:
                raise ValueError(f"Unsafe archive member: {member.name}")
        package.extractall(destination, filter="data")
    for relative in ("index.html", "projects/index.html", "contacts/index.html"):
        if not (destination / relative).is_file():
            raise ValueError(f"Missing expected file: {relative}")
    if not (destination / "_next/static").is_dir():
        raise ValueError("Missing static Next.js assets")


def check_site() -> None:
    for path in ("/", "/projects/", "/projects/soobota/", "/contacts/"):
        subprocess.run(
            ["curl", "--noproxy", "*", "--fail", "--silent", "--show-error", "--max-time", "12",
             "--resolve", "kuminovvv.com:443:127.0.0.1", "-o", "/dev/null", f"https://kuminovvv.com{path}"],
            check=True,
        )


def deploy() -> int:
    release = get_release()
    if not release:
        return 0
    name = release.get("tag_name", "")
    match = TAG.fullmatch(name)
    if not match or release.get("draft") or release.get("prerelease"):
        raise ValueError("Not a stable, published SemVer release")

    last = DEPLOYED.read_text().strip() if DEPLOYED.exists() else ""
    if last == name:
        print(f"Already deployed: {name}")
        return 0
    if TAG.fullmatch(last) and tuple(map(int, match.groups())) <= tuple(map(int, TAG.fullmatch(last).groups())):
        print(f"Ignoring older release {name} (current: {last})")
        return 0

    assets = [asset for asset in release.get("assets", []) if asset.get("name") == "site.tar.gz" and asset.get("state") == "uploaded"]
    if len(assets) != 1:
        raise ValueError("Expected exactly one uploaded site.tar.gz asset")
    asset = assets[0]
    expected = asset.get("digest", "")
    if not re.fullmatch(r"sha256:[a-f0-9]{64}", expected):
        raise ValueError("GitHub has no verifiable SHA-256 asset digest")
    if not isinstance(asset.get("size"), int) or not 0 < asset["size"] <= MAX_BYTES:
        raise ValueError("Unexpected release asset size")

    target = RELEASES / name
    if target.exists():
        raise ValueError(f"Release directory already exists: {target}")
    archive_fd, archive_name = tempfile.mkstemp(prefix="download-", suffix=".tar.gz", dir=ROOT / "uploads")
    os.close(archive_fd)
    archive = Path(archive_name)
    staging = Path(tempfile.mkdtemp(prefix=".staging-", dir=RELEASES))
    previous = CURRENT.resolve() if CURRENT.is_symlink() else None
    switched = False
    try:
        download(asset["browser_download_url"], archive, expected.removeprefix("sha256:"))
        unpack(archive, staging)
        staging.chmod(0o755)
        staging.rename(target)
        temp_link = ROOT / f".current-{name}"
        temp_link.symlink_to(target, target_is_directory=True)
        os.replace(temp_link, CURRENT)
        switched = True
        check_site()
        DEPLOYED.write_text(name + "\n", encoding="utf-8")
        print(f"Deployed and verified {name}: {target}")
        return 0
    except Exception:
        if switched and previous:
            rollback_link = ROOT / f".rollback-{name}"
            rollback_link.symlink_to(previous, target_is_directory=True)
            os.replace(rollback_link, CURRENT)
            print(f"Health check failed; rolled back to {previous}", file=sys.stderr)
        if target.exists():
            shutil.rmtree(target)
        raise
    finally:
        archive.unlink(missing_ok=True)
        if staging.exists():
            shutil.rmtree(staging)


if __name__ == "__main__":
    try:
        sys.exit(deploy())
    except Exception as failure:
        print(f"Deployment failed: {failure}", file=sys.stderr)
        sys.exit(1)

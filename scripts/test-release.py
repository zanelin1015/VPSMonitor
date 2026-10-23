#!/usr/bin/env python3
"""Exercise release.sh with real temporary Git remotes and offline tool doubles."""
import json
import os
from pathlib import Path
import shutil
import subprocess
import tempfile
import unittest

RELEASE_SCRIPT = Path(__file__).resolve().with_name("release.sh")

FAKE_TOOL = r'''#!/usr/bin/env python3
import json, os, pathlib, shutil, subprocess, sys
name = pathlib.Path(sys.argv[0]).name
args = sys.argv[1:]
with open(os.environ["TRACE"], "a") as out:
    out.write(json.dumps([name, args]) + "\n")
failure = os.environ.get("FAIL_STEP", "")
if name == "npm":
    if args == ["ci"]:
        pathlib.Path("node_modules").mkdir(exist_ok=True)
    elif args == ["run", "build"]:
        output = pathlib.Path("../webui/dist/index.html")
        output.parent.mkdir(parents=True, exist_ok=True)
        output.write_text(pathlib.Path("../VERSION").read_text())
        # Bundled vendor template literals can legitimately contain trailing
        # spaces; source whitespace checks must not rewrite generated JS.
        (output.parent / "vendor.js").write_text("const css = `vendor template  \n`;\n")
    elif failure == "frontend":
        sys.exit(1)
elif name == "go":
    if args[:2] == ["version", "-m"]:
        print(pathlib.Path(args[2]).read_text())
    elif args == ["test", "./..."] and failure == "go":
        sys.exit(1)
elif name == "gh":
    state_dir = pathlib.Path(os.environ["RELEASE_STATE"])
    state_file = state_dir / "state.json"
    if args[:2] == ["auth", "status"]:
        pass
    elif args[:2] == ["repo", "view"]:
        print("fixture/repo\tmain")
    elif args[:2] == ["release", "list"]:
        print(os.environ.get("EXISTING_RELEASE", ""))
    elif args[:2] == ["release", "create"]:
        if failure == "upload": sys.exit(1)
        assets = args[3:args.index("--repo")]
        if failure == "missing-asset": assets = assets[:-1]
        state = {"draft": True, "assets": [pathlib.Path(a).name for a in assets]}
        state_file.write_text(json.dumps(state))
        for asset in assets: shutil.copy(asset, state_dir)
        shutil.copy(args[args.index("--notes-file") + 1], state_dir / "notes.md")
    elif args[:2] == ["release", "view"]:
        state = json.loads(state_file.read_text())
        if args[args.index("--json") + 1] == "assets":
            print("\n".join(state["assets"]))
        else:
            print("Release: offline-fixture (draft=" + str(state["draft"]) + ")")
    elif args[:2] == ["release", "download"]:
        output = pathlib.Path(args[args.index("--dir") + 1]) / "checksums.txt"
        shutil.copy(state_dir / "checksums.txt", output)
        if failure == "remote-checksum": output.write_text("corrupted")
    elif args[:2] == ["release", "edit"]:
        state = json.loads(state_file.read_text())
        state["draft"] = False
        state_file.write_text(json.dumps(state))
    else:
        raise SystemExit("unexpected gh call: " + repr(args))
'''

FAKE_PACKAGER = r'''#!/usr/bin/env python3
import io, os, pathlib, subprocess, sys, tarfile, zipfile
assert os.environ["VPSMONITOR_SKIP_WEB_BUILD"] == "1"
version = os.environ["VPSMONITOR_BUILD_VERSION"]
assert os.environ["VPSMONITOR_SERVER_BUILD_VERSION"] == version
assert os.environ["VPSMONITOR_CLIENT_BUILD_VERSION"] == version
assert os.environ["PACKAGE_PREFIX"] == "VPSMonitor"
commit = subprocess.check_output(["git", "rev-parse", "HEAD"], text=True).strip()
metadata = "\tbuild\tvcs.revision=" + commit
failure = os.environ.get("FAIL_STEP", "")
if failure == "metadata": metadata = metadata.replace(commit, "wrong-commit")
for target in sys.argv[1:]:
    platform, arch = target.split("/")
    for role in ["server", "client"]:
        package = "VPSMonitor-" + role + "-" + platform + "-" + arch
        binary_arch = "arm64" if failure == "platform" and arch == "arm" else arch
        values = {package + "/VERSION": "0.0.0" if failure == "package-version" else version + "\n",
                  package + "/bridge-" + role + (".exe" if platform == "windows" else ""): metadata + "\n\tpath\tbridge-core/cmd/bridge-" + role + "\n\tbuild\tGOOS=" + platform + "\n\tbuild\tGOARCH=" + binary_arch}
        if platform == "windows":
            with zipfile.ZipFile("dist/" + package + ".zip", "w") as archive:
                for name, value in values.items(): archive.writestr(name, value)
        else:
            with tarfile.open("dist/" + package + ".tar.gz", "w:gz") as archive:
                for name, value in values.items():
                    content = value.encode()
                    info = tarfile.TarInfo(name)
                    info.size = len(content)
                    archive.addfile(info, io.BytesIO(content))
'''


class ReleaseTests(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory(prefix="vpsmonitor-release-test-")
        self.addCleanup(self.temp.cleanup)
        base = Path(self.temp.name)
        self.root = base / "project with spaces"
        self.root.mkdir()
        self.remote = base / "remote.git"
        self.bin = base / "bin"
        self.bin.mkdir()
        self.state = base / "release state"
        self.state.mkdir()
        self.trace = base / "trace.jsonl"
        self.notes = base / "release notes.md"
        self.notes.write_text("## 更新\n- 内容含 $() 和 `反引号`，应按原文保留。\n")
        packager = base / "packager.py"
        packager.write_text(FAKE_PACKAGER)
        for name in ["npm", "go", "gh"]:
            tool = self.bin / name
            tool.write_text(FAKE_TOOL)
            tool.chmod(0o755)
        self.env = {**os.environ, "PATH": str(self.bin) + os.pathsep + os.environ["PATH"],
                    "TRACE": str(self.trace), "RELEASE_STATE": str(self.state), "FAKE_PACKAGER": str(packager),
                    "VPSMONITOR_SERVER_BUILD_VERSION": "9.9.9", "VPSMONITOR_CLIENT_BUILD_VERSION": "9.9.9"}
        for directory in ["scripts", "web", "webui/dist"]:
            (self.root / directory).mkdir(parents=True, exist_ok=True)
        shutil.copy(RELEASE_SCRIPT, self.root / "scripts/release.sh")
        (self.root / "scripts/build.sh").write_text('#!/usr/bin/env bash\nset -euo pipefail\ngo test ./...\npython3 "$FAKE_PACKAGER" "$@"\n')
        (self.root / "web/package.json").write_text(json.dumps({"scripts": {"test:one": "unused", "test:two": "unused", "build": "unused"}}))
        (self.root / "web/package-lock.json").write_text("{}")
        (self.root / "webui/dist/index.html").write_text("old")
        for name in ["VERSION", "VERSION.server", "VERSION.client"]:
            (self.root / name).write_text("0.3.54\n")
        (self.root / ".gitignore").write_text("/dist/\n/web/node_modules/\n")
        (self.root / "dist").mkdir()
        (self.root / "dist/VPSMonitor-server-linux-amd64.tar.gz").write_text("old archive")
        (self.root / "dist/checksums.txt").write_text("old checksums")
        self.git("init", "-b", "main")
        self.git("config", "user.name", "Offline Release Test")
        self.git("config", "user.email", "offline@example.invalid")
        self.git("add", ".")
        self.git("add", "-f", "dist/VPSMonitor-server-linux-amd64.tar.gz", "dist/checksums.txt")
        self.git("commit", "-m", "Fixture source")
        subprocess.run(["git", "init", "--bare", str(self.remote)], check=True, capture_output=True)
        self.git("remote", "add", "origin", str(self.remote))
        self.git("push", "-u", "origin", "main")
        self.original_head = self.git("rev-parse", "HEAD")

    def git(self, *args):
        return subprocess.check_output(["git", *args], cwd=self.root, text=True, stderr=subprocess.DEVNULL).strip()

    def run_release(self, *args, success=True, failure=""):
        result = subprocess.run(["/bin/bash", "scripts/release.sh", *args], cwd=self.root,
                                env={**self.env, "FAIL_STEP": failure}, capture_output=True, text=True)
        self.assertEqual(result.returncode == 0, success, result.stdout + result.stderr)
        return result

    def publish(self, **kwargs):
        return self.run_release("--version", "0.3.55", "--notes-file", str(self.notes), "--yes", **kwargs)

    def calls(self):
        return [json.loads(line) for line in self.trace.read_text().splitlines()] if self.trace.exists() else []

    def test_dry_run_never_changes_files_or_calls_network(self):
        (self.root / "new-source.go").write_text("uncommitted")
        result = self.run_release("--dry-run", "--notes-file", str(self.notes))
        self.assertIn("v0.3.55", result.stdout)
        self.assertEqual(self.calls(), [])
        self.assertEqual(self.git("rev-parse", "HEAD"), self.original_head)
        self.assertFalse((self.root / "dist/release-v0.3.55.md").exists())
        self.assertEqual((self.root / "dist/checksums.txt").read_text(), "old checksums")

    def test_invalid_arguments_and_empty_notes(self):
        for args in [("--version",), ("--yes",), ("--version", "0.3.54"), ("--version", "00.3.55"), ("--unknown",)]:
            with self.subTest(args=args): self.run_release(*args, success=False)
        self.notes.write_text(" \n\t")
        self.publish(success=False)
        self.assertEqual(self.calls(), [])

    def test_uncommitted_source_is_refused(self):
        (self.root / "new-source.go").write_text("uncommitted")
        self.publish(success=False)
        self.assertEqual(self.calls(), [])
        self.assertEqual(self.git("rev-parse", "HEAD"), self.original_head)

    def test_existing_tag_and_release_are_not_overwritten(self):
        self.git("tag", "v0.3.55")
        self.publish(success=False)
        self.git("tag", "-d", "v0.3.55")
        self.env["EXISTING_RELEASE"] = "v0.3.55"
        self.publish(success=False)
        self.assertEqual(self.git("rev-parse", "HEAD"), self.original_head)

    def test_wrong_branch_and_outdated_source_are_refused(self):
        self.git("checkout", "-b", "feature")
        self.publish(success=False)
        self.git("checkout", "main")
        upstream = subprocess.check_output(["git", "commit-tree", "HEAD^{tree}", "-p", "HEAD"], cwd=self.root, input="upstream update\n", text=True).strip()
        self.git("push", "origin", upstream + ":refs/heads/main")
        self.publish(success=False)
        self.assertEqual(self.git("rev-parse", "HEAD"), self.original_head)
        self.assertFalse((self.root / "dist/release-v0.3.55.md").exists())

    def test_noninteractive_without_confirmation_flag_fails_early(self):
        self.run_release("--version", "0.3.55", "--notes-file", str(self.notes), success=False)
        self.assertEqual(self.calls(), [])
        self.assertEqual(self.git("rev-parse", "HEAD"), self.original_head)

    def test_publish_exact_assets_and_keep_literal_notes(self):
        (self.root / "dist").mkdir(exist_ok=True)
        (self.root / "dist/old-package.zip").write_text("must not upload")
        (self.root / ".DS_Store").write_text("local")
        (self.root / "artifacts").mkdir()
        (self.root / "artifacts/local.txt").write_text("local")
        self.publish()
        state = json.loads((self.state / "state.json").read_text())
        self.assertFalse(state["draft"])
        self.assertEqual(len(state["assets"]), 11)
        self.assertNotIn("old-package.zip", state["assets"])
        self.assertEqual((self.state / "notes.md").read_text(), self.notes.read_text())
        self.assertEqual(len((self.state / "checksums.txt").read_text().splitlines()), 10)
        self.assertEqual(self.git("rev-parse", "v0.3.55^{}"), self.git("rev-parse", "HEAD"))
        self.assertEqual(self.git("rev-parse", "origin/main"), self.git("rev-parse", "HEAD"))
        changed_paths = self.git("diff", "--name-only", "HEAD").splitlines()
        self.assertTrue(changed_paths and all(path.startswith("dist/") for path in changed_paths), "only tracked packaging outputs may remain changed")
        self.assertEqual([args for name, args in self.calls() if name == "npm"], [["ci"], ["run", "test:one"], ["run", "test:two"], ["run", "build"]])
        self.assertNotIn(".DS_Store", self.git("ls-tree", "--name-only", "HEAD"))

    def test_draft_is_not_published(self):
        self.run_release("--version", "v0.3.55", "--notes-file", str(self.notes), "--yes", "--draft")
        self.assertTrue(json.loads((self.state / "state.json").read_text())["draft"])

    def test_failures_before_push_never_create_remote_tag_or_release(self):
        # Subtests need independent source commits; failed packaging deliberately
        # leaves a local release commit for inspection, just like the real script.
        for failure in ["frontend", "go", "package-version", "metadata", "platform"]:
            with self.subTest(failure=failure):
                self.publish(success=False, failure=failure)
                self.assertEqual(self.git("ls-remote", "origin", "refs/tags/v0.3.55"), "")
                self.assertFalse((self.state / "state.json").exists())
                self.git("reset", "--hard", self.original_head)

    def test_incomplete_remote_assets_stay_draft(self):
        self.publish(success=False, failure="missing-asset")
        self.assertTrue(json.loads((self.state / "state.json").read_text())["draft"])

    def test_corrupted_remote_checksums_stay_draft(self):
        self.publish(success=False, failure="remote-checksum")
        self.assertTrue(json.loads((self.state / "state.json").read_text())["draft"])


if __name__ == "__main__":
    unittest.main(verbosity=2)

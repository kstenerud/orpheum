# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for check_released_compose.py: a published compose file changed without a new
# ABOUTME: compose-file version, or a frozen version changed, fails (D462 §6).
import contextlib
import io
import os
import subprocess
import sys
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent))
sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import check_released_compose as crc  # noqa: E402
from gitrepo import GIT_ENV, Repo  # noqa: E402

FILE = "# the header\nservices:\n  orpheum-core:\n    command: [start]\n"
FROZEN = "cmd/orpheum/testdata/released/compose-v1/"
VALUES = "package deployment\n\nconst ComposeFileVersion = {}\n"


def tree(number: int = 1, frozen: str = FILE, published: str = FILE) -> dict[str, str]:
    """A tree at compose-file version number, its published files published, version 1 frozen."""
    files = {"internal/deployment/values.go": VALUES.format(number)}
    for path, name in crc.PUBLISHED.items():
        files[path] = published
        files[FROZEN + name] = frozen
    return files


def problems(root: Path, *argv: str) -> tuple[int, str]:
    with contextlib.redirect_stderr(io.StringIO()) as err:
        code = crc.main([*argv, "--root", str(root)])
    return code, err.getvalue()


def tag(repo: Repo, name: str) -> None:
    subprocess.run(["git", "tag", name], cwd=repo.root, check=True, env={**os.environ, **GIT_ENV})


class TheTree(unittest.TestCase):
    def test_published_files_frozen_as_their_version_pass_at_the_release_too(self):
        self.assertEqual(problems(Repo(tree()).root), (0, ""))
        self.assertEqual(problems(Repo(tree()).root, "--tag", "v0.4.0"), (0, ""))

    def test_a_comments_wording_needs_no_new_version(self):
        self.assertEqual(problems(Repo(tree(published=FILE.replace("the header", "a header"))).root), (0, ""))

    def test_a_change_without_a_bump_fails_naming_the_steps(self):
        code, said = problems(Repo(tree(published=FILE + "    restart: always\n")).root)
        self.assertEqual(code, 1)
        self.assertIn("deploy/omv/compose.yaml is not", said)
        self.assertIn("bump deployment.ComposeFileVersion", said)

    def test_a_bumped_version_not_frozen_fails_and_every_earlier_one_must_stay(self):
        code, said = problems(Repo(tree(number=2)).root)
        self.assertEqual(code, 1)
        self.assertIn("compose-v2/ is missing: run make freeze-compose", said)
        files = tree(number=3)
        code, said = problems(Repo(files).root)
        self.assertIn("compose-v2/ is missing: every version", said)

    def test_a_name_that_is_no_release_tag_is_refused(self):
        self.assertEqual(problems(Repo(tree()).root, "--tag", "main")[0], 2)


class Freezing(unittest.TestCase):
    def test_freeze_writes_the_current_version_and_never_changes_a_frozen_one(self):
        changed = FILE + "    restart: always\n"
        repo = Repo(tree(number=2, published=changed))
        self.assertEqual(problems(repo.root, "--freeze"), (0, ""))
        self.assertEqual((repo.root / "cmd/orpheum/testdata/released/compose-v2/omv.yaml").read_text(), changed)
        self.assertEqual(problems(repo.root), (0, ""))
        repo = Repo(tree(published=changed))
        code, said = problems(repo.root, "--freeze")
        self.assertEqual(code, 1)
        self.assertIn("compose-v1 is frozen already", said)
        self.assertEqual((repo.root / FROZEN / "omv.yaml").read_text(), FILE)


class TheTags(unittest.TestCase):
    def test_a_frozen_version_a_tag_published_is_never_changed(self):
        repo = Repo(tree())
        repo.commit("v0.4.0")
        tag(repo, "v0.4.0")
        self.assertEqual(problems(repo.root), (0, ""))
        edited = FILE + "    restart: always\n"
        repo.commit("edit the frozen file and the published one", {FROZEN + "omv.yaml": edited,
                                                                  "deploy/omv/compose.yaml": edited})
        code, said = problems(repo.root)
        self.assertEqual(code, 1)
        self.assertIn("is not deploy/omv/compose.yaml as v0.4.0 published it", said)

    def test_a_tag_before_the_frozen_versions_is_not_checked(self):
        repo = Repo({"deploy/omv/compose.yaml": "old\n", "internal/deployment/values.go": VALUES.format(0)})
        repo.commit("v0.3.2")
        tag(repo, "v0.3.2")
        repo.commit("v1", tree())
        self.assertEqual(problems(repo.root), (0, ""))


class Wiring(unittest.TestCase):
    ROOT = Path(__file__).resolve().parents[2]

    def test_check_pr_runs_it_and_the_release_runs_it_on_its_tag_before_building(self):
        makefile = (self.ROOT / "Makefile").read_text()
        self.assertRegex(makefile, r"(?m)^PR_FAST_STEPS := .*\bcheck-source\b")
        self.assertRegex(makefile, r"(?m)^check-source:.*\n\t.*scripts/check_released_compose\.py$")
        self.assertRegex(makefile, r"(?m)^freeze-compose:.*\n\t.*scripts/check_released_compose\.py --freeze$")
        workflow = (self.ROOT / ".github/workflows/release-image.yml").read_text()
        self.assertIn('scripts/check_released_compose.py --tag "$TAG"', workflow)
        self.assertRegex(workflow, r"(?m)^  image:\n(?:    .*\n)*?    needs: frozen-compose$")

    def test_it_reads_the_real_version_and_names_every_published_route(self):
        source = (self.ROOT / crc.VERSION_SOURCE).read_text()
        self.assertGreaterEqual(crc.compose_version(source), 1)
        household = (self.ROOT / "internal/deployment/household.go").read_text()
        for route in ("omv", "synology", "portainer", "casaos", "docker"):
            self.assertIn(f'"{route}"', household)
        self.assertEqual(len(crc.PUBLISHED), 5)


if __name__ == "__main__":
    unittest.main()

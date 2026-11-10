# SPDX-License-Identifier: AGPL-3.0-or-later
# ABOUTME: Tests for check_released_compose.py: a release tagged without its compose files
# ABOUTME: frozen, or frozen unlike the tag's, fails, at the release and in check-pr (D461 §1).
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

OMV = "services:\n  orpheum-core:\n    command: [start]\n"
START = ('\tflags.StringVar(&options.lanAddress, "lan-address", "", "x")\n'
         '\tflags.Func("media", "a media folder", func(folder string) error {\n')
FLAGS = "lan-address\nmedia\n"
FROZEN = "cmd/orpheum/testdata/released/v0.3.4/"


def released(extra: dict[str, str] | None = None) -> dict[str, str]:
    """A tree whose v0.3.4 is frozen as it publishes it."""
    return {"deploy/omv/compose.yaml": OMV, "cmd/orpheum/start.go": START,
            FROZEN + "omv.yaml": OMV, FROZEN + "start-flags.txt": FLAGS, **(extra or {})}


def problems(root: Path, *argv: str) -> tuple[int, str]:
    with contextlib.redirect_stderr(io.StringIO()) as err:
        code = crc.main([*argv, "--root", str(root)])
    return code, err.getvalue()


def tag(repo: Repo, name: str) -> None:
    subprocess.run(["git", "tag", name], cwd=repo.root, check=True, env={**os.environ, **GIT_ENV})


class AtTheRelease(unittest.TestCase):
    def test_a_tag_frozen_as_it_publishes_passes(self):
        self.assertEqual(problems(Repo(released()).root, "--tag", "v0.3.4"), (0, ""))

    def test_a_tag_without_its_folder_fails_naming_it(self):
        code, said = problems(Repo(released()).root, "--tag", "v0.3.5")
        self.assertEqual(code, 1)
        self.assertIn("released/v0.3.5/ is missing", said)

    def test_a_frozen_file_unlike_the_published_one_fails(self):
        repo = Repo(released({"deploy/omv/compose.yaml": OMV + "    restart: always\n"}))
        code, said = problems(repo.root, "--tag", "v0.3.4")
        self.assertEqual(code, 1)
        self.assertIn("omv.yaml is not deploy/omv/compose.yaml", said)

    def test_start_flags_must_name_every_flag_start_takes(self):
        start = START + '\tflags.IntVar(&options.composeVersion, "compose-version", 1, "x")\n'
        code, said = problems(Repo(released({"cmd/orpheum/start.go": start})).root, "--tag", "v0.3.4")
        self.assertEqual(code, 1)
        self.assertIn("start-flags.txt names", said)
        self.assertIn("compose-version", said)

    def test_a_name_that_is_no_release_tag_is_refused(self):
        self.assertEqual(problems(Repo(released()).root, "--tag", "main")[0], 2)


class InCheckPR(unittest.TestCase):
    def test_every_tag_from_the_first_frozen_needs_its_folder(self):
        repo = Repo(released())
        repo.commit("v0.3.4")
        tag(repo, "v0.3.4")
        self.assertEqual(problems(repo.root), (0, ""))
        tag(repo, "v0.3.5")
        tag(repo, "v0.3.2")
        code, said = problems(repo.root)
        self.assertEqual(code, 1)
        self.assertIn("released/v0.3.5/ is missing", said)
        self.assertNotIn("v0.3.2", said, "a tag before FIRST_FROZEN was before any household pasted a file")

    def test_a_frozen_file_must_be_the_tags_own(self):
        repo = Repo(released())
        repo.commit("v0.3.4")
        tag(repo, "v0.3.4")
        repo.commit("edit the frozen file", {FROZEN + "omv.yaml": OMV + "# edited\n"})
        code, said = problems(repo.root)
        self.assertEqual(code, 1)
        self.assertIn("is not deploy/omv/compose.yaml as v0.3.4 published it", said)


class Wiring(unittest.TestCase):
    ROOT = Path(__file__).resolve().parents[2]

    def test_check_pr_runs_it_and_the_release_runs_it_on_its_tag_before_building(self):
        makefile = (self.ROOT / "Makefile").read_text()
        self.assertRegex(makefile, r"(?m)^PR_FAST_STEPS := .*\bcheck-released-compose\b")
        self.assertRegex(makefile, r"(?m)^check-released-compose:.*\n\t.*scripts/check_released_compose\.py$")
        workflow = (self.ROOT / ".github/workflows/release-image.yml").read_text()
        self.assertIn('scripts/check_released_compose.py --tag "$TAG"', workflow)
        self.assertRegex(workflow, r"(?m)^  image:\n(?:    .*\n)*?    needs: frozen-compose$")


class Versions(unittest.TestCase):
    def test_tags_are_kept_from_the_first_frozen_on_in_release_order(self):
        tags = ["v0.3.10", "v0.3.2", "v0.3.3", "v0.4.0-rc1", "v0.3.9", "nightly"]
        self.assertEqual(crc.frozen_tags(tags), ["v0.3.3", "v0.3.9", "v0.3.10"])

    def test_the_real_start_names_media_among_its_flags(self):
        source = (Path(__file__).resolve().parents[2] / crc.START_SOURCE).read_text()
        self.assertIn("media", crc.start_flags(source))
        self.assertIn("lan-address", crc.start_flags(source))


if __name__ == "__main__":
    unittest.main()

# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Tests for the development tooling: the pinned-FFmpeg fetcher refuses a wrong hash,
# ABOUTME: the dev library covers every Cut A path, and the dev loop renders and watches right.
import sys
import tempfile
import unittest
from pathlib import Path

sys.path.insert(0, str(Path(__file__).resolve().parent.parent))
import dev  # noqa: E402
import dev_media  # noqa: E402
import fetch_ffmpeg  # noqa: E402
import fetch_mkvtoolnix  # noqa: E402


class FetchMkvtoolnix(unittest.TestCase):
    def test_the_pin_is_a_sha256_for_the_one_platform_with_a_portable_build(self):
        self.assertEqual(set(fetch_mkvtoolnix.PINS), {"linux/amd64"})
        for digest in fetch_mkvtoolnix.PINS.values():
            self.assertRegex(digest, r"^[0-9a-f]{64}$")

    def test_an_unpinned_platform_gets_nothing_rather_than_a_guess(self):
        root = Path(tempfile.mkdtemp())
        self.assertIsNone(fetch_mkvtoolnix.install("darwin/arm64", root))
        self.assertFalse((root / ".tools").exists())


class FetchFfmpeg(unittest.TestCase):
    def test_every_pin_has_an_asset_and_a_sha256(self):
        for target, (suffix, digest) in fetch_ffmpeg.PINS.items():
            self.assertIn(suffix, fetch_ffmpeg.asset_name(target))
            self.assertRegex(digest, r"^[0-9a-f]{64}$")

    def test_a_wrong_hash_is_refused_and_deleted(self):
        path = Path(tempfile.mkdtemp()) / "ffmpeg.tar.xz"
        path.write_bytes(b"not the release")
        with self.assertRaises(SystemExit):
            fetch_ffmpeg.verify(path, "0" * 64)
        self.assertFalse(path.exists())

    def test_a_matching_hash_passes(self):
        path = Path(tempfile.mkdtemp()) / "ok"
        path.write_bytes(b"x")
        fetch_ffmpeg.verify(path, fetch_ffmpeg.sha256_of(path))
        self.assertTrue(path.exists())


class DevMedia(unittest.TestCase):
    def test_the_library_covers_every_cut_a_path(self):
        paths = [item.path for item in dev_media.plan()]
        for needle in ("Direct Play Film (2020).mp4", "Remux Film (2021).mkv", "TrueHD Film",
                       "HEVC Film", " - 1080p.mkv", " - 720p.mkv", "/trailers/", "S01E03",
                       "/._", "Not A Video", "Unidentified/", ".es.srt", "tvshow.nfo"):
            self.assertTrue(any(needle in p for p in paths), needle)
        self.assertTrue(all(p.startswith(("movies/", "shows/")) for p in paths))

    def test_clip_arguments_embed_subtitles_and_cut_by_duration(self):
        clip = next(i for i in dev_media.plan() if isinstance(i, dev_media.Clip) and i.embed_ass)
        args = dev_media.ffmpeg_args(Path("/f/ffmpeg"), clip, Path("/o.mkv"), 30, Path("/s.ass"), 0)
        self.assertIn("2:s", args)
        self.assertEqual(args[args.index("-t") + 1], "30")
        self.assertNotIn("-shortest", args)  # it cut films at the last subtitle

    def test_no_two_clips_are_made_alike(self):
        # Identical settings made identical files, which the scanner rightly took for one file in
        # two places (D179).
        clips = [(index, item) for index, item in enumerate(dev_media.plan())
                 if isinstance(item, dev_media.Clip)]
        sources = set()
        for index, clip in clips:
            args = dev_media.ffmpeg_args(Path("/f"), clip, Path("/o"), 30, Path("/s"), index)
            sources.add((args[args.index("-i") + 1], args[args.index("-i", args.index("-i") + 1) + 1],
                         tuple(clip.video), tuple(clip.audio)))
        self.assertEqual(len(sources), len(clips), sources)


class DevLoop(unittest.TestCase):
    def test_config_rendering_and_listen_override(self):
        template = 'listen = "127.0.0.1:8432"\nroots = ["{root}/.dev/media"]\nffmpeg_dir = "{ffmpeg_dir}"\n'
        text = dev.render_config(template, Path("/r"), Path("/f"), "0.0.0.0:9000")
        self.assertIn('roots = ["/r/.dev/media"]', text)
        self.assertIn('ffmpeg_dir = "/f"', text)
        self.assertEqual(dev.listen_address(text), "0.0.0.0:9000")

    def test_vite_binds_the_host_dev_listen_gives(self):
        self.assertEqual(dev.web_host(None), "127.0.0.1")
        self.assertEqual(dev.web_host("0.0.0.0:8432"), "0.0.0.0")
        self.assertEqual(dev.web_host("[::]:8432"), "::")

    def test_the_banner_names_the_address_to_open_from_the_lan(self):
        lan = "192.168.1.20"
        self.assertEqual(dev.web_url(None, lan), "http://127.0.0.1:5173")
        self.assertEqual(dev.web_url("0.0.0.0:8432", lan), "http://192.168.1.20:5173")
        self.assertEqual(dev.web_url("[::]:8432", lan), "http://192.168.1.20:5173")
        self.assertEqual(dev.web_url("10.0.0.5:8432", lan), "http://10.0.0.5:5173")
        self.assertEqual(dev.web_url("[fd00::2]:8432", lan), "http://[fd00::2]:5173")

    def test_the_real_templates_leave_no_placeholder(self):
        for name in ("orpheum.dev.toml", "seed.toml"):
            template = (dev.ROOT / "deploy" / "dev" / name).read_text()
            text = dev.render_config(template, Path("/r"), Path("/f"), None)
            self.assertNotIn("{root}", text)
            self.assertNotIn("{ffmpeg_dir}", text)

    def test_snapshot_watches_go_and_dev_config_but_not_web_or_state(self):
        root = Path(tempfile.mkdtemp())
        for rel in ("internal/a.go", "go.mod", "deploy/dev/seed.toml", "dev/x.toml", "web/x.go",
                    ".dev/state/b.go", "docs/c.md"):
            (root / rel).parent.mkdir(parents=True, exist_ok=True)
            (root / rel).write_text("x")
        self.assertEqual(sorted(dev.snapshot(root)), ["/deploy/dev/seed.toml", "/go.mod", "/internal/a.go"])


if __name__ == "__main__":
    unittest.main()

#!/usr/bin/env python3
# SPDX-License-Identifier: GPL-3.0-or-later
# ABOUTME: Generates the development library: a small, synthetic set of films and episodes
# ABOUTME: that exercises every Cut A playback path, the scanner's filings and NFO identity.
"""Build `.dev/media` with the pinned FFmpeg: nothing copyrighted, nothing downloaded.

Every file exists to exercise one path Cut A must handle (cut-a.md §1, §6), and its name says
which:

* **Direct play**: MP4 with H.264 and AAC stereo, which every browser plays as it is.
* **Remux**: MKV with H.264, AC-3 5.1, an embedded ASS track and a Spanish SRT sidecar. The
  container and the audio refuse the browser; the video does not.
* **TrueHD**: MKV with TrueHD 5.1, the audio Chromecast and browsers refuse.
* **HEVC**: 10-bit HEVC, which Firefox cannot decode, so it needs `h264`.
* **Versions**: one film at 1080p and 720p, plus a trailer in `trailers/` (an extra).
* **A series**: three episodes, for Continue Watching and Next Up.
* **The scanner's other filings**: an AppleDouble `._` file (skipped), a `.mkv` that is text
  (errored), and a file with no year and no NFO (unidentified).

The 5.1 tracks carry a distinct signal per channel. The **centre** is a 1 kHz tone pulsing twice
a second, standing in for dialogue under a quieter bed, so the dialogue boost is measurable
(D83, D178 §3). The picture is FFmpeg's `testsrc`, whose running counter makes sync visible by
eye. Libraries using these files take their metadata from the NFO files written beside them,
so the dev loop needs no TMDB key.

`--seconds` sets clip length (default 30). Existing files are kept unless `--force` is given.
"""

from __future__ import annotations

import argparse
import subprocess
import sys
from dataclasses import dataclass, field
from pathlib import Path

WIDE = "aevalsrc=exprs="
FIVE_ONE = (
    "0.08*sin(2*PI*220*t)|0.08*sin(2*PI*277*t)|"  # FL FR: the bed
    "0.35*sin(2*PI*1000*t)*mod(floor(t*2)\\,2)|"  # FC: "dialogue", pulsing
    "0.05*sin(2*PI*45*t)|0.04*sin(2*PI*330*t)|0.04*sin(2*PI*392*t)"  # LFE, SL, SR
    ":c=5.1:s=48000"
)
STEREO = "sine=frequency=440:sample_rate=48000"

ASS = """[Script Info]
ScriptType: v4.00+
PlayResX: 1280
PlayResY: 720

[V4+ Styles]
Format: Name, Fontname, Fontsize, PrimaryColour, OutlineColour, BorderStyle, Outline, Alignment, MarginV
Style: Default,sans-serif,48,&H00FFFFFF,&H00000000,1,3,2,40

[Events]
Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text
Dialogue: 0,0:00:01.00,0:00:04.00,Default,,0,0,0,,The centre channel carries this line.
Dialogue: 0,0:00:05.00,0:00:08.00,Default,,0,0,0,,{\\i1}Styled text{\\i0} survives the remux.
Dialogue: 0,0:00:10.00,0:00:14.00,Default,,0,0,0,,If you can read this, libass rendered it.
"""
SRT = """1
00:00:01,000 --> 00:00:04,000
El canal central lleva esta línea.

2
00:00:10,000 --> 00:00:14,000
Si puedes leer esto, el subtítulo externo funciona.
"""


@dataclass(frozen=True)
class Clip:
    path: str
    video: list[str]
    audio_source: str
    audio: list[str]
    container: list[str] = field(default_factory=list)
    embed_ass: bool = False
    size: str = "1280x720"


@dataclass(frozen=True)
class TextFile:
    path: str
    text: str


def movie_nfo(title: str, year: int) -> str:
    return (
        '<?xml version="1.0" encoding="UTF-8"?>\n<movie>\n'
        f"  <title>{title}</title>\n  <year>{year}</year>\n"
        "  <plot>A synthetic film generated for Orpheum's development library.</plot>\n"
        "</movie>\n"
    )


def plan() -> list[Clip | TextFile]:
    h264 = ["-c:v", "libx264", "-preset", "veryfast", "-profile:v", "high", "-crf", "30",
            "-pix_fmt", "yuv420p"]
    hevc10 = ["-c:v", "libx265", "-preset", "ultrafast", "-crf", "32", "-pix_fmt", "yuv420p10le",
              "-tag:v", "hvc1", "-x265-params", "log-level=error"]
    aac2 = ["-c:a", "aac", "-ac", "2", "-b:a", "128k"]
    items: list[Clip | TextFile] = []

    def film(title: str, year: int, clip: Clip) -> None:
        items.append(clip)
        folder = f"movies/{title} ({year})"
        items.append(TextFile(f"{folder}/{title} ({year}).nfo", movie_nfo(title, year)))

    d = "movies/Direct Play Film (2020)"
    film("Direct Play Film", 2020, Clip(f"{d}/Direct Play Film (2020).mp4", h264, STEREO, aac2,
                                        ["-movflags", "+faststart"]))
    r = "movies/Remux Film (2021)"
    film("Remux Film", 2021, Clip(f"{r}/Remux Film (2021).mkv", h264, WIDE + FIVE_ONE,
                                  ["-c:a", "ac3", "-b:a", "448k"], embed_ass=True))
    items.append(TextFile(f"{r}/Remux Film (2021).es.srt", SRT))
    t = "movies/TrueHD Film (2022)"
    film("TrueHD Film", 2022, Clip(f"{t}/TrueHD Film (2022).mkv", h264, WIDE + FIVE_ONE,
                                   ["-c:a", "truehd", "-strict", "-2"]))
    e = "movies/HEVC Film (2023)"
    film("HEVC Film", 2023, Clip(f"{e}/HEVC Film (2023).mkv", hevc10, STEREO, aac2))
    v = "movies/Two Versions Film (2019)"
    film("Two Versions Film", 2019, Clip(f"{v}/Two Versions Film (2019) - 1080p.mkv", h264,
                                         WIDE + FIVE_ONE, ["-c:a", "eac3", "-b:a", "384k"],
                                         size="1920x1080"))
    items.append(Clip(f"{v}/Two Versions Film (2019) - 720p.mkv", h264, STEREO, aac2))
    items.append(Clip(f"{v}/trailers/Two Versions Film trailer.mp4", h264, STEREO, aac2,
                      ["-movflags", "+faststart"], size="640x360"))

    s = "shows/Test Show"
    items.append(TextFile(f"{s}/tvshow.nfo",
                          '<?xml version="1.0" encoding="UTF-8"?>\n<tvshow>\n'
                          "  <title>Test Show</title>\n  <year>2024</year>\n</tvshow>\n"))
    for n in (1, 2, 3):
        items.append(Clip(f"{s}/Season 01/Test Show - S01E0{n}.mkv", h264, WIDE + FIVE_ONE,
                          ["-c:a", "ac3", "-b:a", "448k"], embed_ass=(n == 1)))

    items.append(Clip("movies/Unidentified/untitled rip final v2.mkv", h264, STEREO, aac2,
                      size="640x360"))
    items.append(TextFile("movies/Not A Video (2020)/Not A Video (2020).mkv",
                          "This file has a video extension and text inside it.\n"))
    items.append(TextFile("movies/Direct Play Film (2020)/._Direct Play Film (2020).mp4",
                          "\x00\x05\x16\x07AppleDouble resource fork stand-in\n"))
    return items


def hue_of(index: int) -> int:
    """A different hue shift for each clip, so no two clips carry the same payload.

    Without it, clips with the same settings are byte-identical, and the scanner rightly sees
    them as one file in two places (D179): a duplicate the library was never meant to have.
    """
    return (index * 23) % 360


def ffmpeg_args(ffmpeg: Path, clip: Clip, out: Path, seconds: int, ass: Path, index: int) -> list[str]:
    args = [str(ffmpeg), "-hide_banner", "-loglevel", "error", "-y",
            "-f", "lavfi", "-i", f"testsrc=size={clip.size}:rate=24:duration={seconds},hue=h={hue_of(index)}",
            "-f", "lavfi", "-i", f"{clip.audio_source}:duration={seconds}"
            if clip.audio_source.startswith("sine") else f"{clip.audio_source}:d={seconds}"]
    maps = ["-map", "0:v", "-map", "1:a"]
    if clip.embed_ass:
        args += ["-i", str(ass)]
        maps += ["-map", "2:s", "-c:s", "ass", "-metadata:s:s:0", "language=eng"]
    return args + maps + clip.video + clip.audio + ["-metadata:s:a:0", "language=eng"] + \
        clip.container + ["-t", str(seconds), str(out)]


def build(ffmpeg_dir: Path, dest: Path, seconds: int, force: bool) -> None:
    ffmpeg = ffmpeg_dir / "ffmpeg"
    ass = dest / ".orpheum-dev-subtitle.ass"
    dest.mkdir(parents=True, exist_ok=True)
    ass.write_text(ASS)
    for index, item in enumerate(plan()):
        out = dest / item.path
        if out.exists() and not force:
            continue
        out.parent.mkdir(parents=True, exist_ok=True)
        if isinstance(item, TextFile):
            out.write_text(item.text)
            continue
        print(f"  generating {item.path}", file=sys.stderr)
        done = subprocess.run(ffmpeg_args(ffmpeg, item, out, seconds, ass, index),
                              capture_output=True, text=True, check=False)
        if done.returncode != 0:
            out.unlink(missing_ok=True)
            raise SystemExit(f"FFmpeg failed on {item.path}:\n{done.stderr[-2000:]}")
    ass.unlink(missing_ok=True)


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__)
    ap.add_argument("--ffmpeg-dir", required=True, type=Path)
    ap.add_argument("--dest", required=True, type=Path)
    ap.add_argument("--seconds", type=int, default=30)
    ap.add_argument("--force", action="store_true")
    args = ap.parse_args()
    build(args.ffmpeg_dir, args.dest, args.seconds, args.force)
    print(f"dev media: {args.dest}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())

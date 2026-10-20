// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { TracksNote } from "./tracks-note.tsx";

/** The Big Parade on its ladder, as nas.example offers it to Firefox: its own HEVC picture and 540p. */
const scopeFilm = {
  pictures: [
    { stream: "picture", codec: "hevc", width: 1920, height: 696, kbps: 2205, isOriginal: true },
    {
      stream: "q2000k540p",
      codec: "h264",
      width: 1490,
      height: 540,
      kbps: 2000,
      isOriginal: false,
    },
  ],
  sounds: [
    {
      audioTrackIndex: 0,
      isBoosted: false,
      codec: "aac",
      channelCount: 2,
      kbps: 96,
      isOriginal: false,
    },
    {
      audioTrackIndex: 0,
      isBoosted: true,
      codec: "aac",
      channelCount: 2,
      kbps: 96,
      isOriginal: false,
    },
  ],
  filePicture: { codec: "hevc", width: 1920, height: 696 },
  fileSounds: [{ audioTrackIndex: 0, codec: "eac3", channelCount: 6 }],
};

describe("the tracks a film plays (the owner, 2026-10-20)", () => {
  it("names the picture at the level playing and the sound chosen, each original or transcoded and from what", () => {
    const original = renderToStaticMarkup(
      <TracksNote
        tracks={scopeFilm}
        height={696}
        sound={{ audioTrackIndex: 0, isBoosted: true }}
      />,
    );
    expect(original).toContain("Video: HEVC 1920×696, 2.2 Mb/s (original)");
    expect(original).toContain(
      "Audio: AAC stereo, 96 kb/s (transcoded from E-AC-3 5.1, dialogue boost)",
    );
    const lower = renderToStaticMarkup(
      <TracksNote
        tracks={scopeFilm}
        height={540}
        sound={{ audioTrackIndex: 0, isBoosted: false }}
      />,
    );
    expect(lower).toContain("Video: H.264 1490×540, 2.0 Mb/s (transcoded from HEVC 1920×696)");
    expect(lower).toContain("Audio: AAC stereo, 96 kb/s (transcoded from E-AC-3 5.1)");
  });

  it("names a file played as it is original throughout, and says nothing without tracks", () => {
    const direct = renderToStaticMarkup(
      <TracksNote
        tracks={{
          pictures: [{ codec: "h264", width: 1280, height: 720, kbps: 0, isOriginal: true }],
          sounds: [
            {
              audioTrackIndex: 0,
              isBoosted: false,
              codec: "aac",
              channelCount: 2,
              kbps: 0,
              isOriginal: true,
            },
          ],
          filePicture: { codec: "h264", width: 1280, height: 720 },
          fileSounds: [{ audioTrackIndex: 0, codec: "aac", channelCount: 2 }],
        }}
        sound={{ audioTrackIndex: 0, isBoosted: false }}
      />,
    );
    expect(direct).toContain("Video: H.264 1280×720 (original)");
    expect(direct).toContain("Audio: AAC stereo (original)");
    expect(
      renderToStaticMarkup(
        <TracksNote tracks={undefined} sound={{ audioTrackIndex: 0, isBoosted: false }} />,
      ),
    ).toBe("");
  });
});

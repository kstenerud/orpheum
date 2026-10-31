// SPDX-License-Identifier: MPL-2.0

/**
 * The tracks note as a real browser lays it out (vitest's browser mode): play.css alone decides it,
 * a quiet list under its heading like the reason line beside it.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx), and the player's rules.
import { unmeasuredSound } from "./test-fixtures.ts";
import "./tokens.css";
import "./base.css";
import "./play.css";
import "./i18n.ts";

import { describe, expect, it } from "vitest";

import { mount } from "./test-dom.tsx";
import { TracksNote } from "./tracks-note.tsx";

const filmAsItIs = {
  pictures: [
    { stream: "", codec: "h264", width: 1920, height: 1036, kbps: 6000, isOriginal: true },
  ],
  sounds: [
    {
      audioTrackIndex: 0,
      isBoosted: false,
      codec: "aac",
      channelCount: 2,
      kbps: 0,
      isOriginal: true,
      ...unmeasuredSound,
    },
  ],
  filePicture: { codec: "h264", width: 1920, height: 1036 },
  fileSounds: [{ audioTrackIndex: 0, codec: "aac", channelCount: 2 }],
};

describe("the tracks note, laid out by a real browser", () => {
  it("lists its tracks muted, unbulleted and close under their heading", async () => {
    const mounted = await mount(
      <TracksNote tracks={filmAsItIs} sound={{ audioTrackIndex: 0, isBoosted: false }} />,
    );
    const note = mounted.container.querySelector(".play__tracks");
    const heading = mounted.container.querySelector(".play__tracks p");
    const track = mounted.container.querySelector(".play__tracks li");
    if (note === null || heading === null || track === null) {
      throw new Error("the tracks note is drawn nowhere");
    }
    expect(getComputedStyle(note).color).toBe("rgb(163, 163, 173)");
    expect(getComputedStyle(heading).marginBlockEnd).toBe("0px");
    expect(getComputedStyle(track).listStyleType).toBe("none");
    mounted.unmount();
  });
});

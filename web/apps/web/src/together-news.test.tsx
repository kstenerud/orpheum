// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { startingTuning, type TogetherSnapshot } from "@orpheum/sdk";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { TogetherNews } from "./together-news.tsx";

const snapshot: TogetherSnapshot = {
  status: "connected",
  memberId: 1,
  isHost: true,
  group: undefined,
  groupReceivedAtMs: 0,
  roster: [
    {
      id: 1,
      name: "Karl",
      isHost: true,
      devices: [{ id: 3, role: "playback", isBuffering: false, cannotKeepUp: undefined }],
    },
    {
      id: 2,
      name: "Kim",
      isHost: false,
      devices: [
        {
          id: 4,
          role: "playback",
          isBuffering: true,
          cannotKeepUp: { linkKbps: 600, lowestKbps: 750 },
        },
      ],
    },
  ],
  endReason: undefined,
  minimumRttMs: undefined,
  lastRejectionCode: undefined,
  tuning: startingTuning,
};

describe("what everyone in a session is told", () => {
  it("names who cannot keep up even at the lowest quality, with both numbers (D247 §8)", () => {
    const shown = renderToStaticMarkup(<TogetherNews snapshot={snapshot} />);
    expect(shown).toContain(
      "Kim&#x27;s connection carries about 0.6 Mb/s, and even the lowest quality needs 0.8 Mb/s",
    );
    expect(shown).not.toContain("Karl&#x27;s connection");
  });
});

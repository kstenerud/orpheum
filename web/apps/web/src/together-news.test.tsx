// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { everyRight, startingTuning, type TogetherSnapshot } from "@orpheum/sdk";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { TogetherNews } from "./together-news.tsx";

const snapshot: TogetherSnapshot = {
  status: "connected",
  memberId: 1,
  isHost: true,
  rights: everyRight,
  group: undefined,
  groupReceivedAtMs: 0,
  groupPlacedByRttMs: undefined,
  roster: [
    {
      id: 1,
      name: "Karl",
      isHost: true,
      devices: [
        {
          id: 3,
          role: "playback",
          isBuffering: false,
          cannotKeepUp: undefined,
          status: "watching",
        },
      ],
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
          status: "buffering",
        },
      ],
    },
  ],
  endReason: undefined,
  minimumRttMs: undefined,
  lastRejectionCode: undefined,
  answersHeard: 0,
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

  it("says where each person's player stands, a line each (the owner's session, 2026-10-18)", () => {
    const shown = renderToStaticMarkup(<TogetherNews snapshot={snapshot} />);
    expect(shown).toContain("<li>Karl: watching</li>");
    expect(shown).toContain("<li>Kim: waiting for the film to load</li>");
    const tapping = {
      ...snapshot,
      roster: snapshot.roster.map((member) => ({
        ...member,
        devices: member.devices.map((device) => ({
          ...device,
          status: member.isHost ? undefined : ("needsTap" as const),
        })),
      })),
    };
    const shownTapping = renderToStaticMarkup(<TogetherNews snapshot={tapping} />);
    expect(shownTapping).toContain("<li>Kim: needs to tap the page to start</li>");
    expect(shownTapping).not.toContain("Karl:");
  });
});

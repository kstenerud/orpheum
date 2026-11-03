// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { everyRight, startingTuning, type TogetherSnapshot } from "@orpheum/sdk";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { SessionRemote } from "./session-remote.tsx";

const connected: TogetherSnapshot = {
  status: "connected",
  memberId: 1,
  isHost: true,
  rights: everyRight,
  group: undefined,
  groupReceivedAtMs: 0,
  groupPlacedByRttMs: undefined,
  roster: [],
  endReason: undefined,
  minimumRttMs: undefined,
  lastRejectionCode: undefined,
  answersHeard: 0,
  tuning: startingTuning,
};

/** What the remote's state line says over snapshot. */
function stateLine(snapshot: TogetherSnapshot): string {
  const shown = renderToStaticMarkup(
    <SessionRemote snapshot={snapshot} connection={undefined} durationMs={0} />,
  );
  return /<output>(.*?)<\/output>/u.exec(shown)?.[1] ?? "";
}

describe("a session's remote", () => {
  it("says why its buttons wait while the session connects or reconnects", () => {
    expect(stateLine({ ...connected, status: "connecting" })).toBe("Connecting…");
    expect(stateLine({ ...connected, status: "reconnecting" })).toBe(
      "Connection lost. Reconnecting…",
    );
    expect(stateLine(connected)).toBe("Starting…");
  });
});

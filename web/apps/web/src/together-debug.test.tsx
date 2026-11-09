// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { everyRight, startingTuning, type RosterDevice, type TogetherSnapshot } from "@orpheum/sdk";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { DebugToolsContext } from "./debug-tools.tsx";
import { TogetherDebug } from "./together-debug.tsx";

/** A video device of the roster, leading or not, and why when it leads. */
function device(id: number, leadsBecause?: string): RosterDevice {
  return {
    id,
    role: "video",
    isBuffering: false,
    cannotKeepUp: undefined,
    status: "watching",
    ...(leadsBecause === undefined ? {} : { isLeader: true, leadsBecause }),
  };
}

/** A playing session seen from device 4, the S23's, with the device leading given and why. */
function sessionLedBy(leaderId: number, why: string): TogetherSnapshot {
  const of = (id: number): RosterDevice => device(id, id === leaderId ? why : undefined);
  return {
    status: "connected",
    memberId: 2,
    deviceId: 4,
    isHost: false,
    rights: everyRight,
    group: {
      state: "playing",
      positionMs: 60_000,
      rate: 1,
      fileId: 7,
      event: undefined,
      seekedFromMs: 0,
      byMemberId: undefined,
      isHeldPlaying: false,
    },
    groupReceivedAtMs: 0,
    groupPlacedByRttMs: undefined,
    roster: [
      { id: 1, name: "Karl", isHost: true, devices: [of(3)] },
      { id: 2, name: "Kim", isHost: false, devices: [of(4), of(5)] },
    ],
    endReason: undefined,
    minimumRttMs: undefined,
    lastRejectionCode: undefined,
    answersHeard: 0,
    tuning: startingTuning,
  };
}

/** The debug view's lines of snapshot, its person's debug tools shown or not. */
function linesOf(snapshot: TogetherSnapshot, showsDebugTools: boolean): string[] {
  const host = document.createElement("div");
  host.innerHTML = renderToStaticMarkup(
    <DebugToolsContext value={{ isShown: showsDebugTools, onChanged: () => undefined }}>
      <TogetherDebug snapshot={snapshot} />
    </DebugToolsContext>,
  );
  return [...host.querySelectorAll("li, p")].map((line) => line.textContent);
}

describe("Watch Together's leader debug view (the owner, 2026-10-29)", () => {
  it("marks the device leading and this device, and the marks move with the lead", () => {
    expect(linesOf(sessionLedBy(3, "putOn"), true)).toEqual([
      "Karl: device 3 (video) · ▶ leading",
      "Kim: device 4 (video) · this device",
      "Kim: device 5 (video)",
      "playing · leader device 3 (Karl) · why putOn · this device 4",
    ]);
    expect(linesOf(sessionLedBy(4, "leaderGone:hidden"), true)).toEqual([
      "Karl: device 3 (video)",
      "Kim: device 4 (video) · ▶ leading · this device",
      "Kim: device 5 (video)",
      "playing · leader device 4 (Kim) · why leaderGone:hidden · this device 4",
    ]);
  });

  it("says when no device leads", () => {
    expect(linesOf(sessionLedBy(0, ""), true).at(-1)).toBe(
      "playing · leader none · why none · this device 4",
    );
  });

  it("is hidden while the person's Show debug tools is off", () => {
    expect(linesOf(sessionLedBy(3, "putOn"), false)).toEqual([]);
  });
});

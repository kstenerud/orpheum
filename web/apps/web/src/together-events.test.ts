// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { parseTogetherMessage, type GroupEvent, type GroupView } from "@orpheum/sdk";
import { describe, expect, it } from "vitest";

import { i18n } from "./i18n.ts";
import { filmPositionText, groupEventText } from "./together-events.ts";

const roster = [{ id: 1, name: "Karl", isHost: true, devices: [] }];

function group(event: GroupEvent | undefined, byMemberId?: number, seekedFromMs = 0): GroupView {
  return {
    state: "Paused",
    positionMs: 2_472_000,
    rate: 1,
    fileId: 5,
    event,
    byMemberId,
    seekedFromMs,
  };
}

describe("the group's changes as named events (D188)", () => {
  it("names who did it, and where, in the viewer's words", () => {
    expect(groupEventText(i18n.t, group("paused", 1), roster)).toBe("Karl paused at 41:12");
    expect(groupEventText(i18n.t, group("seeked", 1, 723_000), roster)).toBe(
      "Karl jumped from 12:03 to 41:12",
    );
    expect(groupEventText(i18n.t, group("leaderLeft", 2), roster)).toBe(
      "Someone's screen left; playing on from 41:12",
    );
    expect(groupEventText(i18n.t, group("leaderPlays", 1), roster)).toBe(
      "Karl's screen plays from 41:12",
    );
  });

  it("says whose screen, the session's leader, the group waits for (D371 §1)", () => {
    const parsed = parseTogetherMessage(
      JSON.stringify({
        type: "group",
        group: {
          state: "Waiting",
          positionMs: 2_472_000,
          rate: 1,
          fileId: 7,
          event: "waitingFor",
          byMemberId: 1,
        },
      }),
    );
    const waiting = parsed?.type === "group" ? parsed.group : undefined;
    expect(waiting && groupEventText(i18n.t, waiting, roster)).toBe(
      "Waiting for Karl's player at 41:12",
    );
  });

  it("says nothing of an event it does not know, or of none", () => {
    const unknown = parseTogetherMessage(
      JSON.stringify({ type: "group", group: { state: "Paused", event: "danced", byMemberId: 1 } }),
    );
    const unknownGroup = unknown?.type === "group" ? unknown.group : undefined;
    expect(unknownGroup?.event).toBeUndefined();
    expect(
      unknownGroup === undefined ? "none" : groupEventText(i18n.t, unknownGroup, roster),
    ).toBeUndefined();
    expect(groupEventText(i18n.t, group(undefined), roster)).toBeUndefined();
  });

  it("reads a place past the first hour with its hours", () => {
    expect(filmPositionText(3_723_000, "en")).toBe("1:02:03");
    expect(filmPositionText(5000, "en")).toBe("0:05");
  });
});

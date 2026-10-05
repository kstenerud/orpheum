// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { GroupView } from "@orpheum/sdk";
import { describe, expect, it } from "vitest";

import { i18n } from "./i18n.ts";
import { filmPositionText, groupEventText } from "./together-events.ts";

const roster = [{ id: 1, name: "Karl", isHost: true, devices: [] }];

function group(event: string | undefined, byMemberId?: number): GroupView {
  return { state: "Paused", positionMs: 2_472_000, rate: 1, fileId: 5, event, byMemberId };
}

describe("the group's changes as named events (D188)", () => {
  it("names who did it, and where, in the viewer's words", () => {
    expect(groupEventText(i18n.t, group("paused", 1), roster)).toBe("Karl paused at 41:12");
    expect(groupEventText(i18n.t, group("seeked from 723000", 1), roster)).toBe(
      "Karl jumped from 12:03 to 41:12",
    );
    expect(groupEventText(i18n.t, group("buffering", 2), roster)).toBe(
      "Someone is buffering… everyone waits",
    );
  });

  it("says nothing of an event it does not know, or of none", () => {
    expect(groupEventText(i18n.t, group("danced", 1), roster)).toBeUndefined();
    expect(groupEventText(i18n.t, group(undefined), roster)).toBeUndefined();
  });

  it("reads a place past the first hour with its hours", () => {
    expect(filmPositionText(3_723_000, "en")).toBe("1:02:03");
    expect(filmPositionText(5000, "en")).toBe("0:05");
  });
});

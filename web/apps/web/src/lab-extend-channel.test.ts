// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  extendMessageOf,
  extendSeekS,
  positionAfter,
  screensOf,
  windowFeaturesOn,
} from "./lab-extend-channel.ts";

describe("the extend probe's windows", () => {
  it("read each other's commands and states, and nothing else", () => {
    expect(extendMessageOf({ kind: "command", command: "toggle" })).toEqual({
      kind: "command",
      command: "toggle",
    });
    expect(
      extendMessageOf({ kind: "state", positionS: 12.5, isPlaying: true, isFullscreen: false }),
    ).toEqual({ kind: "state", positionS: 12.5, isPlaying: true, isFullscreen: false });
    expect(extendMessageOf({ kind: "command", command: "eject" })).toBeUndefined();
    expect(extendMessageOf({ kind: "state", positionS: "12" })).toBeUndefined();
    expect(extendMessageOf("toggle")).toBeUndefined();
  });

  it("move the player back and forward by ten seconds, never before the start", () => {
    expect(positionAfter("forward", 3)).toBe(3 + extendSeekS);
    expect(positionAfter("back", 25)).toBe(25 - extendSeekS);
    expect(positionAfter("back", 4)).toBe(0);
    expect(positionAfter("toggle", 4)).toBe(4);
  });

  it("place the player over a screen's whole available area, named or numbered", () => {
    const screens = screensOf({
      screens: [
        {
          label: "Built-in",
          availLeft: 0,
          availTop: 25,
          availWidth: 1440,
          availHeight: 875,
          isPrimary: true,
          isInternal: true,
        },
        { label: "", availLeft: 1440, availTop: 0, availWidth: 1920, availHeight: 1080 },
        "not a screen",
      ],
    });
    expect(screens).toEqual([
      {
        label: "Built-in",
        left: 0,
        top: 25,
        width: 1440,
        height: 875,
        isPrimary: true,
        isInternal: true,
      },
      {
        label: "#2",
        left: 1440,
        top: 0,
        width: 1920,
        height: 1080,
        isPrimary: false,
        isInternal: false,
      },
    ]);
    expect(windowFeaturesOn(screens[1] ?? screens[0]!)).toBe(
      "left=1440,top=0,width=1920,height=1080",
    );
    expect(screensOf(undefined)).toEqual([]);
  });
});

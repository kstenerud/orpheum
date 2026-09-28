// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { createRef } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { addedGainDb, LevelSwitch, levelNote, soundLevel, type SoundLevel } from "./levelling.tsx";
import type { Decision } from "./play.tsx";
import { click, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const alien: SoundLevel = { levelling: { isMeasured: true, gainDb: 12.7 }, carriedGainDb: 0 };

describe("levelling a film's loudness (D226)", () => {
  it("adds the gain after the volume, and takes out what a copy already carries", () => {
    expect(addedGainDb(alien, true)).toBe(12.7);
    expect(addedGainDb(alien, false)).toBe(0);
    const prepared = { ...alien, carriedGainDb: 12.7 };
    expect(addedGainDb(prepared, true)).toBe(0);
    expect(addedGainDb(prepared, false)).toBe(-12.7);
    const unmeasured = { levelling: { isMeasured: false, gainDb: 0 }, carriedGainDb: 0 };
    expect(addedGainDb(unmeasured, true)).toBe(0);
  });

  it("levels a prepared copy by its first mix, and the original by its main track", () => {
    const direct = {
      kind: "direct",
      levelling: { isMeasured: true, gainDb: -4.2 },
    } as unknown as Decision;
    expect(soundLevel(direct)).toEqual({
      levelling: { isMeasured: true, gainDb: -4.2 },
      carriedGainDb: 0,
    });
    const hls = {
      kind: "hls",
      levelling: { isMeasured: true, gainDb: 1 },
      audio: [{ levelling: { isMeasured: true, gainDb: 5 }, carriedGainDb: 5 }],
    } as unknown as Decision;
    expect(soundLevel(hls)).toEqual({
      levelling: { isMeasured: true, gainDb: 5 },
      carriedGainDb: 5,
    });
  });

  it("says what was done: raised, lowered, as mixed, not measured, or what a device's own volume allows", () => {
    expect(levelNote(alien, true, true)).toEqual({ key: "raised", db: 12.7 });
    expect(levelNote(alien, false, true)).toEqual({ key: "asMixed", db: 0 });
    const loud = { levelling: { isMeasured: true, gainDb: -4.2 }, carriedGainDb: 0 };
    expect(levelNote(loud, true, true)).toEqual({ key: "lowered", db: 4.2 });
    const unmeasured = { levelling: { isMeasured: false, gainDb: 0 }, carriedGainDb: 0 };
    expect(levelNote(unmeasured, true, true)).toEqual({ key: "notMeasured", db: 0 });
    expect(levelNote(alien, true, false)).toEqual({ key: "deviceVolume", db: 0 });
    expect(levelNote({ ...alien, carriedGainDb: 12.7 }, false, false)).toEqual({
      key: "raised",
      db: 12.7,
    });
  });

  it("starts as the person left it, says the gain, and switching it off is kept for every file", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "PUT /me/prefs": [204, undefined] }, heard);
    mounted = await mount(
      withServices(<LevelSwitch level={alien} isOn videoRef={createRef()} />, services),
    );
    const box = mounted.container.querySelector("input");
    expect(box?.checked).toBe(true);
    expect(mounted.container.textContent).toContain("Raised 12.7 dB, to the level of other films.");
    await click(box);
    expect(box?.checked).toBe(false);
    expect(mounted.container.textContent).toContain("Playing as it was mixed.");
    expect(heard).toEqual([{ asked: "PUT /me/prefs", body: { isLevelling: false } }]);
  });
});

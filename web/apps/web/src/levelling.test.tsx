// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { createRef } from "react";
import { afterEach, describe, expect, it } from "vitest";

import {
  addedGainDb,
  LevelSwitch,
  levelNote,
  measuringNote,
  soundLevelOf,
  type SoundLevel,
} from "./levelling.tsx";
import { click, mount, servedBy, settleUntil, type Heard, type Mounted } from "./test-dom.tsx";
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

  it("levels a prepared copy by the playing track's mix, and the original by its main track", () => {
    const direct: Parameters<typeof soundLevelOf>[0] = {
      delivery: "direct",
      levelling: { isMeasured: true, gainDb: -4.2 },
    };
    expect(soundLevelOf(direct, undefined)).toEqual({
      levelling: { isMeasured: true, gainDb: -4.2 },
      carriedGainDb: 0,
    });
    const mix = (audioTrackIndex: number, gainDb: number, isDialogueBoosted = false) => ({
      audioTrackIndex,
      language: "en",
      isDialogueBoosted,
      levelling: { isMeasured: true, gainDb },
      carriedGainDb: gainDb,
    });
    const hls: Parameters<typeof soundLevelOf>[0] = {
      delivery: "hls",
      levelling: { isMeasured: true, gainDb: 1 },
      audioRenditions: [mix(0, 5), mix(0, 9, true), mix(1, 3)],
    };
    expect(soundLevelOf(hls, 0)).toEqual({
      levelling: { isMeasured: true, gainDb: 5 },
      carriedGainDb: 5,
    });
    // Another track chosen is levelled by its own mix (D230 §5).
    expect(soundLevelOf(hls, 1)).toEqual({
      levelling: { isMeasured: true, gainDb: 3 },
      carriedGainDb: 3,
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

  it("says how long a sound not measured yet will take, as the server says of its read-through", () => {
    const unmeasured = { levelling: { isMeasured: false, gainDb: 0 }, carriedGainDb: 0 };
    const note = levelNote(unmeasured, true, true);
    expect(measuringNote(note, { isRunning: true, secondsLeft: 180 })).toEqual({
      key: "measuring",
      db: 0,
      seconds: 180,
    });
    expect(measuringNote(note, { isRunning: false, secondsLeft: 720 })).toEqual({
      key: "waitingToMeasure",
      db: 0,
      seconds: 720,
    });
    expect(measuringNote(note, { isRunning: false })).toEqual({ key: "notMeasured", db: 0 });
    const raised = levelNote(alien, true, true);
    expect(measuringNote(raised, { isRunning: true, secondsLeft: 180 })).toEqual(raised);
  });

  it("asks the server how long the measuring will take, and says so", async () => {
    const unmeasured = { levelling: { isMeasured: false, gainDb: 0 }, carriedGainDb: 0 };
    const services = servedBy({
      "GET /files/1/waits": [200, { readThrough: { isRunning: false, secondsLeft: 720 } }],
    });
    mounted = await mount(
      withServices(
        <LevelSwitch fileId={1} level={unmeasured} isInitiallyOn videoRef={createRef()} />,
        services,
      ),
    );
    const said =
      "Its loudness is waiting to be measured, about 12 minutes; until then it plays as it is.";
    await settleUntil(() => mounted?.container.textContent.includes(said) === true);
    expect(mounted.container.textContent).toContain(said);
  });

  it("starts as the person left it, says the gain, and switching it off is kept for every file", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "PUT /me/preferences": [204, undefined] }, heard);
    mounted = await mount(
      withServices(
        <LevelSwitch fileId={1} level={alien} isInitiallyOn videoRef={createRef()} />,
        services,
      ),
    );
    const box = mounted.container.querySelector("input");
    expect(box?.checked).toBe(true);
    expect(mounted.container.textContent).toContain("Raised 12.7 dB, to the level of other films.");
    await click(box);
    expect(box?.checked).toBe(false);
    expect(mounted.container.textContent).toContain("Playing as it was mixed.");
    expect(heard).toEqual([
      { methodAndPath: "PUT /me/preferences", body: { isLevellingOn: false } },
    ]);
  });
});

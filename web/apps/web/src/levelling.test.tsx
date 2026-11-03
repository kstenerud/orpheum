// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { createRef } from "react";
import { afterEach, describe, expect, it } from "vitest";

import {
  addedGainDb,
  LevelSwitch,
  levelNote,
  measuredLevel,
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

const arkon: SoundLevel = { levelling: { isMeasured: true, gainDb: 12.7 }, carriedGainDb: 0 };

describe("levelling a film's loudness (D226)", () => {
  it("adds the gain after the volume, and takes out what a copy already carries", () => {
    expect(addedGainDb(arkon, true)).toBe(12.7);
    expect(addedGainDb(arkon, false)).toBe(0);
    const prepared = { ...arkon, carriedGainDb: 12.7 };
    expect(addedGainDb(prepared, true)).toBe(0);
    expect(addedGainDb(prepared, false)).toBe(-12.7);
    const unmeasured = { levelling: { isMeasured: false, gainDb: 0 }, carriedGainDb: 0 };
    expect(addedGainDb(unmeasured, true)).toBe(0);
  });

  it("levels the original by its main track, which carries no gain of its own", () => {
    expect(soundLevelOf({ levelling: { isMeasured: true, gainDb: -4.2 } })).toEqual({
      levelling: { isMeasured: true, gainDb: -4.2 },
      carriedGainDb: 0,
    });
  });

  it("says what was done: raised, lowered, as mixed, not measured, or what a device's own volume allows", () => {
    expect(levelNote(arkon, true, true)).toEqual({ key: "raised", db: 12.7 });
    expect(levelNote(arkon, false, true)).toEqual({ key: "asMixed", db: 0 });
    const loud = { levelling: { isMeasured: true, gainDb: -4.2 }, carriedGainDb: 0 };
    expect(levelNote(loud, true, true)).toEqual({ key: "lowered", db: 4.2 });
    const unmeasured = { levelling: { isMeasured: false, gainDb: 0 }, carriedGainDb: 0 };
    expect(levelNote(unmeasured, true, true)).toEqual({ key: "notMeasured", db: 0 });
    expect(levelNote(arkon, true, false)).toEqual({ key: "deviceVolume", db: 0 });
    expect(levelNote({ ...arkon, carriedGainDb: 12.7 }, false, false)).toEqual({
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
    const raised = levelNote(arkon, true, true);
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

  it("takes up a level measured as the film plays, for the track playing, and only for a sound not levelled", () => {
    const unmeasured = { levelling: { isMeasured: false, gainDb: 0 }, carriedGainDb: 0 };
    const levellings = [
      { isMeasured: true, gainDb: -4.2 },
      { isMeasured: true, gainDb: 3.1 },
    ];
    expect(measuredLevel(unmeasured, levellings, 1)).toEqual({
      levelling: { isMeasured: true, gainDb: 3.1 },
      carriedGainDb: 0,
    });
    expect(measuredLevel(unmeasured, undefined, 1)).toEqual(unmeasured);
    expect(measuredLevel(unmeasured, [{ isMeasured: false, gainDb: 0 }], 0)).toEqual(unmeasured);
    expect(measuredLevel(unmeasured, levellings, undefined)).toEqual(unmeasured);
    expect(measuredLevel(arkon, levellings, 0)).toEqual(arkon);
    const carried = { levelling: { isMeasured: true, gainDb: 2 }, carriedGainDb: 2 };
    expect(measuredLevel(carried, levellings, 0)).toEqual(carried);
  });

  it("levels a film measured while it plays without its playing again (VQ50)", async () => {
    const unmeasured = { levelling: { isMeasured: false, gainDb: 0 }, carriedGainDb: 0 };
    const services = servedBy({
      "GET /files/1/waits": [200, { levellings: [{ isMeasured: true, gainDb: -4.2 }] }],
    });
    mounted = await mount(
      withServices(
        <LevelSwitch
          fileId={1}
          level={unmeasured}
          audioTrackIndex={0}
          isInitiallyOn
          videoRef={createRef()}
        />,
        services,
      ),
    );
    const said = "Lowered 4.2 dB, to the level of other films.";
    await settleUntil(() => mounted?.container.textContent.includes(said) === true);
    expect(mounted.container.textContent).toContain(said);
  });

  it("starts as the person left it, says the gain, and switching it off is kept for every file", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "PUT /me/preferences": [204, undefined] }, heard);
    mounted = await mount(
      withServices(
        <LevelSwitch fileId={1} level={arkon} isInitiallyOn videoRef={createRef()} />,
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

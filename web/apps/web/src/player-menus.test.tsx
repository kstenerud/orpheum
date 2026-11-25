// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef, type RefObject } from "react";
import { afterEach, describe, expect, it } from "vitest";

import type { PlayerSound } from "./playback-target.ts";
import { PlayerMenus, PlayerMenusContext } from "./player-menus.tsx";
import { PlayerTracksContext, type PlayerTracks } from "./player-tracks.tsx";
import {
  bodyField,
  buttonWithText,
  click,
  enter,
  fire,
  mount,
  servedBy,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";

import { withServices } from "./test-services.tsx";
import {
  type Chosen,
  tracksNoting,
  ways,
  everyMenu,
  drawnMenus,
  opened,
  places,
  openedSubmenu,
  choosing,
  offered,
} from "./test-player-menus.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** The overlay's menus over tracks, the drawing this file's afterEach unmounts. */
async function drawn(tracks: PlayerTracks, told: boolean[] = []): Promise<Mounted> {
  mounted = await drawnMenus(tracks, told);
  return mounted;
}

describe("the overlay's menus (D407 §8)", () => {
  it("reports a problem from ⁝ with where the film stands, taken as it was chosen (D524 §2)", async () => {
    const heard: Heard[] = [];
    const video = document.createElement("video");
    video.currentTime = 42;
    const videoRef: RefObject<HTMLVideoElement | null> = { current: video };
    mounted = await mount(
      withServices(
        <PlayerTracksContext value={tracksNoting({ subtitles: [], audio: [] })}>
          <PlayerMenusContext value={ways}>
            <PlayerMenus videoRef={videoRef} shown={everyMenu} onOpenChange={() => undefined} />
          </PlayerMenusContext>
        </PlayerTracksContext>,
        servedBy({ "POST /problem-reports": [201, { id: 1 }] }, heard),
      ),
    );
    await opened(mounted, "Settings");
    await choosing(mounted, "Report a problem");
    video.currentTime = 50;
    const dialog = mounted.container.querySelector('dialog[aria-label="Report a problem"]');
    await enter(dialog?.querySelector("textarea"), "the picture froze");
    await click(buttonWithText(mounted.container, "Send"));
    const sent = heard.find((each) => each.methodAndPath === "POST /problem-reports");
    expect(bodyField(sent!, "playback")).toEqual({
      fileId: 3,
      positionMs: 42_000,
      quality: "",
      state: "paused",
    });
  });

  // The menu spec of D538 (the owner, 2026-11-23): the tracks a submenu named by the one current,
  // the subtitles' timing under it, Find subtitles last.
  it("lists Off and the subtitle tracks in Subtitles' submenu, then their timing and Find subtitles, no audio track, and chooses one", async () => {
    const chosen: Chosen = { subtitles: [], audio: [] };
    const on = await drawn(tracksNoting(chosen));
    await opened(on, "Subtitles");
    const shown = places(on);
    expect(shown).toHaveLength(3);
    expect(shown[0]).toBe("Subtitles: Off›");
    expect(shown[1]).toContain("Subtitle timing");
    expect(shown[2]).toBe("Find subtitles…");
    await openedSubmenu(on, "Subtitles: Off");
    expect(places(on)).toEqual(["‹Subtitles", "Off", "English", "French"]);
    await choosing(on, "French");
    expect(chosen.subtitles).toEqual(["s7"]);
    expect(on.container.querySelector("dialog")).toBeNull();
  });

  it("lists the audio tracks in the Sound menu's Sound track, and chooses one (D538 §8)", async () => {
    const chosen: Chosen = { subtitles: [], audio: [] };
    const on = await drawn(tracksNoting(chosen));
    await opened(on, "Sound");
    await openedSubmenu(on, "Sound track");
    expect(places(on)).toEqual(["‹Sound track", "English", "French"]);
    await choosing(on, "French");
    expect(chosen.audio).toEqual([81]);
  });

  it("holds the sound in the Sound menu: its track, levelling with the gain it adds, the boost and the timing (D538 §8)", async () => {
    const tracks = tracksNoting({ subtitles: [], audio: [] });
    const boost = { isOn: false, switchTo: () => undefined, problem: undefined };
    const on = await drawn({ ...tracks, boost });
    await opened(on, "Sound");
    const shown = places(on);
    expect(shown[0]).toContain("Sound track");
    expect(shown[0]).toContain("English");
    expect(shown.slice(1, 4)).toEqual([
      "Level loudness (-2.0 dB)",
      "Dialogue boost",
      "Sound timing",
    ]);
    expect(shown[4]).toContain("This device");
  });

  it("says nothing in Level loudness's brackets while the sound is unmeasured", async () => {
    const tracks = tracksNoting({ subtitles: [], audio: [] });
    const level = { levelling: { isMeasured: false, gainDb: 0 }, carriedGainDb: 0 };
    const on = await drawn({ ...tracks, level });
    await opened(on, "Sound");
    expect(places(on)).toContain("Level loudness");
  });

  // The owner, 2026-11-23: the subtitles' timing moved from ⁝ to Subtitles.
  it("holds the rest under ⁝: Start over, and no sound setting nor the subtitles' timing (D538 §8)", async () => {
    const on = await drawn(tracksNoting({ subtitles: [], audio: [] }));
    await opened(on, "Settings");
    const shown = places(on).join(" | ");
    expect(shown).toContain("Start over");
    expect(shown).not.toContain("Subtitle timing");
    expect(on.container.querySelector("dialog .stepper")).toBeNull();
    expect(shown).not.toContain("Level");
    expect(shown).not.toContain("This device");
  });

  it("says under ⁝ how the film plays here, read-only: as it is or converted, quality and link (D444)", async () => {
    mounted = await mount(
      withServices(
        <PlayerTracksContext value={tracksNoting({ subtitles: [], audio: [] })}>
          <PlayerMenusContext
            value={{ ...ways, playback: { delivery: "converted", qualityKbps: 4200 } }}
          >
            <PlayerMenus videoRef={createRef()} shown={everyMenu} onOpenChange={() => undefined} />
          </PlayerMenusContext>
        </PlayerTracksContext>,
      ),
    );
    await opened(mounted, "Settings");
    expect(places(mounted)).toContain("Playing: Converted 4.2 Mb/s");
  });

  it("tells the overlay a menu is open, and Find subtitles' dialog after it, until it closes", async () => {
    const told: boolean[] = [];
    const on = await drawn(tracksNoting({ subtitles: [], audio: [] }), told);
    await opened(on, "Subtitles");
    await choosing(on, "Find subtitles…");
    expect(on.container.querySelector("dialog .subtitle-search")).not.toBeNull();
    expect(told.at(-1)).toBe(true);
    act(() => {
      document.dispatchEvent(new Event("fullscreenchange"));
    });
    expect(on.container.querySelector("dialog")).toBeNull();
    expect(told).toEqual([false, true, false]);
  });

  it("names Sound track by the first audio track where none is chosen, as the playback plays it", async () => {
    const tracks = tracksNoting({ subtitles: [], audio: [] });
    const on = await drawn({ ...tracks, audio: { ...tracks.audio, chosenStreamId: undefined } });
    await opened(on, "Sound");
    expect(places(on)[0]).toBe("Sound track: English›");
  });

  // The owner, 2026-11-04: Sound track and Subtitles are two menus, not one panel; 2026-11-23: the
  // button is named Subtitles, its choice named in the menu, as Sound's is.
  it("names Subtitles by itself, its choice in its menu, a button apart from the Sound menu's", async () => {
    const on = await drawn(tracksNoting({ subtitles: [], audio: [] }));
    const names = [...document.querySelectorAll("[aria-haspopup]")].map((button) =>
      button.getAttribute("aria-label"),
    );
    expect(names).toEqual(["Subtitles", "Sound", "Settings"]);
    await opened(on, "Subtitles");
    expect(places(on)[0]).toBe("Subtitles: Off›");
  });

  // The owner, 2026-11-04: each opens on its current value.
  it("opens Subtitles and the Sound menu's Sound track each on its current choice", async () => {
    const tracks = tracksNoting({ subtitles: [], audio: [] });
    const chosen = {
      ...tracks,
      subtitles: { ...tracks.subtitles, chosenTrackKey: "s5" },
      audio: { ...tracks.audio, chosenStreamId: 81 },
    };
    const subtitles = await drawn(chosen);
    await opened(subtitles, "Subtitles");
    await openedSubmenu(subtitles, "Subtitles: English");
    expect(document.activeElement?.textContent).toBe("English");
    expect(document.activeElement?.getAttribute("aria-current")).toBe("true");
    subtitles.unmount();
    const audio = await drawn(chosen);
    await opened(audio, "Sound");
    await openedSubmenu(audio, "Sound track");
    expect(document.activeElement?.textContent).toBe("French");
    expect(document.activeElement?.getAttribute("aria-current")).toBe("true");
  });

  // The owner, 2026-11-23: the subtitles' timing is Subtitles', a stepper that keeps the menu open.
  it("steps the subtitles' timing in Subtitles, the menu open for the next step", async () => {
    const steps: number[] = [];
    const tracks = tracksNoting({ subtitles: [], audio: [] });
    const on = await drawn({
      ...tracks,
      subtitles: {
        ...tracks.subtitles,
        shiftOffset: (stepMs) => {
          steps.push(stepMs);
        },
      },
    });
    await opened(on, "Subtitles");
    const later = on.container.querySelector<HTMLElement>(
      'dialog .stepper button[aria-label="Subtitles later"]',
    );
    await fire(later, new MouseEvent("click", { bubbles: true }));
    await fire(later, new MouseEvent("click", { bubbles: true }));
    expect(steps).toHaveLength(2);
    expect(steps[0]).toBeGreaterThan(0);
    expect(on.container.querySelector("dialog")).not.toBeNull();
  });

  // The owner, 2026-11-23: "Any button that would have nothing in its submenu should not display at
  // all": with no track to choose and no Find, Subtitles holds only Off.
  it("draws no Subtitles with no subtitle track and no Find subtitles, and draws it with a track", async () => {
    const tracks = tracksNoting({ subtitles: [], audio: [] });
    mounted = await mount(
      withServices(
        <PlayerTracksContext value={{ ...tracks, subtitles: { ...tracks.subtitles, tracks: [] } }}>
          <PlayerMenus videoRef={createRef()} shown={everyMenu} onOpenChange={() => undefined} />
        </PlayerTracksContext>,
      ),
    );
    expect(openerNamed("Subtitles")).toBeUndefined();
    expect(openerNamed("Sound")).toBeDefined();
    mounted.unmount();
    mounted = await mount(
      withServices(
        <PlayerTracksContext value={tracks}>
          <PlayerMenus videoRef={createRef()} shown={everyMenu} onOpenChange={() => undefined} />
        </PlayerTracksContext>,
      ),
    );
    expect(openerNamed("Subtitles")).toBeDefined();
  });

  it("draws nothing without the film's tracks", async () => {
    mounted = await mount(
      withServices(
        <PlayerMenus videoRef={createRef()} shown={everyMenu} onOpenChange={() => undefined} />,
      ),
    );
    expect(mounted.container.querySelector("button")).toBeNull();
  });
});

/** The film's sound as a test holds it: volume and mute, each setting noted in set. */
function soundNoting(set: (number | boolean)[]): PlayerSound {
  return {
    volume: 0.4,
    isMuted: false,
    canSetVolume: true,
    setVolume: (volume) => {
      set.push(volume);
    },
    setMuted: (isMuted) => {
      set.push(isMuted);
    },
  };
}

/** The menu opener whose name starts with name, if one is drawn. */
function openerNamed(name: string): Element | undefined {
  return [...(mounted?.container.querySelectorAll("[aria-haspopup]") ?? [])].find(
    (button) => button.getAttribute("aria-label")?.startsWith(name) === true,
  );
}

describe("🔈 (D407 §10, Q11)", () => {
  it("opens the volume upright and mute, first of the menus, without the film's tracks too", async () => {
    const set: (number | boolean)[] = [];
    const told: boolean[] = [];
    const on = await mount(
      withServices(
        <PlayerMenus
          videoRef={createRef()}
          shown={["sound", ...everyMenu]}
          sound={soundNoting(set)}
          onOpenChange={(isOpen) => told.push(isOpen)}
        />,
      ),
    );
    mounted = on;
    expect(on.container.querySelectorAll("[aria-haspopup]")).toHaveLength(1);
    await opened(on, "Volume");
    const range = on.container.querySelector<HTMLInputElement>("dialog input[type=range]");
    expect(range?.classList.contains("volume-range--upright")).toBe(true);
    expect(range?.value).toBe("40");
    expect(on.container.querySelector("dialog button[aria-label='Mute']")).not.toBeNull();
    expect(told.at(-1)).toBe(true);
  });

  it("is one control: Up and Down step the volume, OK closes it, the slider out of focus's way", async () => {
    const set: (number | boolean)[] = [];
    const told: boolean[] = [];
    const on = await mount(
      withServices(
        <PlayerMenus
          videoRef={createRef()}
          shown={["sound"]}
          sound={soundNoting(set)}
          onOpenChange={(isOpen) => told.push(isOpen)}
        />,
      ),
    );
    mounted = on;
    await opened(on, "Volume");
    const mute = on.container.querySelector("dialog button[aria-label='Mute']");
    expect(mute).not.toBeNull();
    expect(on.container.querySelector<HTMLInputElement>("dialog input[type=range]")?.tabIndex).toBe(
      -1,
    );
    for (const command of ["up", "down", "select"]) {
      expect(offered(mute, command)).toBe(true);
    }
    expect(set).toEqual([0.45, 0.35]); // a step each way from 0.4, as + and − move it
    expect(told.slice(-2)).toEqual([true, false]);
    expect(on.container.querySelector("dialog")).toBeNull();
  });

  it("draws 🔈 before 💬 where the overlay shows the sound, and not where it does not", async () => {
    const tracks = tracksNoting({ subtitles: [], audio: [] });
    mounted = await mount(
      withServices(
        <PlayerTracksContext value={tracks}>
          <PlayerMenus
            videoRef={createRef()}
            shown={["sound", ...everyMenu]}
            sound={soundNoting([])}
            onOpenChange={() => undefined}
          />
        </PlayerTracksContext>,
      ),
    );
    const names = [...mounted.container.querySelectorAll("[aria-haspopup]")].map((button) =>
      button.getAttribute("aria-label"),
    );
    expect(names[0]).toBe("Volume");
    expect(names[1]).toMatch(/^Subtitles/u);
    mounted.unmount();
    await drawn(tracks);
    expect(openerNamed("Volume")).toBeUndefined();
  });
});

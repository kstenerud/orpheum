// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, describe, expect, it } from "vitest";

import type { AudioTrack } from "./audio-tracks.tsx";
import { commandEventName } from "./input-commands.ts";
import type { PlayerSound } from "./player-controls.tsx";
import { PlayerMenus, PlayerMenusContext, type PlayerMenusWays } from "./player-menus.tsx";
import type { OverlayControl } from "./player-overlay-state.ts";
import { PlayerTracksContext, type PlayerTracks } from "./player-tracks.tsx";
import type { SubtitleTrack, TrackKey } from "./subtitles.tsx";
import { fire, mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

function subtitleTrack(overrides: Partial<SubtitleTrack>): SubtitleTrack {
  return {
    streamId: 5,
    language: "en",
    isDefault: false,
    isForced: false,
    isHearingImpaired: false,
    assUrl: "/api/v1/bytes/media/sub/t/5.ass",
    vttUrl: "/api/v1/bytes/media/sub/t/5.vtt",
    ...overrides,
  };
}

const audioTracks: readonly AudioTrack[] = [
  { streamId: 80, audioTrackIndex: 0, language: "en", kind: "plain" },
  { streamId: 81, audioTrackIndex: 1, language: "fr", kind: "plain" },
];

/** What a test's tracks were asked to choose, in order. */
interface Chosen {
  readonly subtitles: (TrackKey | undefined)[];
  readonly audio: number[];
}

/** A film's tracks as a test holds them: what is chosen, noted in chosen. */
function tracksNoting(chosen: Chosen): PlayerTracks {
  const off = { isOn: false, switchTo: () => undefined, problem: undefined };
  return {
    subtitles: {
      tracks: [subtitleTrack({}), subtitleTrack({ streamId: 7, language: "fr" })],
      chosenTrackKey: undefined,
      choose: (key) => {
        chosen.subtitles.push(key);
      },
      offsetMs: 0,
      shiftOffset: () => undefined,
      failure: undefined,
    },
    audio: {
      tracks: audioTracks,
      chosenStreamId: 80,
      choose: (track) => {
        chosen.audio.push(track.streamId);
      },
      problem: undefined,
    },
    levelling: off,
    level: { levelling: { isMeasured: true, gainDb: -2 }, carriedGainDb: 0 },
    measuring: undefined,
    boost: undefined,
  };
}

const ways: PlayerMenusWays = { fileId: 3, soundOffsetMs: 0, onSubtitleFetched: () => undefined };

const everyMenu: readonly OverlayControl[] = ["subtitles", "audio", "more"];

/** Draws the overlay's menus over tracks, telling told each opening and closing. */
async function drawn(tracks: PlayerTracks, told: boolean[] = []): Promise<Mounted> {
  mounted = await mount(
    withServices(
      <PlayerTracksContext value={tracks}>
        <PlayerMenusContext value={ways}>
          <PlayerMenus
            videoRef={createRef()}
            shown={everyMenu}
            onOpenChange={(isOpen) => told.push(isOpen)}
          />
        </PlayerMenusContext>
      </PlayerTracksContext>,
    ),
  );
  return mounted;
}

/** Opens the menu whose button's name starts with name. */
async function opened(on: Mounted, name: string): Promise<void> {
  const opener = [...on.container.querySelectorAll("button[aria-haspopup]")].find(
    (button) => button.getAttribute("aria-label")?.startsWith(name) === true,
  );
  await fire(opener, new MouseEvent("click", { bubbles: true }));
}

/** The places of the open list, by their text. */
function places(on: Mounted): string[] {
  return [...on.container.querySelectorAll("dialog .list-box li")].map(
    (place) => place.textContent,
  );
}

/** Presses the open list's place whose text is text. */
async function choosing(on: Mounted, text: string): Promise<void> {
  const place = [...on.container.querySelectorAll("dialog .list-box button")].find(
    (button) => button.textContent === text,
  );
  await fire(place, new MouseEvent("click", { bubbles: true }));
}

describe("the overlay's menus (D407 §8)", () => {
  it("lists Off, the tracks and Find subtitles under 💬, and chooses one", async () => {
    const chosen: Chosen = { subtitles: [], audio: [] };
    const on = await drawn(tracksNoting(chosen));
    await opened(on, "Subtitles");
    expect(places(on)).toEqual(["Off", "English", "French", "Find subtitles…"]);
    await choosing(on, "French");
    expect(chosen.subtitles).toEqual(["s7"]);
    expect(on.container.querySelector("dialog")).toBeNull();
  });

  it("lists the audio tracks under 🎧, and chooses one", async () => {
    const chosen: Chosen = { subtitles: [], audio: [] };
    const on = await drawn(tracksNoting(chosen));
    await opened(on, "Sound");
    expect(places(on)).toEqual(["English", "French"]);
    await choosing(on, "French");
    expect(chosen.audio).toEqual([81]);
  });

  it("holds the rest under ⁝: Start over, the subtitles' offset, levelling", async () => {
    const on = await drawn(tracksNoting({ subtitles: [], audio: [] }));
    await opened(on, "More");
    const shown = places(on).join(" | ");
    expect(shown).toContain("Start over");
    expect(shown).toContain("Subtitle offset");
    expect(shown).toContain("Level");
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

  it("names 🎧 by the first track where none is chosen, as the playback plays it", async () => {
    const tracks = tracksNoting({ subtitles: [], audio: [] });
    await drawn({ ...tracks, audio: { ...tracks.audio, chosenStreamId: undefined } });
    expect(openerNamed("Sound")?.getAttribute("aria-label")).toBe("Sound: English");
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

/** A command offered at target, as the input layer offers one; whether something took it. */
function offered(target: Element | null, command: string): boolean {
  const event = new CustomEvent(commandEventName, {
    detail: { command, source: "key" },
    bubbles: true,
    cancelable: true,
  });
  act(() => {
    target?.dispatchEvent(event);
  });
  return event.defaultPrevented;
}

/** The menu opener whose name starts with name, if one is drawn. */
function openerNamed(name: string): Element | undefined {
  return [...(mounted?.container.querySelectorAll("button[aria-haspopup]") ?? [])].find(
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
    expect(on.container.querySelectorAll("button[aria-haspopup]")).toHaveLength(1);
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
    const names = [...mounted.container.querySelectorAll("button[aria-haspopup]")].map((button) =>
      button.getAttribute("aria-label"),
    );
    expect(names[0]).toBe("Volume");
    expect(names[1]).toMatch(/^Subtitles/u);
    mounted.unmount();
    await drawn(tracks);
    expect(openerNamed("Volume")).toBeUndefined();
  });
});

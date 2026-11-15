// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { tellOfferedRungs } from "@orpheum/player";
import { act, createRef, type RefObject } from "react";
import { afterEach, describe, expect, it } from "vitest";

import type { AudioTrack } from "./audio-tracks.tsx";
import { DataCapChoices } from "./data-cap-choices.tsx";
import { DataCaps, dataCapsKey, deviceDataCaps } from "./data-cap.ts";
import { DebugToolsContext } from "./debug-tools.tsx";
import { commandEventName } from "./input-commands.ts";
import { ListBox } from "./list-box.tsx";
import type { PlayerSound } from "./player-controls.tsx";
import { PlayerMenus, PlayerMenusContext, type PlayerMenusWays } from "./player-menus.tsx";
import type { OverlayControl } from "./player-overlay-state.ts";
import { PlayerTracksContext, type PlayerTracks } from "./player-tracks.tsx";
import type { SubtitleTrack, TrackKey } from "./subtitles.tsx";
import { fire, mount, type Mounted } from "./test-dom.tsx";
import { FakeConnection, heldStorage } from "./test-data-cap.ts";
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
  const opener = [...on.container.querySelectorAll("[aria-haspopup]")].find(
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
  const place = [...on.container.querySelectorAll('dialog .list-box [role="button"]')].find(
    (button) => button.textContent === text,
  );
  await fire(place, new MouseEvent("click", { bubbles: true }));
}

describe("the overlay's menus (D407 §8)", () => {
  it("lists Off, the subtitle tracks and Find subtitles in Subtitles, no audio track, and chooses one", async () => {
    const chosen: Chosen = { subtitles: [], audio: [] };
    const on = await drawn(tracksNoting(chosen));
    await opened(on, "Subtitles");
    expect(places(on)).toEqual(["Off", "English", "French", "Find subtitles…"]);
    await choosing(on, "French");
    expect(chosen.subtitles).toEqual(["s7"]);
    expect(on.container.querySelector("dialog")).toBeNull();
  });

  it("lists the audio tracks alone in Sound track, and chooses one", async () => {
    const chosen: Chosen = { subtitles: [], audio: [] };
    const on = await drawn(tracksNoting(chosen));
    await opened(on, "Sound track");
    expect(places(on)).toEqual(["English", "French"]);
    await choosing(on, "French");
    expect(chosen.audio).toEqual([81]);
  });

  it("holds the rest under ⁝: Start over, the subtitles' offset, levelling", async () => {
    const on = await drawn(tracksNoting({ subtitles: [], audio: [] }));
    await opened(on, "Settings");
    const shown = places(on).join(" | ");
    expect(shown).toContain("Start over");
    expect(shown).toContain("Subtitle offset");
    expect(shown).toContain("Level");
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
    expect(places(mounted)).toContain("Plays it converted · Quality 4.2 Mb/s");
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
    await drawn({ ...tracks, audio: { ...tracks.audio, chosenStreamId: undefined } });
    expect(openerNamed("Sound track")?.getAttribute("aria-label")).toBe("Sound track: English");
  });

  // The owner, 2026-11-04: Sound track and Subtitles are two menus, not one panel.
  it("names Subtitles by its choice alone, a button apart from Sound track's", async () => {
    await drawn(tracksNoting({ subtitles: [], audio: [] }));
    const names = [...document.querySelectorAll("[aria-haspopup]")].map((button) =>
      button.getAttribute("aria-label"),
    );
    expect(names).toEqual(["Subtitles: Off", "Sound track: English", "Settings"]);
    expect(openerNamed("Subtitles")?.getAttribute("aria-label")).toBe("Subtitles: Off");
  });

  // The owner, 2026-11-04: each opens on its current value.
  it("opens Subtitles and Sound track each on its current choice", async () => {
    const tracks = tracksNoting({ subtitles: [], audio: [] });
    const chosen = {
      ...tracks,
      subtitles: { ...tracks.subtitles, chosenTrackKey: "s5" },
      audio: { ...tracks.audio, chosenStreamId: 81 },
    };
    const subtitles = await drawn(chosen);
    await opened(subtitles, "Subtitles");
    expect(document.activeElement?.textContent).toBe("English");
    expect(document.activeElement?.getAttribute("aria-current")).toBe("true");
    subtitles.unmount();
    const audio = await drawn(chosen);
    await opened(audio, "Sound track");
    expect(document.activeElement?.textContent).toBe("French");
    expect(document.activeElement?.getAttribute("aria-current")).toBe("true");
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

/**
 * A video whose player told it the core's default rungs, each with its 96 kb/s sound (D488), and,
 * given ownPictureKbps, the film's own picture above them at that rate with its sound (D491).
 */
function videoOnTheDefaultLadder(ownPictureKbps?: number): RefObject<HTMLVideoElement | null> {
  const video = document.createElement("video");
  const levels = [400, 700, 1200, 2000, 3000, 6000].map((kbps) => ({
    bitrate: kbps * 1500,
    averageBitrate: (kbps + 96) * 1000,
    uri: `q${String(kbps)}k/stream.m3u8`,
  }));
  if (ownPictureKbps !== undefined) {
    levels.push({
      bitrate: ownPictureKbps * 3000,
      averageBitrate: ownPictureKbps * 1000,
      uri: "picture/stream.m3u8",
    });
  }
  let parsed: (() => void) | undefined = undefined;
  tellOfferedRungs(
    {
      levels,
      on: (_, listener) => {
        parsed = listener;
      },
    },
    video,
  );
  act(() => {
    parsed?.();
  });
  return { current: video };
}

describe("⁝'s data use (D488)", () => {
  afterEach(() => {
    window.localStorage.removeItem(dataCapsKey);
  });

  it("lists No limit, then one choice per rung the film is offered, highest first, in GB per hour", async () => {
    mounted = await mount(
      withServices(
        <DebugToolsContext value={{ isShown: true, onChanged: () => undefined }}>
          <PlayerTracksContext value={tracksNoting({ subtitles: [], audio: [] })}>
            <PlayerMenusContext value={{ ...ways, onOriginalChosen: () => undefined }}>
              <PlayerMenus
                videoRef={videoOnTheDefaultLadder()}
                shown={everyMenu}
                onOpenChange={() => undefined}
              />
            </PlayerMenusContext>
          </PlayerTracksContext>
        </DebugToolsContext>,
      ),
    );
    await opened(mounted, "Settings");
    const perHour = ["2.7", "1.4", "0.9", "0.6", "0.4", "0.2"].map((gb) => `${gb} GB per hour`);
    expect(places(mounted)).toEqual(expect.arrayContaining(["Data use", "No limit", ...perHour]));
    const listed = places(mounted);
    expect(listed.slice(listed.indexOf("Data use"), listed.indexOf("Data use") + 8)).toEqual([
      "Data use",
      "No limit",
      ...perHour,
    ]);
    expect(currentPlaces(mounted)).toContain("No limit");
    expect(places(mounted)).toContain("Play the original file");
    await choosing(mounted, "0.6 GB per hour");
    expect(JSON.parse(window.localStorage.getItem(dataCapsKey) ?? "")).toEqual({ unknown: 1296 });
    await opened(mounted, "Settings");
    expect(currentPlaces(mounted)).toEqual(["0.6 GB per hour"]);
    // A capped device plays a rung, never the film's own file (the owner, 2026-11-14).
    expect(places(mounted)).not.toContain("Play the original file");
  });

  it("offers nothing while the film plays no ladder, its own file", async () => {
    await drawn(tracksNoting({ subtitles: [], audio: [] }));
    await opened(mounted!, "Settings");
    expect(places(mounted!)).not.toContain("No limit");
  });

  it("names the network its cap is for where the browser says which, and follows the device's move", async () => {
    const connection = new FakeConnection("cellular");
    const { storage } = heldStorage();
    const caps = new DataCaps(storage, connection);
    caps.choose(496);
    mounted = await mount(
      <ListBox isFromItsTop>
        <DataCapChoices videoRef={videoOnTheDefaultLadder()} caps={caps} />
      </ListBox>,
    );
    const heading = (): string | null | undefined =>
      mounted?.container.querySelector(".list-box__note")?.textContent;
    const current = (): (string | null)[] =>
      [...(mounted?.container.querySelectorAll('[aria-current="true"]') ?? [])].map(
        (place) => place.textContent,
      );
    expect(heading()).toBe("Data use on mobile data");
    expect(current()).toEqual(["0.2 GB per hour"]);
    act(() => {
      connection.moveTo("wifi");
    });
    expect(heading()).toBe("Data use on Wi-Fi");
    expect(current()).toEqual(["No limit"]);
  });

  it("lists the film's own picture as one more rung, and offers its original under a cap at or above the file's rate (D491)", async () => {
    mounted = await mount(
      withServices(
        <DebugToolsContext value={{ isShown: true, onChanged: () => undefined }}>
          <PlayerTracksContext value={tracksNoting({ subtitles: [], audio: [] })}>
            <PlayerMenusContext
              value={{ ...ways, fileBitRateKbps: 9096, onOriginalChosen: () => undefined }}
            >
              <PlayerMenus
                videoRef={videoOnTheDefaultLadder(8000)}
                shown={everyMenu}
                onOpenChange={() => undefined}
              />
            </PlayerMenusContext>
          </PlayerTracksContext>
        </DebugToolsContext>,
      ),
    );
    await opened(mounted, "Settings");
    const listed = places(mounted);
    // The own picture's 8000 kb/s, its sound's with it, is 3.6 GB an hour.
    expect(listed.slice(listed.indexOf("No limit"), listed.indexOf("No limit") + 3)).toEqual([
      "No limit",
      "3.6 GB per hour",
      "2.7 GB per hour",
    ]);
    await choosing(mounted, "3.6 GB per hour");
    expect(JSON.parse(window.localStorage.getItem(dataCapsKey) ?? "")).toEqual({ unknown: 8000 });
    await opened(mounted, "Settings");
    expect(currentPlaces(mounted)).toEqual(["3.6 GB per hour"]);
    // The file's own 9096 kb/s, every track of it, is over the cap.
    expect(places(mounted)).not.toContain("Play the original file");
    act(() => {
      deviceDataCaps().choose(9096);
    });
    expect(places(mounted)).toContain("Play the original file");
  });
});

/** The places of the open list marked as its current choice, by their text. */
function currentPlaces(on: Mounted): string[] {
  return [...on.container.querySelectorAll('dialog .list-box [aria-current="true"]')].map(
    (place) => place.textContent,
  );
}

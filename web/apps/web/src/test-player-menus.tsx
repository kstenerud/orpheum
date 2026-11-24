// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { tellOfferedRungs } from "@orpheum/player";
import { act, createRef, type RefObject } from "react";

import type { AudioTrack } from "./audio-tracks.tsx";

import { commandEventName } from "./input-commands.ts";

import { PlayerMenus, PlayerMenusContext, type PlayerMenusWays } from "./player-menus.tsx";
import type { OverlayControl } from "./player-overlay-state.ts";
import { PlayerTracksContext, type PlayerTracks } from "./player-tracks.tsx";
import type { SubtitleTrack, TrackKey } from "./subtitles.tsx";
import { fire, mount, type Mounted } from "./test-dom.tsx";

import { testServices, withServices } from "./test-services.tsx";

export function subtitleTrack(overrides: Partial<SubtitleTrack>): SubtitleTrack {
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

export const audioTracks: readonly AudioTrack[] = [
  { streamId: 80, audioTrackIndex: 0, language: "en", kind: "plain" },
  { streamId: 81, audioTrackIndex: 1, language: "fr", kind: "plain" },
];

/** What a test's tracks were asked to choose, in order. */
export interface Chosen {
  readonly subtitles: (TrackKey | undefined)[];
  readonly audio: number[];
}

/** A film's tracks as a test holds them: what is chosen, noted in chosen. */
export function tracksNoting(chosen: Chosen): PlayerTracks {
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

export const ways: PlayerMenusWays = {
  fileId: 3,
  soundOffsetMs: 0,
  onSubtitleFetched: () => undefined,
};

export const everyMenu: readonly OverlayControl[] = ["subtitles", "audio", "more"];

/** Draws the overlay's menus over tracks, telling told each opening and closing. */
export async function drawnMenus(tracks: PlayerTracks, told: boolean[] = []): Promise<Mounted> {
  return await mount(
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
}

/** Opens the menu whose button's name starts with name. */
export async function opened(on: Mounted, name: string): Promise<void> {
  const opener = [...on.container.querySelectorAll("[aria-haspopup]")].find(
    (button) => button.getAttribute("aria-label")?.startsWith(name) === true,
  );
  await fire(opener, new MouseEvent("click", { bubbles: true }));
}

/**
 * The places of the open list shown, by their text: not those an open submenu hides, nor the place
 * that holds it, whose own places are listed in its stead (D490).
 */
export function places(on: Mounted): string[] {
  return [...on.container.querySelectorAll("dialog .list-box li")]
    .filter((place) => !place.hasAttribute("hidden") && place.querySelector("li") === null)
    .map((place) => place.textContent);
}

/** Opens the open list's submenu whose entry's text starts with name. */
export async function openedSubmenu(on: Mounted, name: string): Promise<void> {
  const entry = [...on.container.querySelectorAll("dialog .list-box li")].find(
    (place) => place.textContent.startsWith(name) && place.querySelector("li") === null,
  );
  await fire(
    entry?.querySelector('[role="button"], button') ?? entry,
    new MouseEvent("click", { bubbles: true }),
  );
}

/** Presses the open list's place whose text is text. */
export async function choosing(on: Mounted, text: string): Promise<void> {
  const place = [...on.container.querySelectorAll('dialog .list-box [role="button"]')].find(
    (button) => button.textContent === text,
  );
  await fire(place, new MouseEvent("click", { bubbles: true }));
}

/**
 * A video whose player told it the core's default rungs, each with its 96 kb/s sound (D488), and,
 * given ownPictureKbps, the film's own picture above them at that rate with its sound (D491).
 */
export function videoOnTheDefaultLadder(
  ownPictureKbps?: number,
): RefObject<HTMLVideoElement | null> {
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

/** ⁝ over a film on the core's default rungs, over services (by default a test's own). */
export async function drawnOnTheLadder(
  services: ReturnType<typeof testServices> = testServices(),
): Promise<Mounted> {
  return await mount(
    withServices(
      <PlayerTracksContext value={tracksNoting({ subtitles: [], audio: [] })}>
        <PlayerMenusContext value={ways}>
          <PlayerMenus
            videoRef={videoOnTheDefaultLadder()}
            shown={everyMenu}
            onOpenChange={() => undefined}
          />
        </PlayerMenusContext>
      </PlayerTracksContext>,
      services,
    ),
  );
}

/** The text of what has focus. */
export function focusedText(): string | null | undefined {
  return document.activeElement?.textContent;
}

/** The places of the open list marked as its current choice, by their text. */
export function currentPlaces(on: Mounted): string[] {
  return [...on.container.querySelectorAll('dialog .list-box [aria-current="true"]')].map(
    (place) => place.textContent,
  );
}

/** A command offered at target, as the input layer offers one; whether something took it. */
export function offered(target: Element | null, command: string): boolean {
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

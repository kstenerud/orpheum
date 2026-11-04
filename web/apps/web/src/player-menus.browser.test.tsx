// SPDX-License-Identifier: MPL-2.0

// The overlay's pop-ups laid out by a real browser (D408 §2): what they hold decides their size.

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { createRef } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import type { AudioTrack } from "./audio-tracks.tsx";
import type { PlayerSound } from "./player-controls.tsx";
import { PlayerMenus, PlayerMenusContext, type PlayerMenusWays } from "./player-menus.tsx";
import { PlayerTracksContext, type PlayerTracks } from "./player-tracks.tsx";
import type { SubtitleTrack } from "./subtitles.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A film's sound at half, which the test never sets. */
const sound: PlayerSound = {
  volume: 0.5,
  isMuted: false,
  canSetVolume: true,
  setVolume: () => undefined,
  setMuted: () => undefined,
};

/** How far layout's rounding may put a box. */
const roundingPx = 1;

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

describe("the overlay's pop-ups, laid out by a real browser", () => {
  // The owner, 2026-11-02: "The volume control popup is far too wide."
  it("draws 🔈's pop-up as wide as its slider and mute, with no Close", async () => {
    await page.viewport(1280, 800);
    mounted = await mount(
      withServices(
        <div style={{ position: "fixed", right: "16px", bottom: "16px" }}>
          <PlayerMenus
            videoRef={createRef()}
            shown={["sound"]}
            sound={sound}
            onOpenChange={() => undefined}
          />
        </div>,
      ),
    );
    await userEvent.click(page.getByRole("button", { name: "Volume" }));
    const popUp = document.querySelector("dialog");
    const controls = popUp?.querySelector(".player-controls__sound") ?? null;
    const [slider, mute] = [
      controls?.querySelector(".volume-range") ?? null,
      controls?.querySelector("button") ?? null,
    ];
    expect(popUp?.querySelector('button[aria-label="Close"]')).toBeNull();
    // The pop-up no wider than what it holds, and that no wider than the slider standing upright or
    // mute under it.
    expect(boxOf(popUp).width).toBeLessThanOrEqual(
      boxOf(controls).width + sidesOf(popUp, "border") + sidesOf(popUp, "padding") + roundingPx,
    );
    expect(boxOf(controls).width).toBeLessThanOrEqual(
      Math.max(
        boxOf(slider).width + sidesOf(slider, "margin"),
        boxOf(mute).width + sidesOf(mute, "margin"),
      ) +
        sidesOf(controls, "padding") +
        roundingPx,
    );
    // The slider upright, and a fingertip wide, as mute is: no wider than its button.
    expect(boxOf(slider).width).toBeLessThan(boxOf(slider).height);
    expect(boxOf(slider).width).toBeLessThanOrEqual(boxOf(mute).width + roundingPx);
  });

  // D407 §8: Earlier and Later beside what they move, not each a whole place as a choice is.
  it("sets ⁝'s subtitle offset in one row, its text in line with the choices' text", async () => {
    await page.viewport(1280, 800);
    mounted = await mount(
      withServices(
        <PlayerTracksContext value={subtitledTracks}>
          <PlayerMenusContext value={ways}>
            <PlayerMenus videoRef={createRef()} shown={["more"]} onOpenChange={() => undefined} />
          </PlayerMenusContext>
        </PlayerTracksContext>,
      ),
    );
    await userEvent.click(page.getByRole("button", { name: "Settings" }));
    const startOver = drawnIn("dialog .list-box__option");
    const offset = drawnIn("dialog output");
    const [earlier, later] = [offset.previousElementSibling, offset.nextElementSibling];
    expect(startOver.textContent).toBe("Start over");
    const choiceTextLeft = boxOf(startOver).left + sidesOf(startOver, "padding") / 2;
    expect(boxOf(earlier).left).toBeCloseTo(choiceTextLeft, 0);
    const middle = (box: DOMRect): number => box.top + box.height / 2;
    for (const part of [offset, later]) {
      expect(middle(boxOf(part))).toBeCloseTo(middle(boxOf(earlier)), 0);
    }
    // A step of the space scale between them, --space-2.
    expect(boxOf(offset).left - boxOf(earlier).right).toBeCloseTo(8, 0);
    expect(boxOf(later).left - boxOf(offset).right).toBeCloseTo(8, 0);
  });
});

// ui-audit-2026-11-04.md §3, The player: "Audio and subtitles (one panel, 3–4 presses to change a
// track)". Two menus, 💬 and 🎧, each took a walk along the row to reach before.
describe("Audio and subtitles, walked by a remote's keys", () => {
  /** Draws the panel's button for a film of two audio tracks and one subtitle track, focused. */
  async function drawnFocused(chosen: { audio: number[]; subtitles: string[] }): Promise<void> {
    await page.viewport(960, 480);
    mounted = await mount(
      withServices(
        <PlayerTracksContext value={tracksNoting(chosen)}>
          <PlayerMenus
            videoRef={createRef()}
            shown={["audioAndSubtitles"]}
            onOpenChange={() => undefined}
          />
        </PlayerTracksContext>,
      ),
    );
    const opener = document.querySelector<HTMLElement>('button[aria-label^="Audio and subtitles"]');
    opener?.focus();
    expect(document.activeElement).toBe(opener);
  }

  /**
   * Opens the panel by OK, then presses Down until focus is on the entry after the heading, and
   * OK: the presses it took from the opening, OK included.
   */
  async function pressesToChoose(heading: string, entry: string): Promise<number> {
    await userEvent.keyboard("{Enter}");
    await expect.poll(() => document.querySelector("dialog .list-box") !== null).toBe(true);
    let presses = 0;
    while (!isFocusedUnder(heading, entry) && presses < 8) {
      await userEvent.keyboard("{ArrowDown}");
      presses += 1;
    }
    await userEvent.keyboard("{Enter}");
    return presses + 1;
  }

  it("changes the audio track within 4 presses of its opening", async () => {
    const chosen = { audio: [] as number[], subtitles: [] as string[] };
    await drawnFocused(chosen);
    expect(await pressesToChoose("Audio", "French")).toBeLessThanOrEqual(4);
    expect(chosen).toEqual({ audio: [81], subtitles: [] });
  });

  it("changes the subtitles within 4 presses of its opening", async () => {
    const chosen = { audio: [] as number[], subtitles: [] as string[] };
    await drawnFocused(chosen);
    expect(await pressesToChoose("Subtitles", "English")).toBeLessThanOrEqual(4);
    expect(chosen).toEqual({ audio: [], subtitles: ["s5"] });
  });
});

/** Whether focus is on the panel's entry named so, among those after the heading named so. */
function isFocusedUnder(heading: string, entry: string): boolean {
  const entries = [...document.querySelectorAll("dialog .list-box > *")];
  const from = entries.findIndex((each) => each.textContent === heading);
  const after = entries.slice(from + 1);
  const end = after.findIndex((each) => !each.matches(".list-box__entry"));
  const group = end === -1 ? after : after.slice(0, end);
  const focused = document.activeElement;
  return group.some((each) => each.textContent === entry && each.contains(focused));
}

/** A film's tracks of two audio tracks, English playing, and an English subtitle track, off. */
function tracksNoting(chosen: { audio: number[]; subtitles: string[] }): PlayerTracks {
  return {
    ...subtitledTracks,
    subtitles: {
      ...subtitledTracks.subtitles,
      tracks: [subtitleTrack(5)],
      choose: (key) => {
        chosen.subtitles.push(key ?? "off");
      },
    },
    audio: {
      tracks: twoAudioTracks,
      chosenStreamId: 80,
      choose: (track) => {
        chosen.audio.push(track.streamId);
      },
      problem: undefined,
    },
  };
}

const twoAudioTracks: readonly AudioTrack[] = [
  { streamId: 80, audioTrackIndex: 0, language: "en", kind: "plain" },
  { streamId: 81, audioTrackIndex: 1, language: "fr", kind: "plain" },
];

/** The first element the selector finds, or a failure where none is drawn. */
function drawnIn(selector: string): Element {
  const element = document.querySelector(selector);
  if (element === null) {
    throw new Error(`nothing drawn matches ${selector}`);
  }
  return element;
}

/** A film's tracks with two subtitle tracks, so ⁝ holds their offset; it chooses nothing. */
const subtitledTracks: PlayerTracks = {
  subtitles: {
    tracks: [subtitleTrack(5), subtitleTrack(7)],
    chosenTrackKey: undefined,
    choose: () => undefined,
    offsetMs: 0,
    shiftOffset: () => undefined,
    failure: undefined,
  },
  audio: { tracks: [], chosenStreamId: undefined, choose: () => undefined, problem: undefined },
  levelling: { isOn: false, switchTo: () => undefined, problem: undefined },
  level: { levelling: { isMeasured: true, gainDb: -2 }, carriedGainDb: 0 },
  measuring: undefined,
  boost: undefined,
};

/** The page's ways for ⁝: a file, no sound timing kept. */
const ways: PlayerMenusWays = { fileId: 3, soundOffsetMs: 0, onSubtitleFetched: () => undefined };

/** An English subtitle track, its stream streamId. */
function subtitleTrack(streamId: number): SubtitleTrack {
  return {
    streamId,
    language: "en",
    isDefault: false,
    isForced: false,
    isHearingImpaired: false,
    assUrl: `/api/v1/bytes/media/sub/t/${String(streamId)}.ass`,
    vttUrl: `/api/v1/bytes/media/sub/t/${String(streamId)}.vtt`,
  };
}

/** An element's box, or a failure where it is not drawn. */
function boxOf(element: Element | null | undefined): DOMRect {
  if (element === null || element === undefined) {
    throw new Error("an element the test measures is not drawn");
  }
  return element.getBoundingClientRect();
}

/** An element's border, padding or margin on its left and right together, in pixels. */
function sidesOf(element: Element | null, edge: "border" | "padding" | "margin"): number {
  if (element === null) {
    throw new Error("an element the test measures is not drawn");
  }
  const its = getComputedStyle(element);
  const sides = {
    border: [its.borderLeftWidth, its.borderRightWidth],
    padding: [its.paddingLeft, its.paddingRight],
    margin: [its.marginLeft, its.marginRight],
  } as const;
  const [left, right] = sides[edge];
  return Number.parseFloat(left) + Number.parseFloat(right);
}

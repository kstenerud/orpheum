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

  it("keeps ⁝'s Sound earlier where it is as the timing's text changes (the owner's TCL TV, 2026-11-06)", async () => {
    await page.viewport(960, 480);
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
    const earlier = page.getByRole("button", { name: "Sound 50 ms earlier" }).element();
    const text = [...document.querySelectorAll("dialog span")].find((each) =>
      each.textContent.startsWith("Sound on this device"),
    );
    const before = boxOf(earlier);
    expect(boxOf(text).bottom).toBeLessThanOrEqual(before.top + roundingPx);
    for (const press of [1, 2, 3]) {
      await userEvent.click(earlier);
      await expect
        .element(page.getByText(`Sound on this device: ${String(press * 50)} ms earlier`))
        .toBeVisible();
      expect(boxOf(earlier).top).toBeCloseTo(before.top, 0);
      expect(boxOf(earlier).left).toBeCloseTo(before.left, 0);
    }
  });
});

// The owner, 2026-11-04: Sound track and Subtitles are two menus, not one panel, "each opens on its current
// value, with at least one choice above it and one below it in view where there are any".
describe("Subtitles and Sound track, opened by a remote's OK", () => {
  /** Draws the two menus for a film of many tracks, the one named opener focused, and opens it. */
  async function openedByOk(opener: string, tracks: PlayerTracks): Promise<void> {
    await page.viewport(960, 480);
    mounted = await mount(
      withServices(
        <div style={{ position: "fixed", right: "16px", bottom: "16px" }}>
          <PlayerTracksContext value={tracks}>
            <PlayerMenus
              videoRef={createRef()}
              shown={["subtitles", "audio"]}
              onOpenChange={() => undefined}
            />
          </PlayerTracksContext>
        </div>,
      ),
    );
    const button = document.querySelector<HTMLElement>(
      `[aria-haspopup][aria-label^="${opener}: "]`,
    );
    button?.focus();
    expect(document.activeElement).toBe(button);
    // A remote's OK is Enter on the focused opener, whose click Firefox under load can drop
    // (idiosyncrasies.md); the click Enter makes on a button is what the test gives it.
    button?.click();
    await expect.poll(() => document.querySelector("dialog .list-box") !== null).toBe(true);
  }

  it("opens Subtitles on its current choice, a choice above and below it in view", async () => {
    await openedByOk("Subtitles", manyTracks({ subtitleKey: "s19", audioStreamId: 80 }));
    const shown = focusedAndNeighbours();
    expect(shown.text).toBe("English · Track 19");
    expect(shown.isCurrent).toBe(true);
    // The list is cut by the screen, so the test shows its scrolling, not a list wholly in view.
    expect(shown.isListCut).toBe(true);
    expect(shown.showsNeighbours).toBe(true);
  });

  it("opens Sound track on its current choice, a choice above and below it in view", async () => {
    await openedByOk("Sound track", manyTracks({ subtitleKey: undefined, audioStreamId: 89 }));
    const shown = focusedAndNeighbours();
    expect(shown.text).toBe("English · Track 9");
    expect(shown.isCurrent).toBe(true);
    // The list is cut by the screen, so the test shows its scrolling, not a list wholly in view.
    expect(shown.isListCut).toBe(true);
    expect(shown.showsNeighbours).toBe(true);
  });
});

/**
 * The open list's focused choice: its text, whether it is marked current, whether the list is cut
 * by the screen, and whether the choices either side of it are wholly within the list's view.
 */
function focusedAndNeighbours(): {
  text: string | null | undefined;
  isCurrent: boolean;
  isListCut: boolean;
  showsNeighbours: boolean;
} {
  const focused = document.activeElement;
  const list = drawnIn("dialog .list-box");
  const place = focused?.closest("li");
  const view = boxOf(list);
  const isInView = (neighbour: Element | null | undefined): boolean =>
    boxOf(neighbour).top >= view.top - roundingPx &&
    boxOf(neighbour).bottom <= view.bottom + roundingPx;
  return {
    text: focused?.textContent,
    isCurrent: focused?.getAttribute("aria-current") === "true",
    isListCut: list.scrollHeight > list.clientHeight,
    showsNeighbours: isInView(place?.previousElementSibling) && isInView(place?.nextElementSibling),
  };
}

/** Audio and subtitle tracks enough to cut the lists, each English and titled "Track n". */
function manyTracks(chosen: {
  subtitleKey: string | undefined;
  audioStreamId: number;
}): PlayerTracks {
  const count = 30;
  const numbers = Array.from({ length: count }, (_, index) => index + 1);
  return {
    ...subtitledTracks,
    subtitles: {
      ...subtitledTracks.subtitles,
      tracks: numbers.map((n) => ({ ...subtitleTrack(n), containerTitle: `Track ${String(n)}` })),
      chosenTrackKey: chosen.subtitleKey,
    },
    audio: {
      tracks: numbers.map((n): AudioTrack => ({
        streamId: 80 + n,
        audioTrackIndex: n,
        language: "en",
        kind: "plain",
        containerTitle: `Track ${String(n)}`,
      })),
      chosenStreamId: chosen.audioStreamId,
      choose: () => undefined,
      problem: undefined,
    },
  };
}

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

// SPDX-License-Identifier: MPL-2.0

// The overlay's pop-ups laid out by a real browser (D408 §2): what they hold decides their size.

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { createRef, useState, type JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import type { AudioTrack } from "./audio-tracks.tsx";
import { startInput } from "./input-commands.ts";
import type { PlayerSound } from "./player-controls.tsx";
import { PlayerMenus, PlayerMenusContext, type PlayerMenusWays } from "./player-menus.tsx";
import { PlayerTracksContext, type PlayerTracks } from "./player-tracks.tsx";
import { forgetSoundOffsetSetInThisTab } from "./sound-delay.tsx";
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

  // D407 §8: the timing's parts beside what they move, its label in line with the choices' text.
  it("sets ⁝'s subtitle timing in one row, its label in line with the choices' text", async () => {
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
    const label = [...document.querySelectorAll("dialog .stepper__label")].find(
      (each) => each.textContent === "Subtitle timing",
    );
    const earlier = page.getByRole("button", { name: "Subtitles earlier" }).element();
    const later = page.getByRole("button", { name: "Subtitles later" }).element();
    const field = page.getByRole("textbox", { name: /^Subtitle timing, in seconds/u }).element();
    expect(startOver.textContent).toBe("Start over");
    const choiceTextLeft = boxOf(startOver).left + sidesOf(startOver, "padding") / 2;
    expect(boxOf(label).left).toBeCloseTo(choiceTextLeft, 0);
    const middle = (box: DOMRect): number => box.top + box.height / 2;
    for (const part of [field, later]) {
      expect(middle(boxOf(part))).toBeCloseTo(middle(boxOf(earlier)), 0);
    }
    // In seconds, as the spec's Timing row shows it.
    expect((field as HTMLInputElement).value).toBe("0.0");
  });

  // The plan's "A video's own sound offset": ⁝'s Later was out of a remote's reach, three buttons
  // with no row; a stepper's row takes Left and Right along it (D538 §9).
  it("reaches ⁝'s subtitle timing's Later by a remote's Right, and steps it by OK, at 360, 1280 and 1920 wide", async () => {
    const input = startInput({ goBack: () => false, isNavigating: () => true });
    try {
      for (const width of [360, 1280, 1920]) {
        await page.viewport(width, 800);
        mounted = await mount(withServices(<LiveSubtitleOffset />));
        await userEvent.click(page.getByRole("button", { name: "Settings" }));
        const field = page
          .getByRole("textbox", { name: /^Subtitle timing, in seconds/u })
          .element() as HTMLInputElement;
        field.focus();
        field.setSelectionRange(field.value.length, field.value.length);
        await userEvent.keyboard("{ArrowRight}");
        expect(document.activeElement?.getAttribute("aria-label")).toBe("Subtitles later");
        expect(document.activeElement?.matches(":focus-visible")).toBe(true);
        await userEvent.keyboard("{Enter}");
        await expect.poll(() => field.value).toBe("+0.1");
        await expect
          .poll(() => document.querySelector("dialog .stepper__caption")?.textContent)
          .toBe("Subtitles show 0.1 s later");
        // The menu stays open for the next press, focus where it was.
        expect(document.querySelector("dialog[open]")).not.toBeNull();
        expect(document.activeElement?.getAttribute("aria-label")).toBe("Subtitles later");
        mounted.unmount();
        mounted = undefined;
      }
    } finally {
      input.stop();
    }
  });

  // The owner, 2026-11-22: "Sound on this device" is a text label with the stepper right beside
  // it, in ⁝'s row itself, not a button that opens a pop-up; and, on the owner's TCL TV
  // (2026-11-06), its − and + stay where they are as it steps (D538 §9).
  it("is ⁝'s row Sound on this device, its label with the stepper beside it, which stays put as it steps", async () => {
    await page.viewport(960, 480);
    await openMore();
    const label = [...document.querySelectorAll("dialog span")].find(
      (each) => each.textContent === "Sound on this device",
    );
    const field = soundField();
    const earlier = page.getByRole("button", { name: "Sound 50 ms earlier" }).element();
    const middle = (box: DOMRect): number => box.top + box.height / 2;
    for (const part of [field, earlier]) {
      expect(middle(boxOf(part))).toBeCloseTo(middle(boxOf(label)), 0);
    }
    expect(boxOf(label).right).toBeLessThanOrEqual(boxOf(earlier).left + roundingPx);
    expect(boxOf(earlier).right).toBeLessThanOrEqual(boxOf(field).left + roundingPx);
    const before = boxOf(earlier);
    for (const press of [1, 2, 3]) {
      await userEvent.click(earlier);
      // Below zero with the minus sign, as the stepper shows it.
      await expect.poll(() => field.value).toBe(`−${String(press * 50)}`);
      expect(boxOf(earlier).top).toBeCloseTo(before.top, 0);
      expect(boxOf(earlier).left).toBeCloseTo(before.left, 0);
    }
    // Its caption spells the value out under the control, where it moves nothing in the row, and a
    // screen reader hears each value (D538 §9).
    const said = [...document.querySelectorAll("dialog output")].find((each) =>
      each.textContent.startsWith("Sound on this device"),
    );
    expect(said?.textContent).toBe("Sound on this device: 150 ms earlier");
    expect(boxOf(said).top).toBeGreaterThanOrEqual(boxOf(earlier).bottom);
  });

  it("reaches ⁝'s sound timing by a remote: Left and Right from its field to − and +, OK steps, Up and Down leave its row (the owner, 2026-11-22)", async () => {
    await page.viewport(960, 480);
    const input = startInput({ goBack: () => false, isNavigating: () => true });
    try {
      await openMore();
      forgetSoundOffsetSetInThisTab();
      let field = soundField();
      const focusAtEnd = (): void => {
        field.focus();
        field.setSelectionRange(field.value.length, field.value.length);
      };
      // In step again, drawn before the caret is put at the start, which a new value would move.
      await expect.poll(() => field.value).toBe("0");
      field.focus();
      field.setSelectionRange(0, 0);
      await userEvent.keyboard("{ArrowLeft}");
      expect(document.activeElement?.getAttribute("aria-label")).toBe("Sound 50 ms earlier");
      await userEvent.keyboard("{ArrowRight}");
      focusAtEnd();
      await userEvent.keyboard("{ArrowRight}");
      expect(document.activeElement?.getAttribute("aria-label")).toBe("Sound 50 ms later");
      await userEvent.keyboard("{Enter}");
      await expect.poll(() => field.value).toBe("50");
      await userEvent.keyboard("{ArrowLeft}");
      expect(document.activeElement).toBe(field);
      // Past the row's end, Right closes ⁝, as it does from any of its places.
      page.getByRole("button", { name: "Sound 50 ms later" }).element().focus();
      await userEvent.keyboard("{ArrowRight}");
      await expect.poll(() => document.querySelector("dialog[open]")).toBeNull();
      await userEvent.click(page.getByRole("button", { name: "Settings" }));
      field = soundField();
      const row = field.closest("fieldset");
      for (const key of ["{ArrowDown}", "{ArrowUp}"]) {
        focusAtEnd();
        await userEvent.keyboard(key);
        const focused = document.activeElement;
        expect(drawnIn("dialog").contains(focused)).toBe(true);
        expect(row?.contains(focused)).toBe(false);
      }
      for (const key of ["{ArrowDown}", "{ArrowUp}"]) {
        page.getByRole("button", { name: "Sound 50 ms later" }).element().focus();
        await userEvent.keyboard(key);
        expect(row?.contains(document.activeElement)).toBe(false);
      }
      expect(field.value).toBe("50");
    } finally {
      input.stop();
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

/** Draws ⁝ for a film with subtitles, as the player's overlay has it, and opens it. */
async function openMore(): Promise<void> {
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
}

/** ⁝'s field of this device's sound timing. */
function soundField(): HTMLInputElement {
  const field = page
    .getByRole("textbox", { name: "Sound on this device, in ms: more is later, less is earlier" })
    .element();
  if (!(field instanceof HTMLInputElement)) {
    throw new Error("no sound timing field");
  }
  return field;
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

/** ⁝ over subtitled tracks whose offset moves as the person steps it, as the player's does. */
function LiveSubtitleOffset(): JSX.Element {
  const [offsetMs, setOffsetMs] = useState(0);
  const tracks: PlayerTracks = {
    ...subtitledTracks,
    subtitles: {
      ...subtitledTracks.subtitles,
      offsetMs,
      shiftOffset: (stepMs) => {
        setOffsetMs((now) => now + stepMs);
      },
    },
  };
  return (
    <PlayerTracksContext value={tracks}>
      <PlayerMenusContext value={ways}>
        <PlayerMenus videoRef={createRef()} shown={["more"]} onOpenChange={() => undefined} />
      </PlayerMenusContext>
    </PlayerTracksContext>
  );
}

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

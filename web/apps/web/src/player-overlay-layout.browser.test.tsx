// SPDX-License-Identifier: MPL-2.0

// The player's overlay laid out and driven as each device drives it (D538 §6; player-spec; brief-
// common's browser tests): a finger on a phone upright and on its side, whose overlay stands
// previous, the steps, play and next in the picture's middle; a mouse, the keyboard and a TV remote
// on a computer's screen and a television's. No row wraps, focus shows its ring and never lands on
// nothing as the overlay hides and shows, and the remote's arrows reach every control.

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { act, createRef } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { page, userEvent } from "vitest/browser";

import { commandEventName, startInput } from "./input-commands.ts";
import { PlayUpwardContext } from "./play-up.tsx";
import { PlayerOverlay } from "./player-overlay.tsx";
import { doubleTapWaitMs } from "./player-overlay-state.ts";
import { mount, type Mounted } from "./test-dom.tsx";
import { focusedName, isRingShown } from "./test-parts.ts";
import { withServices } from "./test-services.tsx";
import type { Upward } from "./upward.ts";

/** An episode's place, for the overlay's way up, which draws the menus a receiver's has not. */
const episodeUpward: Upward = {
  crumbs: [{ label: "A series", to: { screen: "home" }, isIt: () => false }],
  here: "An episode",
};

/** A phone upright and on its side, held by a finger; a computer's screen and a television's. */
const phones = [
  { width: 360, height: 740 },
  { width: 740, height: 360 },
] as const;
const wide = [
  { width: 1280, height: 800 },
  { width: 1920, height: 1080 },
] as const;

/** The size of −10, play and +10 in a phone's centre cluster: a fingertip square and a step more. */
const centrePx = 56;
/** The fingertip square every other control keeps (--size-control). */
const controlPx = 44;

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  vi.restoreAllMocks();
});

/** Makes the device's pointer a finger, as a phone's is, for what asks (pointer: coarse). */
function heldByAFinger(): void {
  const own = window.matchMedia.bind(window);
  vi.spyOn(window, "matchMedia").mockImplementation((query) => {
    const list = own(query);
    if (query === "(pointer: coarse)") {
      Object.defineProperty(list, "matches", { get: () => true });
    }
    return list;
  });
}

/** What the overlay's previous and next were pressed to do, and the film's time they may seek. */
interface Film {
  readonly moved: string[];
  readonly atS: () => number;
}

/** Draws the overlay over a film filling a screen of the size given, previous and next with it. */
async function drawnOn(screen: { readonly width: number; readonly height: number }): Promise<Film> {
  await page.viewport(screen.width, screen.height);
  const moved: string[] = [];
  let atS = 100;
  const video = document.createElement("video");
  Object.defineProperty(video, "paused", { get: () => true });
  Object.defineProperty(video, "duration", { get: () => 600 });
  Object.defineProperty(video, "currentTime", {
    get: () => atS,
    set: (to: number) => {
      atS = to;
    },
  });
  video.style.display = "block";
  video.style.width = "100%";
  video.style.height = "100%";
  const videoRef = createRef<HTMLVideoElement>();
  Reflect.set(videoRef, "current", video);
  mounted = await mount(
    withServices(
      <PlayUpwardContext value={episodeUpward}>
        <div
          className="film-player__stage"
          style={{ position: "relative", width: "100vw", height: "100vh" }}
        >
          <PlayerOverlay
            videoRef={videoRef}
            readLinkKbps={() => undefined}
            failure={undefined}
            moves={{
              previous: () => {
                moved.push("previous");
              },
              next: () => {
                moved.push("next");
              },
            }}
          />
        </div>
      </PlayUpwardContext>,
    ),
  );
  overlay().parentElement?.prepend(video);
  return { moved, atS: () => atS };
}

function overlay(): HTMLElement {
  const found = mounted?.container.querySelector<HTMLElement>(".player-overlay");
  if (found === null || found === undefined) {
    throw new Error("no overlay drawn");
  }
  return found;
}

function button(label: string): HTMLButtonElement {
  const found = overlay().querySelector<HTMLButtonElement>(`button[aria-label="${label}"]`);
  if (found === null) {
    throw new Error(`no ${label} drawn`);
  }
  return found;
}

/** Shows the overlay as a mouse moved over the page does, then waits out its shield (VQ347). */
async function shown(): Promise<void> {
  for (const screenX of [5, 50]) {
    act(() => {
      document.body.dispatchEvent(
        new PointerEvent("pointermove", {
          bubbles: true,
          pointerType: "mouse",
          screenX,
          screenY: 5,
        }),
      );
    });
  }
  await expect.poll(() => overlay().getAttribute("data-shown")).toBe("full");
  await new Promise((done) => setTimeout(done, 600));
}

/** A finger's tap on a button, its touch and then its click, as a phone's browser sends them. */
function tapped(target: HTMLElement): void {
  const box = target.getBoundingClientRect();
  const init = {
    bubbles: true,
    pointerType: "touch",
    pointerId: 7,
    isPrimary: true,
    clientX: box.left + box.width / 2,
    clientY: box.top + box.height / 2,
  };
  act(() => {
    target.dispatchEvent(new PointerEvent("pointerdown", init));
    target.dispatchEvent(new PointerEvent("pointerup", init));
    target.click();
  });
}

/** The buttons of one of the overlay's rows, as drawn. */
function rowButtons(row: string): HTMLButtonElement[] {
  return [...overlay().querySelectorAll<HTMLButtonElement>(`${row} button`)].filter(
    (each) => each.getBoundingClientRect().width > 0,
  );
}

/** Whether every box stands centred on one line, inside the screen's width. */
function isOneRowOnScreen(boxes: readonly DOMRect[]): boolean {
  const middleOf = (box: DOMRect): number => box.top + box.height / 2;
  const middle = boxes[0] === undefined ? 0 : middleOf(boxes[0]);
  return boxes.every(
    (box) =>
      Math.abs(middleOf(box) - middle) < 2 && box.left >= 0 && box.right <= window.innerWidth,
  );
}

describe("the player's overlay on a touch phone (D538 §6, player-spec)", () => {
  for (const screen of phones) {
    const at = `${String(screen.width)}×${String(screen.height)}`;

    it(`stands previous, the steps, play and next in the picture's middle, the rest in one row, at ${at}`, async () => {
      heldByAFinger();
      await drawnOn(screen);
      await shown();
      await expect.poll(() => overlay().getAttribute("data-layout")).toBe("phone");
      const cluster = ["Previous episode", "Back 10 s", "Play", "Forward 10 s", "Next episode"].map(
        (label) => button(label).getBoundingClientRect(),
      );
      expect(isOneRowOnScreen(cluster)).toBe(true);
      const middle = (cluster[2]!.left + cluster[2]!.right) / 2;
      expect(Math.abs(middle - window.innerWidth / 2)).toBeLessThan(2);
      // Midway between the top row and the scrub bar, not the screen's middle: on its side a
      // phone's laid them over the scrub bar (the owner, 2026-11-23).
      const height = (cluster[2]!.top + cluster[2]!.bottom) / 2;
      const topRow = overlay().querySelector(".player-overlay__top")!.getBoundingClientRect();
      const scrubRow = overlay().querySelector(".player-overlay__scrub")!.getBoundingClientRect();
      expect(Math.abs(height - (topRow.bottom + scrubRow.top) / 2)).toBeLessThan(2);
      expect(cluster.slice(1, 4).map((box) => Math.round(box.width))).toEqual([
        centrePx,
        centrePx,
        centrePx,
      ]);
      expect([cluster[0], cluster[4]].map((box) => Math.round(box!.width))).toEqual([
        controlPx,
        controlPx,
      ]);
      const trailing = rowButtons(".player-controls__trailing").map((each) =>
        each.getBoundingClientRect(),
      );
      expect(trailing.map((box) => Math.round(box.width))).toContain(controlPx);
      expect(isOneRowOnScreen(trailing)).toBe(true);
      expect(trailing[0]!.top).toBeGreaterThan(cluster[2]!.bottom);
    });

    it(`waits a moment on a tap of next before it moves, and makes two quick taps a seek, at ${at}`, async () => {
      heldByAFinger();
      const film = await drawnOn(screen);
      await shown();
      await expect.poll(() => overlay().getAttribute("data-layout")).toBe("phone");
      tapped(button("Next episode"));
      expect(film.moved).toEqual([]);
      await expect.poll(() => film.moved).toEqual(["next"]);
      tapped(button("Previous episode"));
      tapped(button("Previous episode"));
      await new Promise((done) => setTimeout(done, doubleTapWaitMs * 2));
      expect(film.moved).toEqual(["next"]);
      expect(film.atS()).toBe(90);
      expect(overlay().getAttribute("data-shown")).toBe("full");
    });
  }

  it("moves at once on a mouse's click of next, on a phone too", async () => {
    heldByAFinger();
    const film = await drawnOn(phones[0]);
    await shown();
    await userEvent.click(button("Next episode"));
    expect(film.moved).toEqual(["next"]);
  });
});

describe("the player's overlay on a computer's screen and a television's (D538 §6)", () => {
  for (const screen of wide) {
    const at = `${String(screen.width)}×${String(screen.height)}`;

    it(`keeps every button in one row along the film's bottom at ${at}`, async () => {
      await drawnOn(screen);
      await shown();
      expect(overlay().hasAttribute("data-layout")).toBe(false);
      const boxes = rowButtons(".player-overlay__buttons").map((each) =>
        each.getBoundingClientRect(),
      );
      expect(boxes.length).toBeGreaterThan(5);
      expect(isOneRowOnScreen(boxes)).toBe(true);
    });

    it(`walks every control by the keyboard, each ringed, and focus lands on something as it hides, at ${at}`, async () => {
      await drawnOn(screen);
      await shown();
      overlay().querySelector<HTMLElement>("a[href], button, input")?.focus();
      const walked = new Set<string>();
      for (let step = 0; step < 20; step += 1) {
        await userEvent.keyboard("{Tab}");
        if (!overlay().contains(document.activeElement)) {
          break;
        }
        expect(isRingShown()).toBe(true);
        walked.add(String(focusedName()));
      }
      for (const label of ["Next episode", "Full screen", "Where the film stands"]) {
        expect(walked).toContain(label);
      }
      // Back hides it, and focus leaves what hid for the film, never for nothing (D411).
      button("Play").focus();
      act(() => {
        document.body.dispatchEvent(
          new CustomEvent(commandEventName, {
            detail: { command: "back", source: "key" },
            bubbles: true,
            cancelable: true,
          }),
        );
      });
      expect(overlay().getAttribute("data-shown")).toBe("hidden");
      expect(document.activeElement).not.toBe(document.body);
      expect(document.activeElement?.tagName).toBe("VIDEO");
    });

    it(`reaches every button of its row and the scrub bar by a remote's arrows, and OK presses one, at ${at}`, async () => {
      const film = await drawnOn(screen);
      const input = startInput({ goBack: () => false, isNavigating: () => true });
      try {
        await shown();
        button("Play").focus();
        const reached = new Set<string>();
        const walkRow = async (): Promise<void> => {
          for (let step = 0; step < 12; step += 1) {
            await userEvent.keyboard("{ArrowLeft}");
          }
          for (let step = 0; step < 16; step += 1) {
            expect(document.activeElement).not.toBe(document.body);
            reached.add(String(focusedName()));
            await userEvent.keyboard("{ArrowRight}");
          }
        };
        await walkRow();
        const buttons = rowButtons(".player-overlay__buttons").map((each) =>
          each.getAttribute("aria-label"),
        );
        for (const label of buttons) {
          expect(reached).toContain(String(label));
        }
        await userEvent.keyboard("{ArrowUp}");
        expect(focusedName()).toBe("Where the film stands");
        expect(isRingShown()).toBe(true);
        await userEvent.keyboard("{ArrowDown}");
        expect(
          overlay().querySelector(".player-overlay__buttons")?.contains(document.activeElement),
        ).toBe(true);
        button("Next episode").focus();
        await userEvent.keyboard("{Enter}");
        expect(film.moved).toEqual(["next"]);
      } finally {
        input.stop();
      }
    });
  }
});

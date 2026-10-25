// SPDX-License-Identifier: MPL-2.0

/**
 * The app walked with a remote's keys and a controller in a real browser (controller-input-2026-10-25.md
 * §6.3), at a Fire TV's page in Silk, 960 × 480 CSS pixels below its address bar, and at a desktop's:
 * each page arrived at, its controls arrowed through, one selected, Back pressed, and focus where it
 * was; the shell's banner reached by Up from the page's top.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import {
  filmDetail,
  films,
  focusedTile,
  isDrawn,
  isFocusIn,
  isFocusInView,
  mountApp,
  viewer,
  walkServices,
  type Walking,
  type WalkAnswers,
} from "./test-walk.tsx";

/** The screens the walks are made at: a Fire TV's page in Silk, and a desktop's window. */
const screens = [
  { name: "a Fire TV's page in Silk", width: 960, height: 480 },
  { name: "a desktop's window", width: 1280, height: 800 },
] as const;

/** The server's answers every signed-in page reads. */
const signedIn: WalkAnswers = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /home": { continueWatching: [] },
};

/** The films list of twelve, and Film 3's page. */
const filmsAndOne: WalkAnswers = {
  ...signedIn,
  "GET /titles": { titles: films(12), nextCursor: "" },
  "GET /items/103": filmDetail(103, "Film 3"),
  "GET /items/102": filmDetail(102, "Film 2"),
};

describe.each(screens)("the app by a remote's keys, on $name", ({ width, height }) => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
  });

  it("starts in the page, not the banner, moves across the films, opens one and goes back to it", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices(filmsAndOne));
    await expect.poll(() => isDrawn(".poster-grid__card")).toBe(true);
    await userEvent.keyboard("{ArrowDown}");
    // The list's order, at the bar's end, then the film below it, then along its row to the first.
    await expect.poll(() => document.activeElement?.tagName).toBe("SELECT");
    await userEvent.keyboard("{ArrowDown}");
    expect(focusedTile()).not.toBe("");
    await userEvent.keyboard("{ArrowLeft}{ArrowLeft}{ArrowLeft}{ArrowLeft}{ArrowLeft}{ArrowLeft}");
    await expect.poll(focusedTile).toBe("Film 1");
    await userEvent.keyboard("{ArrowRight}{ArrowRight}");
    await expect.poll(focusedTile).toBe("Film 3");
    await userEvent.keyboard("{Enter}");
    await expect.poll(() => isDrawn(".title")).toBe(true);
    expect(document.querySelector("h1")?.textContent).toBe("Film 3");
    await userEvent.keyboard("{Escape}");
    await expect.poll(() => isDrawn(".poster-grid__card")).toBe(true);
    await expect.poll(focusedTile).toBe("Film 3");
    expect(isFocusInView()).toBe(true);
  });

  it("reaches the banner by Up from the page's top, and comes back down to where it left", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices(filmsAndOne));
    await expect.poll(() => isDrawn(".poster-grid__card")).toBe(true);
    await userEvent.keyboard(
      "{ArrowDown}{ArrowDown}{ArrowLeft}{ArrowLeft}{ArrowLeft}{ArrowLeft}{ArrowLeft}",
    );
    await userEvent.keyboard("{ArrowRight}");
    await expect.poll(focusedTile).toBe("Film 2");
    await userEvent.keyboard("{ArrowUp}");
    expect(isFocusIn(".app-shell__banner")).toBe(true);
    expect(isFocusInView()).toBe(true);
    // Back into the page at the film it left, not the nearest control below the banner.
    await userEvent.keyboard("{ArrowDown}");
    await expect.poll(focusedTile).toBe("Film 2");
  });

  it("keeps the focused film in view as it moves down a long list, clear of the banner", async () => {
    await page.viewport(width, height);
    walking = await mountApp(
      walkServices({ ...filmsAndOne, "GET /titles": { titles: films(40), nextCursor: "" } }),
    );
    await expect.poll(() => isDrawn(".poster-grid__card")).toBe(true);
    await userEvent.keyboard("{ArrowDown}{ArrowDown}");
    for (let step = 0; step < 6; step += 1) {
      await userEvent.keyboard("{ArrowDown}");
      expect(isFocusInView()).toBe(true);
      const banner = document.querySelector(".app-shell__banner")?.getBoundingClientRect();
      const box = document.activeElement?.getBoundingClientRect();
      expect(box?.top).toBeGreaterThanOrEqual(banner?.bottom ?? 0);
    }
    expect(focusedTile()).toMatch(/^Film \d+$/u);
  });
});

/** A standard pad as navigator.getGamepads gives it, its buttons held as the test says. */
interface HeldPad {
  readonly pad: Pick<Gamepad, "index" | "id" | "mapping" | "axes"> & {
    readonly buttons: readonly Pick<GamepadButton, "pressed">[];
  };
  /** Holds button down, or lets every button go. */
  readonly hold: (button: number | undefined) => void;
}

/** An Xbox pad in the standard mapping, at rest. */
function xboxPad(): HeldPad {
  let held: number | undefined;
  return {
    pad: {
      index: 0,
      id: "Xbox 360 Controller (XInput STANDARD GAMEPAD)",
      mapping: "standard",
      axes: [0, 0, 0, 0],
      get buttons() {
        return Array.from({ length: 17 }, (_, index) => ({ pressed: index === held }));
      },
    },
    hold: (button) => {
      held = button;
    },
  };
}

/** Presses pad's button, held over a few frames as a person's press is, then let go. */
async function pressPad(pad: HeldPad, button: number): Promise<void> {
  pad.hold(button);
  await new Promise((resolve) => setTimeout(resolve, 50));
  pad.hold(undefined);
  await new Promise((resolve) => setTimeout(resolve, 50));
}

/** The D-pad's and face buttons' numbers in the standard mapping. */
const padButtons = { ok: 0, back: 1, up: 12, down: 13, left: 14, right: 15 } as const;

describe.each(screens)("the app by a game controller, on $name", ({ width, height }) => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
    Reflect.deleteProperty(navigator, "getGamepads");
  });

  it("moves focus by its D-pad, shown as a key's is, opens a film by OK and goes back to it by B", async () => {
    const pad = xboxPad();
    Object.defineProperty(navigator, "getGamepads", { configurable: true, value: () => [pad.pad] });
    await page.viewport(width, height);
    walking = await mountApp(walkServices(filmsAndOne));
    await expect.poll(() => isDrawn(".poster-grid__card")).toBe(true);
    globalThis.dispatchEvent(new Event("gamepadconnected"));
    await pressPad(pad, padButtons.down);
    await pressPad(pad, padButtons.down);
    expect(focusedTile()).not.toBe("");
    // A gamepad's press is no key: the browser shows the focus only because the app asks it to.
    expect(document.activeElement?.matches(":focus-visible")).toBe(true);
    for (let step = 0; step < 6; step += 1) {
      await pressPad(pad, padButtons.left);
    }
    await pressPad(pad, padButtons.right);
    await expect.poll(focusedTile).toBe("Film 2");
    await pressPad(pad, padButtons.ok);
    await expect.poll(() => isDrawn(".title")).toBe(true);
    await pressPad(pad, padButtons.back);
    await expect.poll(() => isDrawn(".poster-grid__card")).toBe(true);
    await expect.poll(focusedTile).toBe("Film 2");
    expect(document.activeElement?.matches(":focus-visible")).toBe(true);
  });
});

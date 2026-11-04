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

import { apiPrefix } from "@orpheum/sdk";
import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { createNavigation } from "./navigation.ts";
import type { Services } from "./services.tsx";
import { fakePlace, testServices } from "./test-services.tsx";
import {
  filmDetail,
  films,
  focusedTile,
  isDrawn,
  isFocusIn,
  isFocusOnFirst,
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
    // The page opens with focus on its first film, its arrival (ui-audit-2026-11-04.md R3).
    await expect.poll(() => isDrawn(".poster-grid .poster-card__link")).toBe(true);
    await expect.poll(() => isFocusOnFirst(".poster-grid .poster-card__link")).toBe(true);
    expect(focusedTile()).toBe("Film 1");
    await userEvent.keyboard("{ArrowRight}{ArrowRight}");
    await expect.poll(focusedTile).toBe("Film 3");
    await userEvent.keyboard("{Enter}");
    await expect.poll(() => isDrawn(".title")).toBe(true);
    expect(document.querySelector("h1")?.textContent).toBe("Film 3");
    await userEvent.keyboard("{Escape}");
    await expect.poll(() => isDrawn(".poster-grid .poster-card__link")).toBe(true);
    await expect.poll(focusedTile).toBe("Film 3");
    expect(isFocusInView()).toBe(true);
  });

  it("reaches the banner by Up from the page's top, past the list's tools, and comes back down to where it left", async () => {
    await page.viewport(width, height);
    walking = await mountApp(walkServices(filmsAndOne));
    await expect.poll(() => isDrawn(".poster-grid .poster-card__link")).toBe(true);
    await expect.poll(() => isFocusOnFirst(".poster-grid .poster-card__link")).toBe(true);
    await userEvent.keyboard("{ArrowRight}");
    await expect.poll(focusedTile).toBe("Film 2");
    // Up stays in the page before the banner: the list's tools, though not straight above the film
    // (the owner, 2026-11-04).
    await userEvent.keyboard("{ArrowUp}");
    expect(isFocusIn("main .browse__bar")).toBe(true);
    await userEvent.keyboard("{ArrowUp}");
    expect(isFocusIn(".app-shell__banner")).toBe(true);
    expect(isFocusInView()).toBe(true);
    // Back into the page at the tool it left, not the nearest control below the banner, and on
    // down to the film.
    await userEvent.keyboard("{ArrowDown}");
    expect(isFocusIn("main .browse__bar")).toBe(true);
    await userEvent.keyboard("{ArrowDown}");
    await expect.poll(focusedTile).toBe("Film 2");
  });

  it("keeps the focused film in view as it moves down a long list, clear of the banner", async () => {
    await page.viewport(width, height);
    // Sixty films: ten rows at the Fire TV's width, so eight presses from the first film stay in the
    // list; forty ended at the seventh row, and a press past it reached the letters beside the list,
    // where focus is on no film.
    walking = await mountApp(
      walkServices({ ...filmsAndOne, "GET /titles": { titles: films(60), nextCursor: "" } }),
    );
    await expect.poll(() => isDrawn(".poster-grid .poster-card__link")).toBe(true);
    // The first press waits for the page's arrival, on the first film: pressed before it, the arrow
    // from no focus only goes to that film (focus-navigation.ts startIn), one press short, so the
    // walk's end depended on whether the arrival came first.
    await expect.poll(() => isFocusOnFirst(".poster-grid .poster-card__link")).toBe(true);
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

  // The owner's Fire TV, 2026-10-29: "Pressing back from the Gus Doyle page brought me back to the
  // top of the series list. It should bring me back to the point in the list where Gus Doyle is,
  // still highlighted." Silk's Back is a step of the history, and the list drew its first page alone.
  it("goes back by the browser's own Back to a series on the list's second page, focused and in view", async () => {
    await page.viewport(width, height);
    const place = fakePlace("/series");
    walking = await mountApp({
      ...pagedSeriesServices(70),
      navigation: createNavigation(place, false),
    });
    await expect.poll(() => isDrawn(".poster-grid .poster-card__link")).toBe(true);
    const tileOf65 = (): HTMLElement | null =>
      document.querySelector<HTMLElement>('.poster-grid a[href="/series/265"]');
    for (let tries = 0; tries < 20 && tileOf65() === null; tries += 1) {
      window.scrollTo({ top: document.documentElement.scrollHeight });
      await new Promise((settled) => setTimeout(settled, 50));
    }
    tileOf65()?.focus();
    await expect.poll(focusedTile).toBe("Series 65");
    await userEvent.keyboard("{Enter}");
    await expect.poll(() => document.querySelector("h1")?.textContent).toBe("Series 65");
    place.history.back();
    await expect.poll(focusedTile).toBe("Series 65");
    // In view once the rows around it, laid out at an estimate until near (browse.css), took their
    // own height: in Firefox the tile first scrolled to then moved some 90 pixels below the screen.
    await expect.poll(isFocusInView).toBe(true);
    await new Promise((settled) => requestAnimationFrame(settled));
    expect(isFocusInView()).toBe(true);
  });
});

/**
 * The signed-in app's services, its Series list count series long, Series 1 to Series count with ids
 * from 201, sent sixty a page by the cursor asked for, and each series' page.
 */
function pagedSeriesServices(count: number): Services {
  const all = Array.from({ length: count }, (_, index) => ({
    id: 201 + index,
    kind: "series",
    name: `Series ${String(index + 1)}`,
    year: 2000,
  }));
  const pageSize = 60;
  return testServices("token", "/series", async (request) => {
    await Promise.resolve();
    const url = new URL(request.url);
    const path = url.pathname.slice(apiPrefix.length);
    if (request.method === "GET" && path === "/titles") {
      const from = Number(url.searchParams.get("cursor") ?? "0");
      const next = from + pageSize < count ? String(from + pageSize) : "";
      return Response.json({ titles: all.slice(from, from + pageSize), nextCursor: next });
    }
    const series = /^\/collections\/(\d+)$/u.exec(path);
    if (request.method === "GET" && series !== null) {
      const id = Number(series[1]);
      return Response.json({
        id,
        kind: "series",
        name: `Series ${String(id - 200)}`,
        libraryId: 1,
        art: {},
        seasons: [{ seasonNumber: 1, episodeCount: 0 }],
      });
    }
    const answers: WalkAnswers = signedIn;
    const key = `${request.method} ${path}`;
    return key in answers
      ? Response.json(answers[key])
      : Response.json({ error: { code: "not_found", message: "no such thing" } }, { status: 404 });
  });
}

/** A standard pad as navigator.getGamepads gives it, its buttons held as the test says. */
interface HeldPad {
  readonly pad: Pick<Gamepad, "index" | "id" | "mapping" | "axes"> & {
    readonly buttons: readonly Pick<GamepadButton, "pressed">[];
  };
  /** Holds button down, or lets every button go. */
  readonly hold: (button: number | undefined) => void;
  /** How many times the page has read the pad's buttons. */
  readonly reads: () => number;
}

/** An Xbox pad in the standard mapping, at rest. */
function xboxPad(): HeldPad {
  let held: number | undefined;
  let reads = 0;
  return {
    pad: {
      index: 0,
      id: "Xbox 360 Controller (XInput STANDARD GAMEPAD)",
      mapping: "standard",
      axes: [0, 0, 0, 0],
      get buttons() {
        reads += 1;
        return Array.from({ length: 17 }, (_, index) => ({ pressed: index === held }));
      },
    },
    hold: (button) => {
      held = button;
    },
    reads: () => reads,
  };
}

/** Waits until the page has read pad again, as it does once a frame. */
async function untilRead(pad: HeldPad): Promise<void> {
  const before = pad.reads();
  await expect.poll(() => pad.reads() > before).toBe(true);
}

/**
 * Presses pad's button, held until the page has read it held, as a person's press is over a few
 * frames, then let go until the page has read it go. A press held for a fixed 50 ms was never read
 * when a loaded machine drew no frame in it (2026-10-25).
 */
async function pressPad(pad: HeldPad, button: number): Promise<void> {
  pad.hold(button);
  await untilRead(pad);
  pad.hold(undefined);
  await untilRead(pad);
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
    await expect.poll(() => isDrawn(".poster-grid .poster-card__link")).toBe(true);
    await expect.poll(() => isFocusOnFirst(".poster-grid .poster-card__link")).toBe(true);
    expect(focusedTile()).toBe("Film 1");
    globalThis.dispatchEvent(new Event("gamepadconnected"));
    await pressPad(pad, padButtons.right);
    await expect.poll(focusedTile).toBe("Film 2");
    // A gamepad's press is no key: the browser shows the focus only because the app asks it to.
    expect(document.activeElement?.matches(":focus-visible")).toBe(true);
    await pressPad(pad, padButtons.left);
    await expect.poll(focusedTile).toBe("Film 1");
    await pressPad(pad, padButtons.right);
    await expect.poll(focusedTile).toBe("Film 2");
    await pressPad(pad, padButtons.ok);
    await expect.poll(() => isDrawn(".title")).toBe(true);
    await pressPad(pad, padButtons.back);
    await expect.poll(() => isDrawn(".poster-grid .poster-card__link")).toBe(true);
    await expect.poll(focusedTile).toBe("Film 2");
    expect(document.activeElement?.matches(":focus-visible")).toBe(true);
  });

  // The Films page's sort is the app's own drop-down (the owner, 2026-10-28), not a select.
  it("chooses the Films page's sort by a controller, its OK opening the list and choosing in it", async () => {
    const pad = xboxPad();
    Object.defineProperty(navigator, "getGamepads", { configurable: true, value: () => [pad.pad] });
    const heard: string[] = [];
    await page.viewport(width, height);
    walking = await mountApp(walkServices(filmsAndOne, "/films", heard));
    // The page opens with focus on its first film, its arrival (ui-audit-2026-11-04.md R3); the
    // sort is above the row's last films.
    await expect.poll(() => isDrawn(".poster-grid .poster-card__link")).toBe(true);
    await expect.poll(() => isFocusOnFirst(".poster-grid .poster-card__link")).toBe(true);
    globalThis.dispatchEvent(new Event("gamepadconnected"));
    // Along the first row to its last film, short of the alphabet's rail beside the grid.
    const posters = [...document.querySelectorAll<HTMLElement>(".poster-grid .poster-card__link")];
    const topOf = (element: Element | undefined): number | undefined =>
      element?.getBoundingClientRect().top;
    const firstRow = posters.filter((each) => topOf(each) === topOf(posters[0]));
    for (let step = 1; step < firstRow.length; step += 1) {
      await pressPad(pad, padButtons.right);
    }
    await expect.poll(() => document.activeElement).toBe(firstRow.at(-1));
    await pressPad(pad, padButtons.up);
    // The bar's tools, the Sort and filter at their right end (the owner, 2026-10-30; D417).
    expect(document.activeElement?.closest(".listing-tools")).not.toBeNull();
    if (document.activeElement?.classList.contains("list-criteria__button") !== true) {
      await pressPad(pad, padButtons.right);
    }
    expect(document.activeElement?.classList.contains("list-criteria__button")).toBe(true);
    const criteria = document.activeElement;
    await pressPad(pad, padButtons.ok);
    // Its pop-up opens on its field; the sort is a chooser further down it.
    await expect
      .poll(() => document.activeElement?.classList.contains("list-criteria__name"))
      .toBe(true);
    // The sort's chooser, found by the field its button is described by.
    const sortChooser = (): Element | undefined =>
      [...document.querySelectorAll(".list-criteria .route-chooser")]
        .find((each) => each.querySelector(".route-chooser__field")?.textContent === "Sort")
        ?.querySelector(".route-chooser__button") ?? undefined;
    const isOnSort = (): boolean =>
      document.activeElement !== null && document.activeElement === sortChooser();
    for (let step = 0; step < 6 && !isOnSort(); step += 1) {
      await pressPad(pad, padButtons.down);
    }
    expect(isOnSort()).toBe(true);
    await pressPad(pad, padButtons.ok);
    await expect.poll(() => document.activeElement?.getAttribute("aria-label")).toBe("Name, A–Z");
    expect(document.querySelectorAll("dialog")).toHaveLength(2);
    await pressPad(pad, padButtons.down);
    await pressPad(pad, padButtons.ok);
    // The next choice after Name, A–Z is Name, Z–A (D366 §2): the sort's chooser says it, its list
    // closed alone, and B closes the pop-up, its button saying it too.
    await expect.poll(() => sortChooser()?.textContent).toBe("Name, Z–A");
    expect(document.querySelectorAll("dialog")).toHaveLength(1);
    await pressPad(pad, padButtons.back);
    await expect.poll(() => document.querySelector("dialog")).toBeNull();
    expect(document.activeElement).toBe(criteria);
    expect(criteria?.getAttribute("aria-label")).toBe("Sort and filter: Name, Z–A");
    await expect
      .poll(() => heard.filter((each) => each === "GET /titles").length)
      .toBeGreaterThan(1);
  });
});

// SPDX-License-Identifier: MPL-2.0

/**
 * Where focus is as a page or a dialog opens, its arrival (ui-audit-2026-11-04.md R3; focus-navigation.ts
 * arriveWhenDrawn): on what the page is for, so the first press acts on it, in a real browser, as
 * the app draws it from the server's answers; and shown with no ring where a pointer opened it. As
 * the app opens, before any press, focus is on the top bar's Home mark instead (the owner,
 * 2026-11-26; focus-navigation.ts openingArrivalIn), so a page's own arrival is a page opened after.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";
import { page, userEvent } from "vitest/browser";

import { createSession } from "./session.ts";
import { directDecision } from "./test-fixtures.ts";
import { fakeTogetherServer } from "./test-together.ts";
import {
  filmDetail,
  films,
  focusedName,
  isDrawn,
  isFocusIn,
  isFocusOnFirst,
  mountAfterTheOpening,
  mountApp,
  viewer,
  walkServices,
  type Walking,
  type WalkAnswers,
} from "./test-walk.tsx";

/**
 * The server's answers every signed-in page reads: Home's rows in its one read, the newest films six
 * (D476 §6), and the films' list whole in one answer (D475).
 */
const signedIn: WalkAnswers = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /home": { continueWatching: [], newestFilms: films(6), newestSeries: [] },
  "GET /items": { items: films(6) },
};

/** An administrator's answers, for the pages only they reach: Libraries, Users, a library's files. */
const anAdmin: WalkAnswers = {
  ...signedIn,
  "GET /me": { ...viewer, role: "admin", isAdmin: true },
  "GET /libraries": {
    libraries: [
      {
        id: 1,
        name: "Films",
        kind: "film",
        metadataSource: "nfoThenTmdb",
        changeDetection: "watch",
        isWritable: true,
        createdAt: 0,
      },
    ],
  },
  "GET /users": {
    users: [
      { id: 1, name: "Karl", role: "admin", hasPassword: true },
      { id: 2, name: "Dana", role: "viewer", hasPassword: true },
    ],
  },
  "GET /invitations": { invitations: [] },
  "GET /files": {
    files: [
      {
        id: 7,
        libraryId: 1,
        path: "/media/7.mkv",
        filing: "skipped",
        filingReason: "extension",
        firstSeenAt: 1,
        lastSeenAt: 1,
      },
    ],
  },
};

/** A device of the person's, as GET /me/devices lists it. */
function device(id: number, name: string, isThisDevice = false): Record<string, unknown> {
  return {
    id,
    name,
    platform: "web",
    addedAt: 0,
    lastUsedAt: 0,
    isSignedIn: true,
    isByCode: false,
    isThisDevice,
  };
}

/** What has focus, as a failure shows it. */
function focused(): string {
  return document.activeElement?.outerHTML.slice(0, 120) ?? "nothing";
}

/** What the page's live region last said, for a screen reader alone. */
function announced(): string {
  const region = document.querySelector("output.input-layer__announced");
  // A polite live region, so the reader waits for what it is saying.
  return region?.getAttribute("aria-live") === "polite" ? region.textContent : "";
}

/** Whether the top bar's Home mark has focus: where it starts as the app opens. */
function isOnHomeMark(): boolean {
  return document.activeElement?.matches("header .app-shell__home") === true;
}

/** Waits until what has focus is inside what selector names. */
async function arrivesIn(selector: string): Promise<void> {
  await expect.poll(() => isFocusIn(selector), { message: focused() }).toBe(true);
}

describe("a page's arrival", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
  });

  // First, so a Cast dialog it left open would hold the focus of every page after it, as one did
  // before test-walk.tsx's mountApp closed the tray's dialog (tray-dialog.ts keeps it for the tab).
  it("is Cast's first device, once its list is read, not its Close", async () => {
    await page.viewport(1280, 800);
    const server = fakeTogetherServer();
    walking = await mountApp({
      ...walkServices({
        ...signedIn,
        "GET /me/devices": {
          devices: [device(1, "Laptop", true), device(2, "Fire TV"), device(3, "Pixel")],
        },
      }),
      openTogetherSocket: server.openSocket,
    });
    // The live connection opens once the page knows who is signed in (D469 §4).
    await expect.poll(() => server.openedCount()).toBeGreaterThan(0);
    await server.accept();
    await server.say({ type: "devices", deviceIds: [1, 2, 3], inFrontDeviceIds: [1, 2, 3] });
    await expect.poll(() => isDrawn('button[aria-label="Cast to a device"]')).toBe(true);
    document.querySelector<HTMLElement>('button[aria-label="Cast to a device"]')?.focus();
    await userEvent.keyboard("{Enter}");
    await expect.poll(() => isDrawn("dialog")).toBe(true);
    // The devices are listed once the live connection says what this device casts: nothing.
    await server.say({ type: "sending" });
    await expect.poll(() => isDrawn("dialog .cast-to__devices button")).toBe(true);
    await expect.poll(() => isFocusOnFirst("dialog .cast-to__devices button")).toBe(true);
    expect(focusedName()).toContain("Fire TV");
  });

  it("is a list's first poster", async () => {
    await page.viewport(1280, 800);
    walking = await mountAfterTheOpening(walkServices(signedIn, "/films"));
    await expect.poll(() => isFocusOnFirst(".poster-grid .poster-card__link")).toBe(true);
  });

  // ui-audit-2026-11-04.md R4: a screen reader is told where it now is, politely, while focus stays
  // on the page's arrival rather than on its heading.
  it("stays off the heading, which is announced, on a page opened and on one Back returns to", async () => {
    await page.viewport(1280, 800);
    walking = await mountAfterTheOpening(
      walkServices({ ...signedIn, "GET /items/101": filmDetail(101, "Film 1") }, "/films"),
    );
    await expect.poll(() => isFocusOnFirst(".poster-grid .poster-card__link")).toBe(true);
    const listName = document.querySelector("main h1")?.textContent ?? "";
    expect(listName).not.toBe("");
    await userEvent.keyboard("{Enter}");
    await arrivesIn(".title-head__actions");
    await expect.poll(announced).toBe("Film 1");
    expect(document.activeElement?.closest("h1")).toBeNull();
    await userEvent.keyboard("{Escape}");
    await expect.poll(announced).toBe(listName);
    // Back gives focus to the poster it left (focus-memory.ts), not the arrival.
    await expect.poll(() => isFocusOnFirst(".poster-grid .poster-card__link")).toBe(true);
  });

  it("shows no ring on a page a pointer opened", async () => {
    // A window the runner's own holds: at 1280 × 800 Playwright never found See all "visible,
    // enabled and stable" to press, though its box held still.
    await page.viewport(800, 600);
    walking = await mountApp(walkServices(signedIn, "/"));
    await expect.poll(() => isDrawn('main a[href="/titles/101"]')).toBe(true);
    await expect.poll(isOnHomeMark, { message: focused() }).toBe(true);
    // Home's newest films' See all, pressed by a real pointer.
    const seeAll = document.querySelector<HTMLElement>('main a[href^="/films"]');
    expect(seeAll).not.toBeNull();
    if (seeAll !== null) {
      await userEvent.click(seeAll);
    }
    await expect.poll(() => isFocusOnFirst(".poster-grid .poster-card__link")).toBe(true);
    // Focused, so the first key acts on it, but with no ring for a person using a mouse (Q6).
    expect(document.activeElement?.matches(":focus-visible")).toBe(false);
  });

  // ui-audit-2026-11-04.md §3: the actions row's Play, so the remote's first OK plays.
  it("is a film's Play, first in its actions row", async () => {
    await page.viewport(1280, 800);
    walking = await mountAfterTheOpening(
      walkServices({ ...signedIn, "GET /items/103": filmDetail(103, "Film 3") }, "/titles/103"),
    );
    await arrivesIn(".title-head__actions");
    expect(focusedName()).toBe("Play");
  });

  it("is a series' next episode", async () => {
    await page.viewport(1280, 800);
    walking = await mountAfterTheOpening(
      walkServices(
        {
          ...signedIn,
          "GET /items/20": {
            id: 20,
            kind: "series",
            name: "Glass Tide",
            libraryId: 1,
            art: {},
            orderings: [{ name: "aired", kind: "aired", episodeCount: 2 }],
            seasons: [{ seasonNumber: 1, episodeCount: 2 }],
            nextUp: {
              itemId: 302,
              name: "Low Water",
              groupNumber: 1,
              numberInGroup: 2,
              fileId: 3020,
            },
          },
        },
        "/series/20",
      ),
    );
    await arrivesIn('main [data-focus-start="arrival"]');
    expect(focusedName()).toBe("Play S1E2");
  });

  it("is Home's Continue watching, where it has anything, before the newest films", async () => {
    await page.viewport(1280, 800);
    walking = await mountAfterTheOpening(
      walkServices(
        {
          ...signedIn,
          "GET /home": {
            continueWatching: [
              {
                id: 7,
                name: "Metropolis",
                libraryId: 1,
                fileId: 70,
                positionMs: 250,
                durationMs: 1000,
              },
            ],
            newestFilms: films(6),
            newestSeries: [],
          },
        },
        "/",
      ),
    );
    await expect.poll(() => isDrawn('main a[href="/titles/101"]')).toBe(true);
    await expect.poll(focusedName, { message: focused() }).toContain("Metropolis");
  });

  it("is Home's newest films where nothing is being watched", async () => {
    await page.viewport(1280, 800);
    walking = await mountAfterTheOpening(walkServices(signedIn, "/"));
    await expect
      .poll(() => document.activeElement?.getAttribute("href"), { message: focused() })
      .toBe("/titles/101");
  });

  it("is This device's Rename", async () => {
    await page.viewport(1280, 800);
    walking = await mountAfterTheOpening(
      walkServices(
        { ...signedIn, "GET /me/devices": { devices: [device(5, "Karl's laptop", true)] } },
        "/this-device",
      ),
    );
    await expect.poll(focusedName, { message: focused() }).toBe("Rename");
  });

  // A text field, its first keys typing into it: its own arrival at the app's opening too.
  it("is Your account's code field", async () => {
    await page.viewport(1280, 800);
    walking = await mountApp(walkServices(signedIn, "/account"));
    await expect
      .poll(() => document.activeElement?.classList.contains("device-activation__code-field"), {
        message: focused(),
      })
      .toBe(true);
  });

  it("is the Libraries page's first library", async () => {
    await page.viewport(1280, 800);
    walking = await mountAfterTheOpening(walkServices(anAdmin, "/libraries"));
    await arrivesIn(".libraries__list");
    expect(focusedName()).toBe("Manage this library");
  });

  it("is the People page's first person", async () => {
    await page.viewport(1280, 800);
    walking = await mountAfterTheOpening(walkServices(anAdmin, "/users"));
    await arrivesIn('[aria-labelledby="people-accounts"] .grid-list');
    expect(focusedName()).toContain("Karl");
  });

  it("is the first of the files a library's scanner saw", async () => {
    await page.viewport(1280, 800);
    walking = await mountAfterTheOpening(walkServices(anAdmin, "/libraries/1/files"));
    await arrivesIn("details");
  });

  it("is the sign-in form's first empty field", async () => {
    await page.viewport(1280, 800);
    const services = walkServices({ "GET /client-settings": {} });
    walking = await mountApp({ ...services, session: createSession(undefined) });
    await expect
      .poll(() => document.activeElement?.getAttribute("name"), { message: focused() })
      .toBe("name");
  });
});

/** A Fire TV's Silk, as the device names itself: a television's browser. */
const fireTvSilk =
  "Mozilla/5.0 (Linux; Android 9; AFTSSS) AppleWebKit/537.36 (KHTML, like Gecko) Silk/138.19.9 like Chrome/138.0.7204.244 Safari/537.36";

/** Home with one film being watched, its file 70, and what opening that file to play answers. */
const watchingOne: WalkAnswers = {
  ...signedIn,
  "GET /home": {
    continueWatching: [
      { id: 7, name: "Metropolis", libraryId: 1, fileId: 70, positionMs: 250, durationMs: 1000 },
    ],
    newestFilms: films(6),
    newestSeries: [],
  },
  "GET /items/7": filmDetail(7, "Metropolis"),
  "POST /playback": directDecision({
    titleName: "Metropolis",
    plays: { itemId: 7, kind: "film", libraryId: 1 },
    url: "/no-such-media",
  }),
};

/** Whether element draws the focus ring (tokens.css): an outline or the ring's shadow. */
function drawsRing(element: Element | null): boolean {
  if (element === null) {
    return false;
  }
  const style = getComputedStyle(element);
  return style.outlineStyle !== "none" || style.boxShadow !== "none";
}

/**
 * Puts the browser where a page's first load leaves it, drawing a ring for a script's focus, whatever
 * an earlier test's pointer left: a key pressed before the app starts, which the app's input never
 * hears, so no key has pressed for it.
 */
async function asAtFirstLoad(): Promise<void> {
  await userEvent.keyboard("{Shift}");
}

// The owner, Firefox on a Mac with a mouse, 2026-11-26: Home's first Continue watching poster drew
// the ring as the page loaded, nothing pressed, and the Play page it opened ringed the whole stage;
// then: "I'd like the home icon on the top bar to start out highlighted on TVs, and selected but not
// showing the ring on desktop. All keyboard movement happens from there and begins highlighting."
describe("focus a script gives before any key or controller has pressed", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
    vi.restoreAllMocks();
  });

  it("is the top bar's Home mark at the app's opening, focused with no ring on a computer, Tab moving on with the ring", async () => {
    await page.viewport(1280, 800);
    await asAtFirstLoad();
    walking = await mountApp(walkServices(watchingOne, "/"));
    await expect.poll(() => isDrawn(".continue-watching .poster-card__link")).toBe(true);
    await expect.poll(isOnHomeMark, { message: focused() }).toBe(true);
    expect(document.activeElement?.matches(":focus-visible")).toBe(false);
    expect(drawsRing(document.activeElement)).toBe(false);
    const mark = document.activeElement;
    await userEvent.keyboard("{Tab}");
    await expect.poll(() => document.activeElement !== mark).toBe(true);
    expect(drawsRing(document.activeElement)).toBe(true);
  });

  it("shows the ring where the first arrow from the unringed Home mark lands", async () => {
    await page.viewport(1280, 800);
    await asAtFirstLoad();
    walking = await mountApp(walkServices(watchingOne, "/"));
    await expect.poll(() => isDrawn(".continue-watching .poster-card__link")).toBe(true);
    await expect.poll(isOnHomeMark, { message: focused() }).toBe(true);
    const mark = document.activeElement;
    await userEvent.keyboard("{ArrowRight}");
    await expect.poll(() => document.activeElement !== mark, { message: focused() }).toBe(true);
    expect(drawsRing(document.activeElement)).toBe(true);
  });

  it("shows the ring on the unringed Home mark where the first arrow cannot move from it", async () => {
    await page.viewport(1280, 800);
    await asAtFirstLoad();
    walking = await mountApp(walkServices(watchingOne, "/"));
    await expect.poll(() => isDrawn(".continue-watching .poster-card__link")).toBe(true);
    await expect.poll(isOnHomeMark, { message: focused() }).toBe(true);
    expect(drawsRing(document.activeElement)).toBe(false);
    // Nothing lies left of the mark, the top bar's first place.
    await userEvent.keyboard("{ArrowLeft}");
    await expect.poll(() => drawsRing(document.activeElement), { message: focused() }).toBe(true);
    expect(isOnHomeMark()).toBe(true);
  });

  it("opens Home by Enter on the unringed Home mark", async () => {
    await page.viewport(1280, 800);
    await asAtFirstLoad();
    walking = await mountApp(walkServices(signedIn, "/films"));
    await expect.poll(isOnHomeMark, { message: focused() }).toBe(true);
    expect(drawsRing(document.activeElement)).toBe(false);
    await userEvent.keyboard("{Enter}");
    await expect.poll(() => walking?.services.navigation.route().screen).toBe("home");
  });

  it("is the top bar's Home mark at the app's opening, ringed on a television, Down from it reaching the page", async () => {
    vi.spyOn(Navigator.prototype, "userAgent", "get").mockReturnValue(fireTvSilk);
    await page.viewport(960, 480);
    await asAtFirstLoad();
    walking = await mountApp(walkServices(watchingOne, "/"));
    await expect.poll(() => isDrawn(".continue-watching .poster-card__link")).toBe(true);
    await expect.poll(isOnHomeMark, { message: focused() }).toBe(true);
    expect(drawsRing(document.activeElement)).toBe(true);
    await userEvent.keyboard("{ArrowDown}");
    await expect.poll(() => isFocusIn("main"), { message: focused() }).toBe(true);
    expect(drawsRing(document.activeElement)).toBe(true);
  });

  it("leaves no ring on the picture of the film a pointer opened from it", async () => {
    await page.viewport(1280, 800);
    await asAtFirstLoad();
    walking = await mountApp(walkServices(watchingOne, "/"));
    await expect.poll(() => isDrawn(".continue-watching .poster-card__link")).toBe(true);
    await expect.poll(isOnHomeMark, { message: focused() }).toBe(true);
    const poster = document.querySelector<HTMLElement>(".continue-watching .poster-card__link");
    if (poster !== null) {
      await userEvent.click(poster);
    }
    await expect.poll(() => document.querySelector("video")?.error?.code).toBeDefined();
    const picture = document.querySelector(".film-player__stage video");
    await expect.poll(() => document.activeElement === picture, { message: focused() }).toBe(true);
    expect(drawsRing(picture)).toBe(false);
  });

  it("leaves no ring on the picture of the film a key opened", async () => {
    await page.viewport(1280, 800);
    await asAtFirstLoad();
    walking = await mountApp(walkServices(watchingOne, "/"));
    await expect.poll(() => isDrawn(".continue-watching .poster-card__link")).toBe(true);
    await expect.poll(isOnHomeMark, { message: focused() }).toBe(true);
    const poster = document.querySelector<HTMLElement>(".continue-watching .poster-card__link");
    poster?.focus({ focusVisible: true });
    expect(drawsRing(poster)).toBe(true);
    await userEvent.keyboard("{Enter}");
    await expect.poll(() => document.querySelector("video")?.error?.code).toBeDefined();
    const picture = document.querySelector(".film-player__stage video");
    await expect.poll(() => document.activeElement === picture, { message: focused() }).toBe(true);
    expect(drawsRing(picture)).toBe(false);
    await userEvent.keyboard("{Shift}");
    expect(document.activeElement).toBe(picture);
    expect(drawsRing(picture)).toBe(false);
  });
});

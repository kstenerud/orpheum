// SPDX-License-Identifier: MPL-2.0

/**
 * Where focus is as a page or a dialog opens, its arrival (ui-audit-2026-11-04.md R3; focus-navigation.ts
 * arriveWhenDrawn): on what the page is for, so the first press acts on it, in a real browser, as
 * the app draws it from the server's answers; and shown with no ring where a pointer opened it.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { createSession } from "./session.ts";
import { fakeTogetherServer } from "./test-together.ts";
import {
  filmDetail,
  films,
  focusedName,
  isDrawn,
  isFocusIn,
  isFocusOnFirst,
  mountApp,
  viewer,
  walkServices,
  type Walking,
  type WalkAnswers,
} from "./test-walk.tsx";

/** The server's answers every signed-in page reads. */
const signedIn: WalkAnswers = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /home": { continueWatching: [] },
  "GET /items": { items: films(6), nextCursor: "" },
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
        kind: "movie",
        metadataSource: "nfo_then_tmdb",
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
    walking = await mountApp(walkServices(signedIn, "/films"));
    await expect.poll(() => isFocusOnFirst(".poster-grid .poster-card__link")).toBe(true);
  });

  // ui-audit-2026-11-04.md R4: a screen reader is told where it now is, politely, while focus stays
  // on the page's arrival rather than on its heading.
  it("stays off the heading, which is announced, on a page opened and on one Back returns to", async () => {
    await page.viewport(1280, 800);
    walking = await mountApp(
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
    await expect.poll(() => document.activeElement?.getAttribute("href")).toBe("/titles/101");
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
    walking = await mountApp(
      walkServices({ ...signedIn, "GET /items/103": filmDetail(103, "Film 3") }, "/titles/103"),
    );
    await arrivesIn(".title-head__actions");
    expect(focusedName()).toBe("Play");
  });

  it("is a series' next episode", async () => {
    await page.viewport(1280, 800);
    walking = await mountApp(
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
              titleId: 302,
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
    walking = await mountApp(
      walkServices(
        {
          ...signedIn,
          "GET /home": {
            continueWatching: [
              { id: 7, name: "Metropolis", libraryId: 1, fileId: 70, positionMs: 250, durationMs: 1000 },
            ],
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
    walking = await mountApp(walkServices(signedIn, "/"));
    await expect
      .poll(() => document.activeElement?.getAttribute("href"), { message: focused() })
      .toBe("/titles/101");
  });

  it("is This device's Rename", async () => {
    await page.viewport(1280, 800);
    walking = await mountApp(
      walkServices(
        { ...signedIn, "GET /me/devices": { devices: [device(5, "Karl's laptop", true)] } },
        "/this-device",
      ),
    );
    await expect.poll(focusedName, { message: focused() }).toBe("Rename");
  });

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
    walking = await mountApp(walkServices(anAdmin, "/libraries"));
    await arrivesIn(".libraries__list");
    expect(focusedName()).toBe("Manage this library");
  });

  it("is the People page's first person", async () => {
    await page.viewport(1280, 800);
    walking = await mountApp(walkServices(anAdmin, "/users"));
    await arrivesIn(".people__list");
    expect(focusedName()).toContain("Karl");
  });

  it("is the first of the files a library's scanner saw", async () => {
    await page.viewport(1280, 800);
    walking = await mountApp(walkServices(anAdmin, "/libraries/1/files"));
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

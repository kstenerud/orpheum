// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { act } from "react";

import { App } from "./app.tsx";
import { tabStorage } from "./browser-storage.ts";
import {
  buttonWithText,
  click,
  mount,
  refusal,
  servedBy,
  settleUntil,
  unreachable,
  type Answers,
  type Heard,
} from "./test-dom.tsx";
import { createPathMoves } from "./services.tsx";
import { testServices, withServices } from "./test-services.tsx";
import { forgetMembership, keepMembership } from "./together-storage.ts";

/** What the sign-in page says after signing out, when the server answers DELETE /session so. */
async function noticeAfterSigningOut(answer: Answers[string]): Promise<string | undefined> {
  const services = servedBy(
    {
      "GET /me": [200, { id: 1, name: "owner", role: "admin", isAdmin: true }],
      "GET /home": [200, { continueWatching: [] }],
      "GET /titles": [200, { titles: [], nextCursor: "" }],
      "DELETE /session": answer,
    },
    [],
    "token",
  );
  const mounted = await mount(withServices(<App />, services));
  await click(buttonWithText(mounted.container, "Sign out"));
  const isSignedOut = services.session.token() === undefined;
  const notice = mounted.container.querySelector("output")?.textContent ?? undefined;
  mounted.unmount();
  return isSignedOut ? notice : "still signed in";
}

describe("App", () => {
  it("asks to sign in until there is a session", () => {
    const markup = renderToStaticMarkup(withServices(<App />, testServices()));
    expect(markup).toContain('<main class="credentials-form">');
    expect(markup).not.toContain("app-shell");
  });

  it("shows the shell and the route's screen once signed in", () => {
    const markup = renderToStaticMarkup(withServices(<App />, testServices("token", "/about")));
    expect(markup).toContain('class="app-shell"');
    expect(markup).toContain("not endorsed or certified by TMDB");
  });

  it("shows the screen each address names", () => {
    const films = renderToStaticMarkup(withServices(<App />, testServices("token", "/films")));
    expect(films).toContain('id="browse-heading">Films<');
    const search = renderToStaticMarkup(withServices(<App />, testServices("token", "/search")));
    expect(search).toContain('role="search"');
    const libraries = renderToStaticMarkup(
      withServices(<App />, testServices("token", "/libraries")),
    );
    expect(libraries).toContain('id="libraries-heading"');
    const files = renderToStaticMarkup(
      withServices(<App />, testServices("token", "/libraries/3/files")),
    );
    expect(files).toContain('id="files-heading"');
    const identify = renderToStaticMarkup(
      withServices(<App />, testServices("token", "/identify")),
    );
    expect(identify).toContain('id="identify-heading"');
    const jobs = renderToStaticMarkup(withServices(<App />, testServices("token", "/jobs")));
    expect(jobs).toContain('id="jobs-heading"');
    const system = renderToStaticMarkup(withServices(<App />, testServices("token", "/system")));
    expect(system).toContain("Loading…");
    expect(system).not.toContain("There is nothing at this address.");
  });

  it("asks again what this session may do each time the page's requests move path (D297 §6)", async () => {
    const heard: Heard[] = [];
    const answers: Answers = {
      // On the core's public name it administers nothing; on its LAN name, once moved there, it does.
      "GET /me": [
        [200, { id: 1, name: "owner", role: "admin", isAdmin: false }],
        [200, { id: 1, name: "owner", role: "admin", isAdmin: true }],
      ],
      "GET /home": [200, { continueWatching: [] }],
      "GET /titles": [200, { titles: [], nextCursor: "" }],
      "GET /needs-attention": [
        200,
        { rootCount: 1, diskCount: 0, waitingRootCount: 0, unreadCount: 0, matchCount: 0 },
      ],
    };
    const moves = createPathMoves();
    const services = { ...servedBy(answers, heard, "token"), pathMoves: moves };
    const mounted = await mount(withServices(<App />, services));
    const page = mounted.container;
    const hasSign = (): boolean => page.querySelector(".status-tray__button--warning") !== null;
    const askedMe = (): number =>
      heard.filter((request) => request.methodAndPath === "GET /me").length;
    expect(hasSign()).toBe(false);
    const askedBefore = askedMe();
    act(() => {
      moves.moved();
    });
    await settleUntil(hasSign);
    expect(askedMe()).toBeGreaterThan(askedBefore);
    mounted.unmount();
  });

  it("keeps an admin's warning sign while what the session may do is read again after a move (the owner, 2026-10-17)", async () => {
    const answers: Answers = {
      "GET /me": [200, { id: 1, name: "owner", role: "admin", isAdmin: true }],
      "GET /home": [200, { continueWatching: [] }],
      "GET /titles": [200, { titles: [], nextCursor: "" }],
      "GET /needs-attention": [
        200,
        { rootCount: 1, diskCount: 0, waitingRootCount: 0, unreadCount: 0, matchCount: 0 },
      ],
    };
    const moves = createPathMoves();
    const mounted = await mount(
      withServices(<App />, { ...servedBy(answers, [], "token"), pathMoves: moves }),
    );
    const hasSign = (): boolean =>
      mounted.container.querySelector(".status-tray__button--warning") !== null;
    expect(hasSign()).toBe(true);
    act(() => {
      moves.moved();
    });
    expect(hasSign()).toBe(true);
    mounted.unmount();
  });

  it("shows an admin a warning sign in the tray, not a reminder under the bar, and asks nothing of it for anyone else", async () => {
    const pageFor = async (
      isAdmin: boolean,
    ): Promise<{ text: string; hasSign: boolean; asked: string[] }> => {
      const heard: Heard[] = [];
      const services = servedBy(
        {
          "GET /me": [200, { id: 1, name: "owner", role: isAdmin ? "admin" : "user", isAdmin }],
          "GET /home": [200, { continueWatching: [] }],
          "GET /titles": [200, { titles: [], nextCursor: "" }],
          "GET /needs-attention": [
            200,
            { rootCount: 1, diskCount: 0, waitingRootCount: 0, unreadCount: 0, matchCount: 0 },
          ],
        },
        heard,
        "token",
      );
      const mounted = await mount(withServices(<App />, services));
      const text = mounted.container.textContent;
      const hasSign = mounted.container.querySelector(".status-tray__button--warning") !== null;
      mounted.unmount();
      return { text, hasSign, asked: heard.map((request) => request.methodAndPath) };
    };
    // The tray's sign says it alone: the reminder under the bar said it twice (the owner, 2026-10-12).
    const admin = await pageFor(true);
    expect(admin.hasSign).toBe(true);
    expect(admin.text).not.toContain("offline since");
    const person = await pageFor(false);
    expect(person.hasSign).toBe(false);
    expect(person.asked).not.toContain("GET /needs-attention");
  });

  it("shows an admin the warning sign while films or series wait for their match, and the page leads to them (the owner, 2026-10-13)", async () => {
    const services = servedBy(
      {
        "GET /me": [200, { id: 1, name: "owner", role: "admin", isAdmin: true }],
        "GET /home": [200, { continueWatching: [] }],
        "GET /titles": [200, { titles: [], nextCursor: "" }],
        "GET /needs-attention": [
          200,
          { rootCount: 0, diskCount: 0, waitingRootCount: 0, unreadCount: 0, matchCount: 97 },
        ],
      },
      [],
      "token",
    );
    const mounted = await mount(withServices(<App />, services));
    const sign = mounted.container.querySelector(".status-tray__button--warning");
    expect(sign).not.toBeNull();
    await click(sign);
    expect(mounted.container.textContent).toContain(
      "97 films and series need you to choose their match.",
    );
    const toMatches = [...mounted.container.querySelectorAll("a")].find(
      (link) => link.textContent === "Needs a match" && link.closest("main") !== null,
    );
    expect(toMatches?.getAttribute("href")).toBe("/identify");
    mounted.unmount();
  });

  it("offers the UI gallery in the menu only while the server's setting for it is on (the owner, 2026-10-17)", async () => {
    const offersGallery = async (isOn: boolean | undefined): Promise<boolean> => {
      const services = servedBy(
        {
          "GET /me": [200, { id: 1, name: "owner", role: "user", isAdmin: false }],
          "GET /home": [200, { continueWatching: [] }],
          "GET /titles": [200, { titles: [], nextCursor: "" }],
          ...(isOn === undefined
            ? {}
            : {
                "GET /client-settings": [
                  200,
                  {
                    screenRefreshFirstWaitSeconds: 2,
                    screenRefreshLongestWaitSeconds: 30,
                    screenRefreshFailedWaitSeconds: 5,
                    shouldShowDeveloperPages: isOn,
                  },
                ],
              }),
        },
        [],
        "token",
      );
      const mounted = await mount(withServices(<App />, services));
      const hasGallery = mounted.container.querySelector('a[href="/ui-gallery"]') !== null;
      mounted.unmount();
      return hasGallery;
    };
    expect(await offersGallery(true)).toBe(true);
    expect(await offersGallery(false)).toBe(false);
    // Unread, the setting is off.
    expect(await offersGallery(undefined)).toBe(false);
  });

  it("says when an address names nothing", () => {
    const markup = renderToStaticMarkup(withServices(<App />, testServices("token", "/nowhere")));
    expect(markup).toContain("There is nothing at this address.");
  });

  it("signs out here whatever the server says, and says when the server could not be told", async () => {
    expect(await noticeAfterSigningOut([204, undefined])).toBeUndefined();
    expect(await noticeAfterSigningOut(unreachable)).toContain("the server could not be told");
    expect(await noticeAfterSigningOut(refusal(503, "migrating"))).toContain(
      "the server could not be told",
    );
  });

  it("marks Watch Together as on, on every screen, while this tab hosts a session (the owner's UI backlog)", async () => {
    window.sessionStorage.clear();
    const services = servedBy(
      {
        "GET /me": [
          200,
          { id: 1, name: "owner", role: "admin", isAdmin: true, canHostWatchTogether: true },
        ],
        "GET /home": [200, { continueWatching: [] }],
        "GET /titles": [200, { titles: [], nextCursor: "" }],
        "GET /needs-attention": [
          200,
          { rootCount: 0, diskCount: 0, waitingRootCount: 0, unreadCount: 0, matchCount: 0 },
        ],
      },
      [],
      "token",
    );
    const mounted = await mount(withServices(<App />, services));
    const mark = (): Element | null => mounted.container.querySelector(".app-shell__on");
    expect(mark()).toBeNull();
    await act(async () => {
      keepMembership(tabStorage(), "host", { sessionId: 3, credential: "host-credential" });
      await Promise.resolve();
    });
    expect(mark()?.closest("a")?.getAttribute("href")).toBe("/watch-together");
    expect(mark()?.closest("a")?.textContent).toBe("Watch Together on now");
    await act(async () => {
      forgetMembership(tabStorage(), "host");
      await Promise.resolve();
    });
    expect(mark()).toBeNull();
    mounted.unmount();
  });
});

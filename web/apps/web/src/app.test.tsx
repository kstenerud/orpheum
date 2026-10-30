// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { readyAheadOf, setReadyAheadPercent, tellNextChunkLoadMs } from "@orpheum/sdk";
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

/**
 * What every screen asks in the background once the shell mounts, regardless of what a test
 * cares about: the alphabet rail's letters, the poll's own waits, this tab's Watch Together
 * sessions (D237, D267 §2) and the libraries the banner's Libraries menu lists. A test spreads this
 * in and adds what it is actually testing.
 */
const backgroundAnswers: Answers = {
  "GET /libraries": [200, { libraries: [] }],
  "GET /titles/letters": [200, { letters: [] }],
  "GET /client-settings": [
    200,
    {
      screenRefreshFirstWaitSeconds: 2,
      screenRefreshLongestWaitSeconds: 30,
      screenRefreshFailedWaitSeconds: 5,
      interestSaidEverySeconds: 30,
      shouldShowDeveloperPages: false,
      shouldOfferDeviceTests: false,
    },
  ],
  "GET /together/mine": [200, { sessions: [] }],
};

/** What the sign-in page says after signing out, when the server answers DELETE /session so. */
async function noticeAfterSigningOut(answer: Answers[string]): Promise<string | undefined> {
  const services = servedBy(
    {
      ...backgroundAnswers,
      "GET /me": [200, { id: 1, name: "owner", role: "admin", isAdmin: true }],
      "GET /home": [200, { continueWatching: [] }],
      "GET /titles": [200, { titles: [], nextCursor: "" }],
      "GET /needs-attention": [
        200,
        { rootCount: 0, diskCount: 0, waitingRootCount: 0, unreadCount: 0, matchCount: 0 },
      ],
      "GET /setup": [200, { isRequired: false }],
      "DELETE /session": answer,
    },
    [],
    "token",
  );
  const mounted = await mount(withServices(<App />, services));
  await click(mounted.container.querySelector(".app-shell__menu-button"));
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
    expect(films).toContain('id="browse-heading">All Films<');
    // Its bar needs the libraries list first (for the chooser, the sort and the view it offers),
    // so it shows Loading… here, as the System page does; library-browse.test.tsx mounts it
    // asynchronously and finds its role="search" field once that answers.
    const browse = renderToStaticMarkup(withServices(<App />, testServices("token", "/browse")));
    expect(browse).toContain("Loading…");
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

  it("shows a library's own page and creating one, each at an address of its own (D361)", () => {
    for (const path of ["/libraries/3", "/libraries/new"]) {
      const markup = renderToStaticMarkup(withServices(<App />, testServices("token", path)));
      expect(markup).toContain('class="up-trail"');
      expect(markup).not.toContain("There is nothing at this address.");
    }
  });

  it("asks again what this session may do each time the page's requests move path (D297 §6)", async () => {
    const heard: Heard[] = [];
    const answers: Answers = {
      ...backgroundAnswers,
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
      ...backgroundAnswers,
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
          ...backgroundAnswers,
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
        ...backgroundAnswers,
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
          "GET /titles/letters": [200, { letters: [] }],
          "GET /together/mine": [200, { sessions: [] }],
          "GET /libraries": [200, { libraries: [] }],
          "GET /me": [200, { id: 1, name: "owner", role: "user", isAdmin: false }],
          "GET /home": [200, { continueWatching: [] }],
          "GET /titles": [200, { titles: [], nextCursor: "" }],
          "GET /client-settings":
            isOn === undefined
              ? unreachable
              : [
                  200,
                  {
                    screenRefreshFirstWaitSeconds: 2,
                    screenRefreshLongestWaitSeconds: 30,
                    screenRefreshFailedWaitSeconds: 5,
                    interestSaidEverySeconds: 30,
                    shouldShowDeveloperPages: isOn,
                    shouldOfferDeviceTests: false,
                  },
                ],
        },
        [],
        "token",
      );
      const mounted = await mount(withServices(<App />, services));
      // The menu's places are drawn while it is open (drop-down.tsx).
      await click(mounted.container.querySelector(".app-shell__menu-button"));
      const hasGallery = mounted.container.querySelector('a[href="/ui-gallery"]') !== null;
      mounted.unmount();
      return hasGallery;
    };
    expect(await offersGallery(true)).toBe(true);
    expect(await offersGallery(false)).toBe(false);
    // Unread, the setting is off.
    expect(await offersGallery(undefined)).toBe(false);
  });

  it("has every player wait by the server's margin on its next chunk's load (the owner, 2026-10-28)", async () => {
    const services = servedBy(
      {
        ...backgroundAnswers,
        "GET /client-settings": [
          200,
          {
            screenRefreshFirstWaitSeconds: 2,
            screenRefreshLongestWaitSeconds: 30,
            screenRefreshFailedWaitSeconds: 5,
            interestSaidEverySeconds: 30,
            shouldShowDeveloperPages: false,
            shouldOfferDeviceTests: false,
            readyAheadOfNextChunkPercent: 300,
          },
        ],
        "GET /me": [200, { id: 1, name: "owner", role: "user", isAdmin: false }],
        "GET /home": [200, { continueWatching: [] }],
        "GET /titles": [200, { titles: [], nextCursor: "" }],
      },
      [],
      "token",
    );
    const video = {};
    tellNextChunkLoadMs(video, () => ({ loadMs: 1000, from: "confident" }));
    const mounted = await mount(withServices(<App />, services));
    await settleUntil(() => readyAheadOf(video).aheadMs === 3000);
    expect(readyAheadOf(video).aheadMs).toBe(3000);
    mounted.unmount();
    setReadyAheadPercent(150);
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

  // The owner's Fire TV, 2026-10-29: "When I sign out, it goes to the core's sign-in page for a while
  // and then switches to the discovery's sign-in page. That would be confusing for a user."
  it("signed out under a discovery's loader through a realm, draws no server's sign-in while the loader leaves", async () => {
    const services = servedBy(
      {
        ...backgroundAnswers,
        "GET /me": [200, { id: 1, name: "owner", role: "viewer", isAdmin: false }],
        "GET /home": [200, { continueWatching: [] }],
        "GET /titles": [200, { titles: [], nextCursor: "" }],
        "DELETE /session": [204, undefined],
      },
      [],
      "token",
    );
    const mounted = await mount(withServices(<App />, { ...services, realm: "family" }));
    await click(mounted.container.querySelector(".app-shell__menu-button"));
    await click(buttonWithText(mounted.container, "Sign out"));
    await settleUntil(() => services.session.token() === undefined);
    expect(mounted.container.querySelector(".credentials-form")).toBeNull();
    expect(mounted.container.textContent).toContain("Signed out. Taking you to sign in");
    mounted.unmount();
  });

  it("turns Watch Together's sign blue, on every screen, while this tab hosts a session, with no top-bar item (D364 §12)", async () => {
    window.sessionStorage.clear();
    const services = servedBy(
      {
        ...backgroundAnswers,
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
        "GET /together/3": [
          200,
          {
            sessionId: 3,
            nextRights: { canPlayPause: true, canSeek: true },
            links: [],
            members: [{ id: 1, name: "owner", isHost: true }],
          },
        ],
      },
      [],
      "token",
    );
    const mounted = await mount(withServices(<App />, services));
    const sign = (): Element | null =>
      mounted.container.querySelector(".status-tray a[href='/watch-together']");
    expect(
      mounted.container.querySelector(".app-shell__watching a[href='/watch-together']"),
    ).toBeNull();
    expect(sign()?.className).toContain("status-tray__button--off");
    await act(async () => {
      keepMembership(tabStorage(), "host", { sessionId: 3, credential: "host-credential" });
      await Promise.resolve();
    });
    expect(sign()?.className).toContain("status-tray__button--on");
    expect(sign()?.getAttribute("aria-label")).toBe("Watch Together is on: its controls");
    await act(async () => {
      forgetMembership(tabStorage(), "host");
      await Promise.resolve();
    });
    expect(sign()?.className).toContain("status-tray__button--off");
    mounted.unmount();
  });
});

describe("the signed-in page's start", () => {
  it("reads each row Home polls once, when the server's waits arrive after it started (W3)", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /me": [200, { id: 1, name: "owner", role: "user", isAdmin: false }],
        "GET /titles": [200, { titles: [], nextCursor: "" }],
        "GET /home": [200, { continueWatching: [] }],
        "GET /client-settings": [
          200,
          {
            screenRefreshFirstWaitSeconds: 3,
            screenRefreshLongestWaitSeconds: 60,
            screenRefreshFailedWaitSeconds: 7,
          },
        ],
        "GET /together/mine": [200, { sessions: [] }],
        "GET /libraries": [200, { libraries: [] }],
      },
      heard,
      "token",
    );
    services.navigation.go({ screen: "home" });
    const mounted = await mount(withServices(<App />, services));
    expect(heard.some((each) => each.methodAndPath === "GET /client-settings")).toBe(true);
    const rows = heard
      .filter((each) => each.methodAndPath === "GET /home" || each.methodAndPath === "GET /titles")
      .map((each) => `${each.methodAndPath}${each.query ?? ""}`);
    expect(rows.length).toBe(3);
    // Each row is asked for once, not again when the poller takes the server's waits.
    expect(new Set(rows).size).toBe(rows.length);
    mounted.unmount();
  });
});

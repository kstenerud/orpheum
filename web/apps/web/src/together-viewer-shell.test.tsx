// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { RosterMember, TogetherSnapshot } from "@orpheum/sdk";
import { act } from "react";
import { beforeEach, describe, expect, it } from "vitest";

import { App } from "./app.tsx";
import { createNavigation } from "./navigation.ts";
import { buttonWithText, click, mount, servedBy, settleUntil } from "./test-dom.tsx";
import { fakePlace, withServices } from "./test-services.tsx";
import { liveAndSession } from "./test-together.ts";
import { TogetherNews } from "./together-news.tsx";
import { SessionPeople, type SessionDetails } from "./together-people.tsx";

const welcome = {
  type: "welcome",
  seq: 1,
  sessionId: 1,
  memberId: 2,
  deviceId: 8,
  group: { state: "Idle", positionMs: 0, rate: 1, fileId: 0 },
  roster: [
    { id: 1, name: "Karl", isHost: true, devices: [{ id: 3, role: "video" }] },
    { id: 2, name: "Gran", devices: [{ id: 8, role: "video" }] },
  ],
};

/** A signed-in guest's App at address, keeping their membership of session 1. */
function signedInGuest(address: string): {
  readonly services: ReturnType<typeof servedBy>;
  readonly sockets: ReturnType<typeof liveAndSession>;
} {
  window.localStorage.setItem(
    "orpheum.together.viewer",
    JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
  );
  const sockets = liveAndSession(1); // the shell's live connection opens first
  return {
    services: {
      ...servedBy(
        {
          "GET /me": [200, { id: 4, name: "Gran", role: "user", isAdmin: false }],
          "GET /client-settings": [200, {}],
          "GET /together/mine": [200, { sessions: [] }],
        },
        [],
        "token",
      ),
      navigation: createNavigation(fakePlace(address)),
      openTogetherSocket: sockets.openSocket,
    },
    sockets,
  };
}

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
});

describe("a signed-in guest's Watch Together page (D372 §9; the owner, 2026-10-29)", () => {
  it("is a screen in the shell: the banner, its menu with Your account, and the tray's Watch Together sign leading back to it, in one main landmark", async () => {
    const { services, sockets } = signedInGuest("/together");
    const mounted = await mount(withServices(<App />, services));
    await settleUntil(() => mounted.container.textContent.includes("Tap to continue"));
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await sockets.live.accept();
    await sockets.session.acceptEach();
    await sockets.session.say(welcome);
    const { container } = mounted;
    expect(container.querySelector("header nav")).not.toBeNull();
    expect(container.querySelectorAll("main")).toHaveLength(1);
    expect(container.querySelector("main")?.textContent).toContain("Watching together");
    await click(container.querySelector('button[aria-label="Menu"]'));
    expect(container.textContent).toContain("Your account");
    const sign = container.querySelector(
      '.status-tray a[aria-label="Watch Together is on: its controls"]',
    );
    expect(sign?.getAttribute("href")).toBe("/together");
    // The banner's logo is the page's one: the guest's page draws none of its own in the shell.
    expect(container.querySelector("main .together-viewer__logo")).toBeNull();
    mounted.unmount();
  });

  it("goes elsewhere in the app a member still, and comes back by the tray's sign to the session, connecting again by the same membership", async () => {
    const { services, sockets } = signedInGuest("/together");
    const mounted = await mount(withServices(<App />, services));
    await settleUntil(() => mounted.container.textContent.includes("Tap to continue"));
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await sockets.live.accept();
    await sockets.session.acceptEach();
    await sockets.session.say(welcome);
    const openedBefore = sockets.session.openedCount();
    await act(async () => {
      services.navigation.go({ screen: "about" });
      await Promise.resolve();
    });
    expect(window.localStorage.getItem("orpheum.together.viewer")).toContain("viewer-credential");
    expect(mounted.container.querySelector(".together-viewer")).toBeNull();
    const sign = mounted.container.querySelector<HTMLAnchorElement>(
      '.status-tray a[aria-label="Watch Together is on: its controls"]',
    );
    expect(sign).not.toBeNull();
    await click(sign);
    await sockets.session.acceptEach();
    expect(services.navigation.route()).toEqual({ screen: "together" });
    expect(sockets.session.openedCount()).toBeGreaterThan(openedBefore);
    const hellos = sockets.session.heard.filter((each) => each["type"] === "hello");
    expect(hellos.at(-1)).toMatchObject({ credential: "viewer-credential" });
    mounted.unmount();
  });

  it("stays a page of its own for a guest by link alone: no banner", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const sockets = liveAndSession(1);
    const services = {
      ...servedBy({ "GET /health": [200, { status: "ready", reasons: [] }] }),
      navigation: createNavigation(fakePlace("/together")),
      openTogetherSocket: sockets.openSocket,
    };
    const mounted = await mount(withServices(<App />, services));
    await settleUntil(() => mounted.container.textContent.includes("Tap to continue"));
    expect(mounted.container.querySelector("nav")).toBeNull();
    expect(mounted.container.querySelectorAll("main")).toHaveLength(1);
    mounted.unmount();
  });
});

describe("who is away from the session's page (D372 §9)", () => {
  const roster = [
    { id: 1, name: "Karl", isHost: true, devices: [{ id: 3, role: "video" }] },
    { id: 2, name: "Gran", isHost: false, devices: [] },
  ] as unknown as readonly RosterMember[];

  it("says, in who is here, a member with no device on the session's page is away", async () => {
    const snapshot = { status: "connected", roster } as unknown as TogetherSnapshot;
    const mounted = await mount(<TogetherNews snapshot={snapshot} />);
    expect(mounted.container.textContent).toContain("Here: Karl and Gran (away)");
    mounted.unmount();
  });

  it("says so on the host's list of people, once the roster is known", async () => {
    const details = {
      sessionId: 1,
      members: [
        { id: 1, name: "Karl", isHost: true },
        { id: 2, name: "Gran", isHost: false, hostNote: "Mum" },
      ],
      links: [],
      nextRights: { canPlayPause: false, canSeek: false },
    } as unknown as SessionDetails;
    const actions = {
      details,
      run: () => undefined,
      onChanged: () => undefined,
      onIssued: () => undefined,
    };
    const unknown = await mount(withServices(<SessionPeople {...actions} />, servedBy({})));
    expect(unknown.container.textContent).toBe("Gran (Mum)");
    unknown.unmount();
    const known = await mount(
      withServices(<SessionPeople {...actions} roster={roster} />, servedBy({})),
    );
    expect(known.container.textContent).toBe("Gran (Mum), away");
    known.unmount();
  });
});

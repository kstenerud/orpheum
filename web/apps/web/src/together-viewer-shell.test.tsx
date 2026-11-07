// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { RosterMember, TogetherSnapshot } from "@orpheum/sdk";
import { beforeEach, describe, expect, it } from "vitest";

import { App } from "./app.tsx";
import { createNavigation } from "./navigation.ts";
import { dismissNotice, currentNotice } from "./notice.ts";
import {
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
  type Answers,
  type Heard,
} from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { fakePlace, withServices } from "./test-services.tsx";
import { liveAndSession } from "./test-together.ts";
import { TogetherNews } from "./together-news.tsx";
import { SessionPeople, type SessionDetails } from "./together-people.tsx";
import { currentTrayDialog } from "./tray-dialog.ts";

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

/** A signed-in guest's App at address, keeping their membership of session 1; more answered too. */
function signedInGuest(
  address: string,
  more: Answers = {},
): {
  readonly services: ReturnType<typeof servedBy>;
  readonly sockets: ReturnType<typeof liveAndSession>;
  readonly heard: Heard[];
} {
  window.localStorage.setItem(
    "orpheum.together.viewer",
    JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
  );
  const sockets = liveAndSession(1); // the shell's live connection opens first
  const heard: Heard[] = [];
  return {
    services: {
      ...servedBy(
        {
          "GET /me": [200, { id: 4, name: "Gran", role: "user", isAdmin: false }],
          "GET /client-settings": [200, {}],
          "GET /me/together": [200, { sessions: [] }],
          "GET /libraries": [200, { libraries: [] }],
          "GET /home": [200, { continueWatching: [] }],
          "GET /items": [200, { items: [], nextCursor: "" }],
          "POST /together/playback": [200, directDecision({})],
          ...more,
        },
        heard,
        "token",
      ),
      navigation: createNavigation(fakePlace(address)),
      openTogetherSocket: sockets.openSocket,
    },
    sockets,
    heard,
  };
}

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
  const left = currentNotice();
  if (left !== undefined) {
    dismissNotice(left);
  }
});

describe("a signed-in guest in a Watch Together session (D372 §9, D384 §2)", () => {
  it("waits on their page, in the shell, while the group plays nothing, no dialog opened, and moves to the Play page of the film the host starts (VQ193)", async () => {
    const { services, sockets } = signedInGuest("/together");
    const mounted = await mount(withServices(<App />, services));
    await sockets.live.accept();
    await settleUntil(() => sockets.session.openedCount() > 0); // what the group plays
    await sockets.session.acceptEach();
    await sockets.session.say(welcome);
    const { container } = mounted;
    await settleUntil(() => buttonWithText(container, "Tap to continue") !== undefined);
    await click(buttonWithText(container, "Tap to continue"));
    await sockets.session.acceptEach();
    await sockets.session.say(welcome);
    expect(services.navigation.route()).toEqual({ screen: "together" });
    expect(container.textContent).toContain("Waiting for the host to start a film.");
    expect(container.querySelector("header nav")).not.toBeNull();
    expect(container.querySelectorAll("main")).toHaveLength(1);
    expect(container.querySelector("dialog")).toBeNull();
    expect(currentTrayDialog()).toBeUndefined();
    const hellos = sockets.session.heard.filter((each) => each["type"] === "hello");
    expect(hellos.at(-1)).toMatchObject({ credential: "viewer-credential", role: "video" });
    await sockets.session.say({
      type: "group",
      seq: 2,
      group: { state: "Playing", positionMs: 0, rate: 1, fileId: 5 },
    });
    await settleUntil(() => services.navigation.route().screen === "play");
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 5 });
    expect(currentTrayDialog()).toBeUndefined();
    mounted.unmount();
  });

  it("lands on the Play page of the group's film from the join, no dialog opened (VQ193)", async () => {
    const { services, sockets } = signedInGuest("/together#link-secret", {
      "POST /together/join/check": [204, undefined],
      "POST /together/join": [
        200,
        { sessionId: 1, memberId: 2, membershipCredential: "viewer-credential", locator: "" },
      ],
    });
    window.localStorage.clear(); // not a member yet: the link joins
    const mounted = await mount(withServices(<App />, services));
    await sockets.live.accept();
    const { container } = mounted;
    await settleUntil(() => buttonWithText(container, "Join") !== undefined);
    await settleUntil(() => container.querySelector("input")?.value === "Gran");
    await click(buttonWithText(container, "Join"));
    await settleUntil(() => sockets.session.openedCount() > 0); // what the group plays
    await sockets.session.acceptEach();
    await sockets.session.say({
      ...welcome,
      group: { state: "Playing", positionMs: 0, rate: 1, fileId: 5 },
    });
    await settleUntil(() => services.navigation.route().screen === "play");
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 5 });
    expect(container.querySelector("dialog")).toBeNull();
    expect(currentTrayDialog()).toBeUndefined();
    expect(window.localStorage.getItem("orpheum.together.viewer")).toContain("viewer-credential");
    mounted.unmount();
  });

  it("goes elsewhere in the app a member still, and comes back by the tray's dialog to the Play page of the group's film (D384 §2)", async () => {
    const { services, sockets } = signedInGuest("/about");
    const mounted = await mount(withServices(<App />, services));
    await sockets.live.accept();
    await settleUntil(
      () =>
        mounted.container.querySelector(
          '.status-tray button[aria-label="Watch Together is on: its controls"]',
        ) !== null,
    );
    await click(
      mounted.container.querySelector(
        '.status-tray button[aria-label="Watch Together is on: its controls"]',
      ),
    );
    await settleUntil(() => sockets.session.openedCount() > 0);
    await sockets.session.acceptEach();
    await sockets.session.say({
      ...welcome,
      group: { state: "Playing", positionMs: 0, rate: 1, fileId: 5 },
    });
    expect(services.navigation.route()).toEqual({ screen: "about" });
    await click(buttonWithText(mounted.container, "Back to the film"));
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 5 });
    expect(mounted.container.querySelector("dialog[open]")).toBeNull();
    expect(window.localStorage.getItem("orpheum.together.viewer")).toContain("viewer-credential");
    mounted.unmount();
  });

  it("leaves from the tray's dialog, Leave beside Back to the film: home, told so, the membership forgotten, the dialog closed (the owner's 11-20 finding 2, 2026-11-07)", async () => {
    const { services, sockets, heard } = signedInGuest("/about", {
      "POST /together/leave": [204, undefined],
    });
    const mounted = await mount(withServices(<App />, services));
    await sockets.live.accept();
    const sign = '.status-tray button[aria-label="Watch Together is on: its controls"]';
    await settleUntil(() => mounted.container.querySelector(sign) !== null);
    await click(mounted.container.querySelector(sign));
    await settleUntil(() => sockets.session.openedCount() > 0);
    await sockets.session.acceptEach();
    await sockets.session.say({
      ...welcome,
      group: { state: "Playing", positionMs: 0, rate: 1, fileId: 5 },
    });
    expect(buttonWithText(mounted.container, "Back to the film")).toBeDefined();
    await click(buttonWithText(mounted.container, "Leave"));
    await settleUntil(() => services.navigation.route().screen === "home");
    expect(heard.map((each) => each.methodAndPath)).toContain("POST /together/leave");
    expect(currentNotice()?.text).toBe("You left the Watch Together session.");
    expect(window.localStorage.getItem("orpheum.together.viewer")).toBeNull();
    expect(mounted.container.querySelector('dialog[aria-label="Watch Together"]')).toBeNull();
    mounted.unmount();
  });

  it("ends a session the server no longer has from the tray's dialog: home, told so, the membership forgotten, the dialog closed (the owner's S23, 2026-10-30)", async () => {
    const { services, sockets } = signedInGuest("/about");
    const mounted = await mount(withServices(<App />, services));
    await sockets.live.accept();
    const sign = '.status-tray button[aria-label="Watch Together is on: its controls"]';
    await settleUntil(() => mounted.container.querySelector(sign) !== null);
    await click(mounted.container.querySelector(sign));
    await settleUntil(() => sockets.session.openedCount() > 0);
    await sockets.session.accept();
    await sockets.session.refuse();
    await settleUntil(() => services.navigation.route().screen === "home");
    expect(window.localStorage.getItem("orpheum.together.viewer")).toBeNull();
    expect(mounted.container.querySelector('dialog[aria-label="Watch Together"]')).toBeNull();
    expect(mounted.container.querySelector(sign)).toBeNull();
    mounted.unmount();
  });

  it("goes home from the group's film on its Play page as the host ends the session, in the shell, told why there, the membership forgotten (the owner's S23, staging, 2026-10-30)", async () => {
    const { services, sockets } = signedInGuest("/play/5");
    const mounted = await mount(withServices(<App />, services));
    await sockets.live.accept();
    await settleUntil(() => sockets.session.openedCount() > 0); // whose the film is
    await sockets.session.acceptEach();
    await sockets.session.say({
      ...welcome,
      group: { state: "Playing", positionMs: 0, rate: 1, fileId: 5 },
    });
    const { container } = mounted;
    await settleUntil(() => buttonWithText(container, "Tap to continue") !== undefined);
    await click(buttonWithText(container, "Tap to continue"));
    await sockets.session.acceptEach();
    await sockets.session.say({
      ...welcome,
      group: { state: "Playing", positionMs: 0, rate: 1, fileId: 5 },
    });
    expect(container.querySelector('[aria-label="The film"], .together-viewer')).not.toBeNull();
    await sockets.session.say({ type: "ended", reason: "hostEnded" });
    await settleUntil(() => services.navigation.route().screen === "home");
    expect(currentNotice()?.text).toBe("The host ended the Watch Together session.");
    expect(window.localStorage.getItem("orpheum.together.viewer")).toBeNull();
    expect(container.querySelector("header nav")).not.toBeNull();
    expect(container.querySelectorAll("main")).toHaveLength(1);
    expect(container.querySelector(".together-viewer")).toBeNull();
    expect(container.querySelector("video")).toBeNull();
    mounted.unmount();
  });

  it("goes home from the Play page of the group's film loaded again after the session ended, told so, never playing the film as their own (the owner's S23, staging, 2026-10-30)", async () => {
    const { services, sockets, heard } = signedInGuest("/play/5");
    const mounted = await mount(withServices(<App />, services));
    await sockets.live.accept();
    await settleUntil(() => sockets.session.openedCount() > 0); // whose the film is
    await sockets.session.accept();
    await sockets.session.refuse();
    await settleUntil(() => services.navigation.route().screen === "home");
    expect(currentNotice()?.text).toBe("The Watch Together session is over.");
    expect(window.localStorage.getItem("orpheum.together.viewer")).toBeNull();
    expect(heard.map((each) => each.methodAndPath)).not.toContain("POST /playback");
    expect(mounted.container.querySelector("header nav")).not.toBeNull();
    expect(mounted.container.querySelector("video")).toBeNull();
    mounted.unmount();
  });

  it("stays a page of its own for a guest by link alone: no banner", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const sockets = liveAndSession(1);
    const services = {
      ...servedBy({ "POST /together/join/check": [204, undefined] }),
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

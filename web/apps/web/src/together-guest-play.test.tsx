// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { tabStorage } from "./browser-storage.ts";
import { createNavigation } from "./navigation.ts";
import { currentNotice, dismissNotice } from "./notice.ts";
import { TogetherLeft } from "./old-addresses.tsx";
import { buttonWithText, click, mount, servedBy, type Mounted } from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { fakePlace, withServices } from "./test-services.tsx";
import { fakeTogetherServer, type FakeTogetherServer } from "./test-together.ts";
import { WhoseFilm } from "./together-guest-play.tsx";
import { forgetMembership } from "./together-storage.ts";
import { GroupFilmPage } from "./together-viewer.tsx";
import { currentTrayDialog } from "./tray-dialog.ts";

/** The session's welcome to a device of member 2, the group playing fileId (0: nothing). */
function welcomePlaying(fileId: number): Readonly<Record<string, unknown>> {
  return {
    type: "welcome",
    seq: 1,
    sessionId: 1,
    memberId: 2,
    deviceId: 8,
    group: { state: fileId === 0 ? "idle" : "paused", positionMs: 0, rate: 1, fileId },
    roster: [
      { id: 1, name: "Karl", isHost: true, devices: [{ id: 3, role: "video" }] },
      { id: 2, name: "Gran", devices: [{ id: 8, role: "panel" }] },
    ],
  };
}

/** This browser a signed-in guest in session 1. */
function beAGuest(): void {
  window.localStorage.setItem(
    "orpheum.together.viewer",
    JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
  );
}

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
  const left = currentNotice();
  if (left !== undefined) {
    dismissNotice(left);
  }
});

describe("whose a Play page's film is (D384 §2)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  /** Film fileId's Play page, its own playback standing for the person's own. */
  async function playPage(
    fileId: number,
    navigation = createNavigation(fakePlace(`/play/${String(fileId)}`)),
  ): Promise<FakeTogetherServer> {
    const server = fakeTogetherServer();
    mounted = await mount(
      withServices(<WhoseFilm fileId={fileId} own={<p>own film</p>} />, {
        ...servedBy({}, [], "token"),
        navigation,
        openTogetherSocket: server.openSocket,
      }),
    );
    return server;
  }

  it("is the group's for a signed-in guest whose session plays it, followed with the session around it", async () => {
    beAGuest();
    const server = await playPage(5);
    expect(mounted?.container.textContent).not.toContain("own film");
    await server.accept();
    await server.say(welcomePlaying(5));
    expect(server.heard[0]).toMatchObject({ credential: "viewer-credential", role: "panel" });
    expect(mounted?.container.textContent).not.toContain("own film");
    expect(buttonWithText(mounted!.container, "Tap to continue")).toBeDefined();
  });

  it("is the guest's own where their session plays another film, a member still", async () => {
    beAGuest();
    const server = await playPage(7);
    await server.accept();
    await server.say(welcomePlaying(5));
    expect(mounted?.container.textContent).toContain("own film");
    expect(window.localStorage.getItem("orpheum.together.viewer")).toContain("viewer-credential");
  });

  it("ends for a guest who hosts the session too, the person's own on another device, as it ends under the page: home, told so, the film never theirs (the owner's S23, staging, 2026-10-30)", async () => {
    beAGuest(); // joined here by the link
    window.sessionStorage.setItem(
      "orpheum.together.host",
      JSON.stringify({ sessionId: 1, credential: "host-credential" }),
    );
    const navigation = createNavigation(fakePlace("/play/5"));
    const server = await playPage(5, navigation);
    expect(mounted?.container.textContent).toContain("own film"); // the host's player
    // The session ended, the host's player forgets it (HostPlaysTogether).
    act(() => {
      forgetMembership(tabStorage(), "host");
    });
    expect(mounted?.container.textContent).not.toContain("own film");
    await server.accept();
    await server.refuse();
    expect(navigation.route()).toEqual({ screen: "home" });
    expect(currentNotice()?.text).toBe("The Watch Together session is over.");
    expect(window.localStorage.getItem("orpheum.together.viewer")).toBeNull();
    expect(mounted?.container.textContent).not.toContain("own film");
  });

  it("ends for a guest whose session ends before it says what it plays, the film never theirs", async () => {
    beAGuest();
    const navigation = createNavigation(fakePlace("/play/5"));
    const server = await playPage(5, navigation);
    await server.accept();
    await server.say({ type: "ended", reason: "hostEnded" });
    expect(navigation.route()).toEqual({ screen: "home" });
    expect(currentNotice()?.text).toBe("The host ended the Watch Together session.");
    expect(window.localStorage.getItem("orpheum.together.viewer")).toBeNull();
    expect(mounted?.container.textContent).not.toContain("own film");
  });

  it("is the person's own at once outside a session", async () => {
    const server = await playPage(5);
    expect(mounted?.container.textContent).toContain("own film");
    expect(server.openedCount()).toBe(0);
  });
});

describe("the group's film on a guest's Play page (D384 §2)", () => {
  it("moves to the Play page of the next film the group plays", async () => {
    const server = fakeTogetherServer();
    const services = {
      ...servedBy({ "POST /together/playback": [200, directDecision({})] }, [], "token"),
      navigation: createNavigation(fakePlace("/play/5")),
      openTogetherSocket: server.openSocket,
    };
    const mounted = await mount(
      withServices(
        <GroupFilmPage membership={{ sessionId: 1, credential: "viewer-credential" }} />,
        services,
      ),
    );
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await server.accept();
    await server.say(welcomePlaying(0));
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 5 });
    await server.say({
      type: "group",
      seq: 2,
      group: { state: "idle", positionMs: 0, rate: 1, fileId: 6 },
    });
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 6 });
    mounted.unmount();
  });
});

describe("an old address of Watch Together's page (D384 §2)", () => {
  /** /watch-together, as a history entry holds it, this tab hosting session 1. */
  async function leftAt(fileId: number): Promise<ReturnType<typeof servedBy>> {
    window.sessionStorage.setItem(
      "orpheum.together.host",
      JSON.stringify({ sessionId: 1, credential: "host-credential" }),
    );
    const server = fakeTogetherServer();
    const services = servedBy({}, [], "token");
    services.navigation.go({ screen: "watch-together" });
    const mounted = await mount(
      withServices(<TogetherLeft />, { ...services, openTogetherSocket: server.openSocket }),
    );
    await server.accept();
    await server.say({ ...welcomePlaying(fileId), memberId: 1, isHost: true });
    mounted.unmount();
    return services;
  }

  it("goes to the Play page of the group's film in its place", async () => {
    const services = await leftAt(4);
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 4 });
    expect(services.navigation.behind()).toEqual([{ screen: "films" }]);
    expect(currentTrayDialog()).toBeUndefined();
  });

  it("goes home, Watch Together's dialog open over it, while the group plays nothing", async () => {
    const services = await leftAt(0);
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(currentTrayDialog()).toBe("together");
  });
});

// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { TogetherEndReason } from "@orpheum/sdk";
import { beforeEach, describe, expect, it } from "vitest";

import { createNavigation } from "./navigation.ts";
import { currentNotice, dismissNotice } from "./notice.ts";
import {
  buttonWithText,
  click,
  mount,
  refusal,
  servedBy,
  settleUntil,
  type Mounted,
} from "./test-dom.tsx";
import type { Services } from "./services.tsx";
import { fakePlace, withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";
import { GroupFilmPage, TogetherViewer } from "./together-viewer.tsx";

/** The guest's membership of session 1, as their browser keeps it. */
const viewer = { membership: { sessionId: 1, credential: "viewer-credential" } };

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

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
  const left = currentNotice();
  if (left !== undefined) {
    dismissNotice(left);
  }
});

/**
 * A guest's page back in their session, which the server then ends for reason: signed in by token,
 * the Play page of the group's film (D384 §2); by link alone, their own page.
 */
async function endedFor(
  reason: TogetherEndReason,
  token: string | undefined,
): Promise<{ readonly mounted: Mounted; readonly services: Services }> {
  window.localStorage.setItem(
    "orpheum.together.viewer",
    JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
  );
  const server = fakeTogetherServer();
  const services = {
    ...servedBy({ "GET /together/mine": [200, { sessions: [] }] }, [], token),
    navigation: createNavigation(fakePlace(token === undefined ? "/together" : "/play/5")),
    openTogetherSocket: server.openSocket,
  };
  const mounted = await mount(
    withServices(
      token === undefined ? <TogetherViewer /> : <GroupFilmPage {...viewer} />,
      services,
    ),
  );
  // A guest back in the session taps through its first page (TapToContinue).
  await click(buttonWithText(mounted.container, "Tap to continue"));
  await server.accept();
  await server.say(welcome);
  await server.say({ type: "ended", reason });
  return { mounted, services };
}

// A page that said only "You can close this page" stopped a signed-in guest's app as their session
// ended (the owner, 2026-10-29): they go home, told why there.
describe("a Watch Together guest's page as their session ends", () => {
  it("takes a signed-in guest home, with a notice of why", async () => {
    const { mounted, services } = await endedFor("hostEnded", "signed-in-token");
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(currentNotice()?.text).toBe("The host ended the Watch Together session.");
    expect(mounted.container.textContent).not.toContain("You can close this page");
    expect(window.localStorage.getItem("orpheum.together.viewer")).toBeNull();
    mounted.unmount();
  });

  it("takes a signed-in guest home whose session the server no longer has, as after its restart, not to a page saying the address has no invitation (the owner's Mac, staging, 2026-10-29)", async () => {
    const server = fakeTogetherServer();
    const services = {
      ...servedBy({ "GET /together/mine": [200, { sessions: [] }] }, [], "signed-in-token"),
      navigation: createNavigation(fakePlace("/together")),
      openTogetherSocket: server.openSocket,
    };
    const mounted = await mount(withServices(<TogetherViewer />, services));
    await settleUntil(() => services.navigation.route().screen === "home");
    expect(services.navigation.route()).toEqual({ screen: "home" });
    expect(currentNotice()?.text).toBe("The Watch Together session is no longer there.");
    expect(mounted.container.textContent).not.toContain("no invitation");
    mounted.unmount();
  });

  // Principles §28: a failed ask took the guest home saying the session was no longer there.
  it("says a failed ask for a signed-in guest's sessions, keeping them here, and asks again by its button", async () => {
    const server = fakeTogetherServer();
    const services = {
      ...servedBy(
        {
          "GET /together/mine": [refusal(503, "unavailable"), [200, { sessions: [] }]],
        },
        [],
        "signed-in-token",
      ),
      navigation: createNavigation(fakePlace("/together")),
      openTogetherSocket: server.openSocket,
    };
    const mounted = await mount(withServices(<TogetherViewer />, services));
    await settleUntil(() => mounted.container.querySelector("[role=alert]") !== null);
    expect(services.navigation.route().screen).toBe("together");
    expect(currentNotice()).toBeUndefined();
    await click(buttonWithText(mounted.container, "Try again"));
    await settleUntil(() => services.navigation.route().screen === "home");
    expect(currentNotice()?.text).toBe("The Watch Together session is no longer there.");
    mounted.unmount();
  });

  it("takes a signed-in guest home whose Leave the server answers with no such session", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const server = fakeTogetherServer();
    const services = {
      ...servedBy(
        {
          "GET /together/mine": [200, { sessions: [] }],
          "POST /together/leave": refusal(404, "not_found"),
        },
        [],
        "signed-in-token",
      ),
      navigation: createNavigation(fakePlace("/play/5")),
      openTogetherSocket: server.openSocket,
    };
    const mounted = await mount(withServices(<GroupFilmPage {...viewer} />, services));
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await server.accept();
    await server.say(welcome);
    await click(buttonWithText(mounted.container, "Leave"));
    await settleUntil(() => services.navigation.route().screen === "home");
    expect(currentNotice()?.text).toBe("The Watch Together session is over.");
    expect(window.localStorage.getItem("orpheum.together.viewer")).toBeNull();
    mounted.unmount();
  });

  it("keeps a signed-in guest sent away for the host's connection on the page, with Join again (D286 §5)", async () => {
    const { mounted, services } = await endedFor("uploadFull", "signed-in-token");
    expect(services.navigation.route()).toEqual({ screen: "play", fileId: 5 });
    expect(currentNotice()).toBeUndefined();
    expect(buttonWithText(mounted.container, "Join again")).toBeDefined();
    expect(window.localStorage.getItem("orpheum.together.viewer")).not.toBeNull();
    mounted.unmount();
  });

  it("tells a guest by link alone their session is over on their page loaded again, not that it has no invitation (staging, 2026-10-29)", async () => {
    const { mounted } = await endedFor("gone", undefined);
    mounted.unmount();
    // The page loaded again, as an update reloads it: the membership is forgotten, the link spent.
    const again = await mount(
      withServices(<TogetherViewer />, {
        ...servedBy({}, [], undefined),
        navigation: createNavigation(fakePlace("/together")),
        openTogetherSocket: fakeTogetherServer().openSocket,
      }),
    );
    expect(again.container.textContent).toContain("This session is over. You can close this page.");
    expect(again.container.textContent).not.toContain("no invitation");
    again.unmount();
  });

  it("asks a guest by link alone on their page again for a link opened after their session ended", async () => {
    const { mounted } = await endedFor("gone", undefined);
    mounted.unmount();
    const again = await mount(
      withServices(<TogetherViewer />, {
        ...servedBy({ "POST /together/join/check": [204, undefined] }, [], undefined),
        navigation: createNavigation(fakePlace("/together#another-secret")),
        openTogetherSocket: fakeTogetherServer().openSocket,
      }),
    );
    expect(buttonWithText(again.container, "Join")).toBeDefined();
    again.unmount();
  });

  it("tells a guest by link alone on the page itself, having nowhere else to go", async () => {
    const { mounted, services } = await endedFor("ejected", undefined);
    expect(services.navigation.route()).toEqual({ screen: "together" });
    expect(currentNotice()).toBeUndefined();
    expect(mounted.container.textContent).toContain(
      "The host removed you from the session. You can close this page.",
    );
    mounted.unmount();
  });
});

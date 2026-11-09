// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { beforeEach, describe, expect, it } from "vitest";

import { App } from "./app.tsx";
import { createNavigation } from "./navigation.ts";
import {
  buttonWithText,
  click,
  enter,
  mount,
  servedBy,
  submit,
  type Answers,
  type Heard,
} from "./test-dom.tsx";
import { fakePlace, withServices } from "./test-services.tsx";
import { fakeTogetherServer, type FakeTogetherServer } from "./test-together.ts";
import { TogetherViewer } from "./together-viewer.tsx";

const joined = {
  sessionId: 1,
  memberId: 2,
  membershipCredential: "viewer-credential",
  locator: "viewer-locator",
};

const welcome = {
  type: "welcome",
  seq: 1,
  sessionId: 1,
  memberId: 2,
  deviceId: 8,
  group: { state: "idle", positionMs: 0, rate: 1, fileId: 0 },
  roster: [
    { id: 1, name: "Karl", isHost: true, devices: [{ id: 3, role: "video" }] },
    { id: 2, name: "Gran", devices: [{ id: 8, role: "video" }] },
  ],
};

/** The viewer's page at address, served by answers and a fake Watch Together server. */
function viewerServices(
  address: string,
  answers: Answers,
  token?: string,
): {
  services: ReturnType<typeof servedBy>;
  heard: Heard[];
  server: FakeTogetherServer;
  place: ReturnType<typeof fakePlace>;
} {
  const heard: Heard[] = [];
  const server = fakeTogetherServer();
  const place = fakePlace(address);
  return {
    services: {
      ...servedBy(answers, heard, token),
      navigation: createNavigation(place),
      openTogetherSocket: server.openSocket,
    },
    heard,
    server,
    place,
  };
}

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
});

describe("a Watch Together viewer's page (D246 §3, §6)", () => {
  it("comes back as the same member by the link it joined by, after the page was closed (D267 §1)", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential", link: "link-secret" }),
    );
    const { services, heard, server } = viewerServices("/together#link-secret", {});
    const mounted = await mount(withServices(<TogetherViewer />, services));
    // A guest back in the session taps through its first page (TapToContinue).
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await server.accept();
    expect(server.heard[0]).toMatchObject({ type: "hello", credential: "viewer-credential" });
    expect(heard).toEqual([]); // no join: the spent link is not used again
    expect(services.navigation.fragment()).toBe("");
    mounted.unmount();
  });

  it("has back, signed in, the session its person joined on another device (D267 §2)", async () => {
    const { services, server } = viewerServices(
      "/together",
      {
        "GET /me/together": [
          200,
          {
            sessions: [
              {
                sessionId: 1,
                memberId: 2,
                isHost: false,
                membershipCredential: "phone-credential",
              },
            ],
          },
        ],
      },
      "signed-in-token",
    );
    const mounted = await mount(withServices(<TogetherViewer />, services));
    await server.accept();
    expect(server.heard[0]).toMatchObject({ type: "hello", credential: "phone-credential" });
    mounted.unmount();
  });

  it("leaves, and says so, forgetting the membership", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const { services, heard } = viewerServices("/together", {
      "POST /together/leave": [204, undefined],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    // A guest back in the session taps through its first page (TapToContinue).
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await click(buttonWithText(mounted.container, "Leave"));
    expect(heard.map((each) => each.methodAndPath)).toEqual(["POST /together/leave"]);
    expect(mounted.container.textContent).toContain("You left the session.");
    expect(window.localStorage.getItem("orpheum.together.viewer")).toBeNull();
    mounted.unmount();
  });

  it("tells a viewer the host ended the session", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const { services, server } = viewerServices("/together", {});
    const mounted = await mount(withServices(<TogetherViewer />, services));
    // A guest back in the session taps through its first page (TapToContinue).
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await server.accept();
    await server.say(welcome);
    await server.say({ type: "ended", reason: "hostEnded" });
    expect(mounted.container.textContent).toContain("The host ended the session.");
    mounted.unmount();
  });

  it("tells a viewer the server closed a session idle for hours (D346)", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const { services, server } = viewerServices("/together", {});
    const mounted = await mount(withServices(<TogetherViewer />, services));
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await server.accept();
    await server.say(welcome);
    await server.say({ type: "ended", reason: "idle" });
    expect(mounted.container.textContent).toContain(
      "The session was closed because nothing had happened in it for hours.",
    );
    mounted.unmount();
  });

  it("tells a viewer sent away for the host's connection why, keeps them a member, and lets them back in", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const { services, server } = viewerServices("/together", {});
    const mounted = await mount(withServices(<TogetherViewer />, services));
    // A guest back in the session taps through its first page (TapToContinue).
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await server.accept();
    await server.say(welcome);
    await server.say({ type: "ended", reason: "uploadFull" });
    expect(mounted.container.textContent).toContain(
      "The host's connection can't carry everyone right now.",
    );
    expect(window.localStorage.getItem("orpheum.together.viewer")).not.toBeNull();
    await click(buttonWithText(mounted.container, "Join again"));
    expect(mounted.container.textContent).not.toContain("can't carry everyone");
    expect(buttonWithText(mounted.container, "Leave")).toBeDefined();
    mounted.unmount();
  });

  it("joins by a new link pasted into a tab still on the page, which does not load again", async () => {
    window.localStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const { services, server, heard, place } = viewerServices("/together", {
      "POST /together/join/check": [204, undefined],
      "POST /together/join": [200, { ...joined, sessionId: 2, membershipCredential: "second" }],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    // A guest back in the session taps through its first page (TapToContinue).
    await click(buttonWithText(mounted.container, "Tap to continue"));
    await server.accept();
    await server.say(welcome);
    await server.say({ type: "ended", reason: "hostEnded" });
    // Only the part after # changed: the browser says so, and the page stays.
    await act(async () => {
      place.back("/together#second-secret");
      await Promise.resolve();
    });
    const join = buttonWithText(mounted.container, "Join");
    expect(join?.disabled).toBe(false);
    await enter(mounted.container.querySelector("input"), "Gran");
    await submit(mounted.container.querySelector("form"));
    expect(heard).toEqual([
      { methodAndPath: "POST /together/join/check", body: { secret: "second-secret" } },
      { methodAndPath: "POST /together/join", body: { secret: "second-secret", name: "Gran" } },
    ]);
    mounted.unmount();
  });

  it("fills in the name of someone signed in here, which they may change before joining", async () => {
    const { services, heard } = viewerServices(
      "/together#link-secret",
      {
        "POST /together/join/check": [204, undefined],
        "GET /me/together": [200, { sessions: [] }],
        "GET /me": [200, { id: 4, name: "Partner", isAdmin: false, canHostWatchTogether: false }],
        "POST /together/join": [200, joined],
      },
      "partner-token",
    );
    const mounted = await mount(withServices(<TogetherViewer />, services));
    const input = mounted.container.querySelector("input");
    expect(input?.value).toBe("Partner");
    await enter(input, "Gran");
    await submit(mounted.container.querySelector("form"));
    expect(heard.find((h) => h.methodAndPath === "POST /together/join")?.body).toEqual({
      secret: "link-secret",
      name: "Gran",
    });
    mounted.unmount();
  });

  it("fills in no name that arrives once its person is in the field (check-pr, 2026-10-15)", async () => {
    const { services } = viewerServices(
      "/together#link-secret",
      {
        "POST /together/join/check": [204, undefined],
        "GET /me": [200, { id: 4, name: "Partner", isAdmin: false, canHostWatchTogether: false }],
      },
      "partner-token",
    );
    let answerMe = (): void => undefined;
    const meAnswered = new Promise<void>((resolve) => {
      answerMe = resolve;
    });
    services.api.use({
      async onRequest({ request }) {
        if (new URL(request.url).pathname.endsWith("/me")) {
          await meAnswered;
        }
        return undefined;
      },
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    const input = mounted.container.querySelector("input");
    expect(input?.value).toBe("");
    act(() => {
      input?.focus();
    });
    // Their own name arrives once they are in the field: it goes nowhere, as what they type would
    // have gone after it.
    answerMe();
    await act(async () => {
      await new Promise((resolve) => setTimeout(resolve, 20));
    });
    expect(input?.value).toBe("");
    mounted.unmount();
  });

  it("asks no one not signed in who they are, and leaves the name empty", async () => {
    const { services, heard } = viewerServices("/together#link-secret", {
      "POST /together/join/check": [204, undefined],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    expect(mounted.container.querySelector("input")?.value).toBe("");
    expect(heard.map((h) => h.methodAndPath)).not.toContain("GET /me");
    mounted.unmount();
  });

  it("with no link in the address and no session, says so and offers nothing to fill in", async () => {
    const { services } = viewerServices("/together", {});
    const mounted = await mount(withServices(<TogetherViewer />, services));
    expect(mounted.container.textContent).toContain("This address has no invitation in it.");
    expect(mounted.container.querySelector("input")).toBeNull();
    expect(mounted.container.querySelector("button")).toBeNull();
    mounted.unmount();
  });

  it("tells a browser that cannot keep in step to update, before anyone joins (D250 §7)", async () => {
    const { services, heard } = viewerServices("/together#link-secret", {});
    const mounted = await mount(
      withServices(<TogetherViewer />, { ...services, canFollowAGroup: () => false }),
    );
    expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain(
      "update to iOS 17.1 or later",
    );
    expect(mounted.container.querySelector("input")).toBeNull();
    expect(mounted.container.querySelector("button")).toBeNull();
    expect(heard).toEqual([]);
    mounted.unmount();
  });

  it("is a page of its own for someone not signed in: no sign-in, no menu", async () => {
    const { services } = viewerServices("/together#link-secret", {
      "POST /together/join/check": [204, undefined],
    });
    const mounted = await mount(withServices(<App />, services));
    expect(mounted.container.textContent).toContain("Join");
    expect(mounted.container.querySelector("nav")).toBeNull();
    expect(mounted.container.textContent).not.toContain("Sign in");
    mounted.unmount();
  });
});

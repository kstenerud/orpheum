// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { stallAfterMs } from "@orpheum/player";
import { act } from "react";
import { beforeEach, describe, expect, it } from "vitest";

import { App } from "./app.tsx";
import { createNavigation } from "./navigation.ts";
import {
  buttonWithText,
  click,
  enter,
  fire,
  mount,
  servedBy,
  submit,
  type Answers,
  type Heard,
} from "./test-dom.tsx";
import { directDecision } from "./test-fixtures.ts";
import { fakePlace, withServices } from "./test-services.tsx";
import { fakeTogetherServer, type FakeTogetherServer } from "./test-together.ts";
import { TogetherViewer } from "./together-viewer.tsx";

const joined = { sessionId: 1, memberId: 2, membershipCredential: "viewer-credential" };

const welcome = {
  type: "welcome",
  seq: 1,
  sessionId: 1,
  memberId: 2,
  deviceId: 8,
  group: { state: "Idle", positionMs: 0, rate: 1, fileId: 0 },
  roster: [
    { id: 1, name: "Karl", isHost: true, devices: [{ id: 3, role: "playback" }] },
    { id: 2, name: "Gran", devices: [{ id: 8, role: "playback" }] },
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
  it("joins by the link with a name, then asks about speakers, then shows black until a film is on", async () => {
    const { services, heard, server } = viewerServices("/together#link-secret", {
      "POST /together/join": [200, joined],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    await enter(mounted.container.querySelector("input"), "Gran");
    await submit(mounted.container.querySelector("form"));
    expect(heard).toEqual([
      { methodAndPath: "POST /together/join", body: { secret: "link-secret", name: "Gran" } },
    ]);
    // The secret is spent: it is in neither the address nor the history.
    expect(services.navigation.fragment()).toBe("");
    expect(window.sessionStorage.getItem("orpheum.together.viewer")).toContain("viewer-credential");
    // Asked at once, before anything plays, and kept in this browser.
    expect(mounted.container.textContent).toContain("How does this device play sound?");
    await server.accept();
    expect(server.heard[0]).toMatchObject({
      type: "hello",
      credential: "viewer-credential",
      role: "playback",
    });
    await click(
      [...mounted.container.querySelectorAll("button")].find((button) =>
        button.textContent.startsWith("Headphones"),
      ),
    );
    expect(window.localStorage.getItem("orpheum.together.audioOutput")).toBe("headphones");
    await server.say(welcome);
    const stage = mounted.container.querySelector('section[aria-label="The film"]');
    expect(stage).not.toBeNull();
    expect(stage?.querySelector("video")).toBeNull();
    expect(mounted.container.textContent).toContain("Here: Karl and Gran");
    expect(mounted.container.textContent).not.toContain("Sign out");
    mounted.unmount();
  });

  it("plays the film the host puts on, asked for by the membership, and names who did it", async () => {
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
    window.sessionStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const { services, heard, server } = viewerServices("/together", {
      "POST /together/playback": [200, directDecision()],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    // The speaker question was answered in this browser before: it is not asked again.
    expect(mounted.container.textContent).not.toContain("How does this device play sound?");
    await server.accept();
    await server.say(welcome);
    await server.say({
      type: "group",
      seq: 2,
      group: {
        state: "Paused",
        positionMs: 2_472_000,
        rate: 1,
        fileId: 5,
        event: "paused",
        byMemberId: 1,
      },
    });
    expect(heard).toEqual([
      {
        methodAndPath: "POST /together/playback",
        body: {
          playableFormats: services.playableFormats(),
          audioOutput: "receiver",
        },
      },
    ]);
    expect(mounted.container.querySelector("video")?.getAttribute("src")).toBe(
      "/api/v1/bytes/media/direct/abc",
    );
    expect(mounted.container.textContent).toContain("Karl paused at 41:12");
    mounted.unmount();
  });

  it("reports a viewer's stall by the membership, and moves to the lower quality offered (D247 §2)", async () => {
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
    window.sessionStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const offer = {
      url: "/api/v1/bytes/media/chunks/t/master.m3u8",
      startAtBps: 1_242_000,
      qualityKbps: 700,
      height: 432,
      isBelowTheLowest: false,
      soundCarriedGainDb: 0,
    };
    const { services, heard, server } = viewerServices("/together", {
      "POST /together/playback": [200, directDecision()],
      "POST /playback/t/report": [200, { lowerQuality: offer }],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    await server.accept();
    await server.say(welcome);
    await server.say({
      type: "group",
      seq: 2,
      group: { state: "Playing", positionMs: 60_000, rate: 1, fileId: 5 },
    });
    const video = mounted.container.querySelector("video");
    if (video === null) {
      throw new Error("no video");
    }
    // Playing along, it runs out of data.
    Object.defineProperty(video, "paused", { value: false });
    await act(async () => {
      video.dispatchEvent(new Event("playing"));
      video.dispatchEvent(new Event("waiting"));
      await new Promise((resolve) => setTimeout(resolve, stallAfterMs + 50));
    });
    const report = heard.find((each) => each.methodAndPath === "POST /playback/t/report");
    expect(report?.body).toMatchObject({ stalls: [{ positionMs: 0 }] });
    expect(mounted.container.textContent).toContain(
      "Your connection could not keep up, so this moves to a lower quality: 432p, 0.7 Mb/s.",
    );
    mounted.unmount();
  });

  it("tells a viewer the player's own words when a prepared film fails, asking nothing of the host's file", async () => {
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
    window.sessionStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const prepared = directDecision({
      delivery: "hls",
      url: "/api/v1/bytes/media/hls/abc/master.m3u8",
      profile: "remux",
      notDirectReasons: ["container"],
    });
    const { services, heard, server } = viewerServices("/together", {
      "POST /together/playback": [200, prepared],
      "GET /files/5/availability": [200, {}],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    await server.accept();
    await server.say(welcome);
    await server.say({
      type: "group",
      seq: 2,
      group: { state: "Paused", positionMs: 0, rate: 1, fileId: 5 },
    });
    await fire(mounted.container.querySelector("video"), new Event("error"));
    expect(heard.map((each) => each.methodAndPath)).toEqual(["POST /together/playback"]);
    mounted.unmount();
  });

  it("leaves, and says so, forgetting the membership", async () => {
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
    window.sessionStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const { services, heard } = viewerServices("/together", {
      "POST /together/leave": [204, undefined],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
    await click(buttonWithText(mounted.container, "Leave"));
    expect(heard.map((each) => each.methodAndPath)).toEqual(["POST /together/leave"]);
    expect(mounted.container.textContent).toContain("You left the session.");
    expect(window.sessionStorage.getItem("orpheum.together.viewer")).toBeNull();
    mounted.unmount();
  });

  it("tells a viewer the host ended the session", async () => {
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
    window.sessionStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const { services, server } = viewerServices("/together", {});
    const mounted = await mount(withServices(<TogetherViewer />, services));
    await server.accept();
    await server.say(welcome);
    await server.say({ type: "ended", reason: "hostEnded" });
    expect(mounted.container.textContent).toContain("The host ended the session.");
    mounted.unmount();
  });

  it("joins by a new link pasted into a tab still on the page, which does not load again", async () => {
    window.localStorage.setItem("orpheum.together.audioOutput", "receiver");
    window.sessionStorage.setItem(
      "orpheum.together.viewer",
      JSON.stringify({ sessionId: 1, credential: "viewer-credential" }),
    );
    const { services, server, heard, place } = viewerServices("/together", {
      "POST /together/join": [200, { ...joined, sessionId: 2, membershipCredential: "second" }],
    });
    const mounted = await mount(withServices(<TogetherViewer />, services));
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
      { methodAndPath: "POST /together/join", body: { secret: "second-secret", name: "Gran" } },
    ]);
    mounted.unmount();
  });

  it("fills in the name of someone signed in here, which they may change before joining", async () => {
    const { services, heard } = viewerServices(
      "/together#link-secret",
      {
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

  it("asks no one not signed in who they are, and leaves the name empty", async () => {
    const { services, heard } = viewerServices("/together#link-secret", {});
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
    const { services } = viewerServices("/together#link-secret", {});
    const mounted = await mount(withServices(<App />, services));
    expect(mounted.container.textContent).toContain("Join");
    expect(mounted.container.querySelector("nav")).toBeNull();
    expect(mounted.container.textContent).not.toContain("Sign in");
    mounted.unmount();
  });
});

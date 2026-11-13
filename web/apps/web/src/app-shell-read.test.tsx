// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { components } from "@orpheum/sdk";
import { describe, expect, it } from "vitest";

import { App } from "./app.tsx";
import { mount, settleUntil } from "./test-dom.tsx";
import { testServices, withServices } from "./test-services.tsx";

/** What the shell reads as it starts, as GET /me?shell=true and a sign-in carry it (D476). */
const shell = {
  clientSettings: {
    screenRefreshFirstWaitSeconds: 2,
    screenRefreshLongestWaitSeconds: 30,
    screenRefreshFailedWaitSeconds: 5,
    interestSaidEverySeconds: 15,
    shouldShowDeveloperPages: false,
    shouldOfferDeviceTests: false,
    readyAheadOfNextChunkPercent: 150,
  },
  libraries: [{ id: 1, name: "Films", kind: "film" }],
  needsAttention: {
    rootCount: 0,
    diskCount: 0,
    waitingRootCount: 0,
    unreadCount: 0,
    matchCount: 0,
    seriesReadWronglyCount: 0,
    filmsWithoutSoundCount: 0,
  },
  update: { isAvailable: false },
};

/** An admin, as GET /me?shell=true answers, the shell's parts with it. */
const admin = {
  id: 1,
  name: "owner",
  role: "admin",
  isAdmin: true,
  canHostWatchTogether: false,
  canManageAccount: true,
  hasNoLibrary: false,
  showsDebugTools: false,
  shell,
} as unknown as components["schemas"]["UserBody"];

/** The paths the shell reads apart, which its one read carries. */
const carriedApart = ["/client-settings", "/libraries", "/needs-attention", "/system/update"];

/**
 * A server that answers GET /me as meAnswer says (never, for "never"), Home's rows empty and every
 * other read in a way the page lives with, each request heard as its path and query.
 */
function serverHeard(meAnswer: "now" | "never"): {
  readonly heard: string[];
  readonly send: (request: Request) => Promise<Response>;
} {
  const heard: string[] = [];
  const json = (body: unknown): Response =>
    new Response(JSON.stringify(body), {
      status: 200,
      headers: { "Content-Type": "application/json" },
    });
  const pending = (request: Request): Promise<Response> =>
    new Promise<Response>((_resolve, reject) => {
      request.signal.addEventListener("abort", () => {
        reject(new DOMException("aborted", "AbortError"));
      });
    });
  const send = async (request: Request): Promise<Response> => {
    const url = new URL(request.url);
    const path = url.pathname.replace(/^\/api\/v1/u, "");
    heard.push(`${request.method} ${path}${url.search}`);
    if (request.method === "GET" && path === "/me") {
      return meAnswer === "now" ? json(admin) : await pending(request);
    }
    if (request.method === "GET" && path === "/home") {
      return json({ continueWatching: [], newestFilms: [], newestSeries: [] });
    }
    return await pending(request);
  };
  return { heard, send };
}

describe("the shell's one read (D476, principles §30)", () => {
  it("is GET /me with what the shell reads as it starts, and the shell asks none of those apart", async () => {
    const { heard, send } = serverHeard("now");
    const mounted = await mount(withServices(<App />, testServices("token", "/", send)));
    await settleUntil(() => heard.includes("GET /home"));
    await settleUntil(() => mounted.container.querySelector(".status-tray") !== null);
    expect(heard.filter((each) => /^GET \/me(\?|$)/u.test(each))).toEqual(["GET /me?shell=true"]);
    expect(
      heard.filter((each) => carriedApart.some((path) => each.startsWith(`GET ${path}`))),
    ).toEqual([]);
    mounted.unmount();
  });

  it("is not made after a sign-in, whose answer stands for it", async () => {
    const { heard, send } = serverHeard("now");
    const services = testServices(undefined, "/", send);
    services.session.signIn("token", { user: admin });
    const mounted = await mount(withServices(<App />, services));
    await settleUntil(() => heard.includes("GET /home"));
    expect(heard.filter((each) => /^GET \/me(\?|$)/u.test(each))).toEqual([]);
    expect(
      heard.filter((each) => carriedApart.some((path) => each.startsWith(`GET ${path}`))),
    ).toEqual([]);
    mounted.unmount();
  });

  it("does not hold back the screen's own read, which starts alongside it", async () => {
    const { heard, send } = serverHeard("never");
    // A session its last read said is a person's, as a reload of one is.
    const services = testServices("token", "/", send);
    services.session.noteKnownAs("person");
    const mounted = await mount(withServices(<App />, services));
    await settleUntil(() => heard.includes("GET /home"));
    expect(heard).toContain("GET /me?shell=true");
    mounted.unmount();
  });
});

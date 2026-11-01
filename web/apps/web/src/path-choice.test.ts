// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { reachCore, type Target } from "./discovery-start.ts";
import {
  browserFamily,
  pairsSettleMs,
  pathChoiceOf,
  reportPathChoice,
  type PathChoice,
} from "./path-choice.ts";

const lan = "https://192-168-1-199.k7xq2.zone.example:8443";
const ipv6 = "https://2a00-1.k7xq2.zone.example";

describe("why a page took its path to the core (local-path-2026-10-27.md §4 item 1)", () => {
  it("names the browser's family from its user agent", () => {
    expect(
      browserFamily("Mozilla/5.0 (X11; Linux x86_64; rv:158.0) Gecko/20100101 Firefox/158.0"),
    ).toBe("firefox");
    expect(
      browserFamily(
        "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/146.0.0.0 Safari/537.36",
      ),
    ).toBe("chrome");
    expect(
      browserFamily(
        "Mozilla/5.0 (Macintosh; Intel Mac OS X 10_15_7) AppleWebKit/605.1.15 (KHTML, like Gecko) Version/19.0 Safari/605.1.15",
      ),
    ).toBe("safari");
    expect(
      browserFamily(
        "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 (KHTML, like Gecko) HeadlessChrome/146.0.0.0 Safari/537.36",
      ),
    ).toBe("chrome"); // not safari, as the core logged it (2026-11-01)
    expect(browserFamily("curl/8.0")).toBe("other");
  });

  it("says of each name what came of it, and why a LAN name was left out", () => {
    const firefox = "Mozilla/5.0 (X11; Linux x86_64; rv:158.0) Gecko/20100101 Firefox/158.0";
    const chrome =
      "Mozilla/5.0 (X11; Linux x86_64) AppleWebKit/537.36 Chrome/146.0.0.0 Safari/537.36";
    const race = (userAgent: string, localNetwork: "denied" | "prompt") =>
      pathChoiceOf({
        directUrls: [lan, ipv6],
        lanUrls: [lan],
        raced: [{ url: ipv6, outcome: "answered" }],
        localNetwork,
        userAgent,
        directOrigin: undefined,
      });
    expect(race(firefox, "denied")).toEqual({
      browser: "firefox",
      isLanNameUsed: false,
      localNetwork: "denied",
      directOffered: 2,
      lanOffered: 1,
      names: [
        { name: lan, outcome: "left out: this browser fails a page's requests to a LAN name" },
        { name: ipv6, outcome: "answered" },
      ],
      reached: "webrtc",
    });
    expect(race(chrome, "denied").names[0]?.outcome).toBe(
      "left out: the local-network permission is denied",
    );
    expect(race(chrome, "prompt").names[0]?.outcome).toBe(
      "left out: asked apart, to raise the browser's local-network prompt",
    );
  });

  it("is what reachCore chose by: the names offered, each one's outcome, and the path taken", async () => {
    const target: Target = {
      coreId: "k7xq2",
      fingerprint: "Vkdap1RjR0wChd9dvyvKtz2mUTWIOem3dIGy6rEHcIw",
      held: { bootstrapToken: "a-bootstrap-token" },
      key: new Uint8Array(32),
      iceServers: [],
      directUrls: [lan, ipv6],
      lanUrls: [lan],
    };
    const reaching = await reachCore(
      {
        clientApi: "https://discovery.example/client/v1",
        fetch: () => Promise.resolve(new Response("{}", { status: 200 })),
      },
      target,
      {
        state: () => Promise.resolve("denied"),
        onGranted: () => () => undefined,
        onAsking: () => () => undefined,
      },
    );
    expect(reaching.choice).toMatchObject({
      localNetwork: "denied",
      directOffered: 2,
      lanOffered: 1,
      names: [
        { name: lan, outcome: "left out: the local-network permission is denied" },
        { name: ipv6, outcome: "answered" },
      ],
      reached: `direct ${ipv6}`,
    });
  });
});

describe("telling the core how the page chose its path", () => {
  const choice: PathChoice = {
    browser: "firefox",
    isLanNameUsed: false,
    localNetwork: "denied",
    directOffered: 1,
    lanOffered: 1,
    names: [{ name: lan, outcome: "left out: this browser fails a page's requests to a LAN name" }],
    reached: "webrtc",
  };

  /** A send that keeps each request's path, Authorization and body. */
  const keeping = (sent: { path: string; authorization: string | null; body: unknown }[]) => {
    return async (request: Request): Promise<Response> => {
      sent.push({
        path: new URL(request.url).pathname,
        authorization: request.headers.get("Authorization"),
        body: await request.json(),
      });
      return new Response(null, { status: 204 });
    };
  };

  it("over WebRTC, sends the choice with the connection's pairs once they have settled", async () => {
    const sent: { path: string; authorization: string | null; body: unknown }[] = [];
    const waited: number[] = [];
    const signedIn: { token?: string } = {};
    const pair = {
      state: "failed",
      browser: "host udp 3f2a.local:51233",
      core: "host udp 192.168.1.199:8443",
    };
    await reportPathChoice(
      keeping(sent),
      {
        token: () => signedIn.token,
        choice,
        transport: { candidatePairs: () => Promise.resolve([pair]) },
      },
      (ms) => {
        waited.push(ms);
        // A hand-off's page takes its session in the meantime.
        signedIn.token = "a-token";
        return Promise.resolve();
      },
    );
    expect(waited).toEqual([pairsSettleMs]);
    expect(sent).toEqual([
      {
        path: "/api/v1/client-paths",
        authorization: "Bearer a-token",
        body: { ...choice, webrtcPairs: [pair] },
      },
    ]);
  });

  it("sends nothing for a page with no session, and no pairs for a direct path", async () => {
    const sent: { path: string; authorization: string | null; body: unknown }[] = [];
    const now = (): Promise<void> => Promise.resolve();
    await reportPathChoice(keeping(sent), { token: () => undefined, choice }, now);
    expect(sent).toEqual([]);
    await reportPathChoice(
      keeping(sent),
      { token: () => "a-token", choice: { ...choice, reached: `direct ${ipv6}` } },
      now,
    );
    expect(sent.map((each) => each.body)).toEqual([{ ...choice, reached: `direct ${ipv6}` }]);
  });
});

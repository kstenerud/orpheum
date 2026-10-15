// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { isNetworkFailure, namingFailures, PathKeeper } from "./path-keeper.ts";

/** A path the test sends on: it answers with its name, or fails as a blocked network request does. */
interface FakePath {
  readonly name: string;
  isBlocked: boolean;
}

function keeperOver(first: FakePath, others: FakePath[], used: string[]): PathKeeper<FakePath> {
  return new PathKeeper(first, {
    sendOn: (path) => (request) =>
      path.isBlocked
        ? Promise.reject(new TypeError("NetworkError when attempting to fetch resource."))
        : Promise.resolve(new Response(`${path.name} ${new URL(request.url).pathname}`)),
    another: (failed) => {
      const next = others.find((path) => path !== failed && !path.isBlocked);
      return next === undefined
        ? Promise.reject(new Error("no path answers"))
        : Promise.resolve(next);
    },
    onUse: (path) => {
      used.push(path.name);
    },
  });
}

describe("the path a page's requests take (webrtc-transport.md W2c items 5 and 6)", () => {
  it("moves to another path when the one in use fails for the network's sake, and sends again there", async () => {
    const lan = { name: "lan", isBlocked: false };
    const ipv6 = { name: "ipv6", isBlocked: false };
    const used: string[] = [];
    const keeper = keeperOver(lan, [lan, ipv6], used);
    expect(await (await keeper.fetch(new Request("https://page.example/home"))).text()).toBe(
      "lan /home",
    );
    // Firefox blocks the LAN name until its prompt is answered: every request fails at once.
    lan.isBlocked = true;
    const answers = await Promise.all(
      ["/titles", "/me", "/session"].map(async (path) => {
        const answer = await keeper.fetch(
          new Request(`https://page.example${path}`, { method: "POST", body: "x" }),
        );
        return await answer.text();
      }),
    );
    expect(answers).toEqual(["ipv6 /titles", "ipv6 /me", "ipv6 /session"]);
    expect(used).toEqual(["lan", "ipv6"]);
    // Allowed later, the LAN is found better and moved to between requests.
    lan.isBlocked = false;
    keeper.moveTo(lan);
    expect(await (await keeper.fetch(new Request("https://page.example/home"))).text()).toBe(
      "lan /home",
    );
    expect(used).toEqual(["lan", "ipv6", "lan"]);
  });

  it("never sends again a request that failed for another reason, or that the page aborted", async () => {
    const used: string[] = [];
    const keeper = new PathKeeper("only", {
      sendOn: () => () => Promise.reject(new Error("the core's response head is not one")),
      another: () => Promise.reject(new Error("never asked")),
      onUse: (path) => {
        used.push(path);
      },
    });
    await expect(keeper.fetch(new Request("https://page.example/x"))).rejects.toThrow(
      "head is not one",
    );
    const aborting = new AbortController();
    aborting.abort();
    const aborted = new PathKeeper("only", {
      sendOn: () => () => Promise.reject(new TypeError("aborted")),
      another: () => Promise.reject(new Error("never asked")),
      onUse: () => undefined,
    });
    await expect(
      aborted.fetch(new Request("https://page.example/x", { signal: aborting.signal })),
    ).rejects.toThrow("aborted");
    expect(used).toEqual(["only"]);
  });
});

describe("a failure on a path", () => {
  it("names the request and the path it failed on, and stays a network failure", async () => {
    const send = namingFailures(
      () => Promise.reject(new TypeError("NetworkError when attempting to fetch resource.")),
      "the core's name, https://lan.example:8443",
    );
    const failed = await send(new Request("https://page.example/api/v1/titles?kind=film")).catch(
      (problem: unknown) => problem,
    );
    expect(isNetworkFailure(failed)).toBe(true);
    expect((failed as Error).message).toBe(
      "NetworkError when attempting to fetch resource. (GET /api/v1/titles over the core's name, https://lan.example:8443)",
    );
  });
});

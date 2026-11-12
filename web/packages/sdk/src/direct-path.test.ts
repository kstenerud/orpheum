// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it, vi } from "vitest";

import {
  directFetch,
  endingOf,
  firstDirectPath,
  opensItsAPIToThisPage,
  type DirectOutcome,
} from "./direct-path.ts";

/** A fetch whose hosts answer as named: a status, or never. */
function hosts(answers: Record<string, number | "never">, asked: string[] = []): typeof fetch {
  return (input, init) => {
    const url = input instanceof Request ? input.url : input.toString();
    asked.push(`${init?.method ?? "GET"} ${url} ${init?.credentials ?? ""}`);
    const answer = answers[new URL(url).host];
    if (answer === undefined) {
      return Promise.reject(new TypeError("Failed to fetch"));
    }
    if (answer === "never") {
      return new Promise((_resolve, reject) => {
        init?.signal?.addEventListener("abort", () => {
          reject(new DOMException("timed out", "TimeoutError"));
        });
      });
    }
    return Promise.resolve(new Response(null, { status: answer }));
  };
}

describe("a core's direct paths (W2c)", () => {
  it("takes the first name whose core answers, whatever its order, and none when none does", async () => {
    const send = hosts({ "lan.core.example": "never", "v4.core.example": 200 });
    expect(
      await firstDirectPath(["https://lan.core.example", "https://v4.core.example:8443/x"], send, {
        answerMs: 50,
      }),
    ).toBe(undefined);
    expect(
      await firstDirectPath(["https://lan.core.example", "https://v4.core.example"], send, {
        answerMs: 50,
      }),
    ).toBe("https://v4.core.example");
    expect(
      await firstDirectPath(["https://gone.core.example"], send, { answerMs: 50 }),
    ).toBeUndefined();
    expect(await firstDirectPath([], send, { answerMs: 50 })).toBeUndefined();
  });

  it("takes the LAN's name when it answers shortly after a public one, as at home", async () => {
    const delayed =
      (answers: Record<string, number>): typeof fetch =>
      (input) =>
        new Promise((resolve) => {
          const host = new URL(input instanceof Request ? input.url : input.toString()).host;
          setTimeout(() => {
            resolve(new Response(null, { status: 200 }));
          }, answers[host] ?? 0);
        });
    // The public IPv6 name, through the VPS, answers first; the LAN's 20 ms after: the LAN's wins.
    expect(
      await firstDirectPath(
        ["https://lan.core.example", "https://v6.core.example"],
        delayed({ "lan.core.example": 30, "v6.core.example": 10 }),
        { answerMs: 500, betterMs: 200 },
      ),
    ).toBe("https://lan.core.example");
    // Away from home the LAN's never answers: the public one is taken once the grace is over.
    const away = hosts({ "lan.core.example": "never", "v6.core.example": 200 });
    const started = Date.now();
    expect(
      await firstDirectPath(["https://lan.core.example", "https://v6.core.example"], away, {
        answerMs: 500,
        betterMs: 50,
      }),
    ).toBe("https://v6.core.example");
    expect(Date.now() - started).toBeLessThan(400);
  });

  it("tells what came of each name once the race is decided, in the discovery's order", async () => {
    const send = hosts({
      "lan.core.example": "never",
      "v6.core.example": 200,
      "v4.core.example": 502,
    });
    const told: unknown[] = [];
    const urls = [
      "https://lan.core.example",
      "https://v6.core.example",
      "https://gone.core.example",
    ];
    expect(
      await firstDirectPath(urls, send, {
        answerMs: 50,
        betterMs: 10,
        tell: (outcomes) => {
          told.push(outcomes);
        },
      }),
    ).toBe("https://v6.core.example");
    // The LAN name had not answered when the grace ran out; the race went on without it.
    // Each name that answered or failed says how long after the race began (D414 §4).
    const inTime = expect.any(Number) as number;
    expect(told).toEqual([
      [
        { url: "https://lan.core.example", outcome: "not waited for" },
        { url: "https://v6.core.example", outcome: "answered", afterMs: inTime },
        {
          url: "https://gone.core.example",
          outcome: "failed: TypeError: Failed to fetch",
          afterMs: inTime,
        },
      ],
    ]);
    told.length = 0;
    await firstDirectPath(["https://lan.core.example", "https://v4.core.example"], send, {
      answerMs: 50,
      tell: (outcomes) => {
        told.push(outcomes);
      },
    });
    expect(told).toEqual([
      [
        {
          url: "https://lan.core.example",
          outcome: "failed: no answer within 50 ms",
          afterMs: inTime,
        },
        {
          url: "https://v4.core.example",
          outcome: "failed: Error: https://v4.core.example answered 502",
          afterMs: inTime,
        },
      ],
    ]);
    // The name that waited out its 50 ms says so.
    expect((told[0] as readonly DirectOutcome[])[0]?.afterMs).toBeGreaterThanOrEqual(45);
  });

  it("tells a name that failed at once from one that never answered or answered a failure", async () => {
    const send = hosts({ "lan.core.example": "never", "v4.core.example": 502 });
    let told: readonly DirectOutcome[] = [];
    const urls = ["https://lan.core.example", "https://v4.core.example", "https://v6.core.example"];
    await firstDirectPath(urls, send, {
      answerMs: 20,
      tell: (outcomes) => {
        told = outcomes;
      },
    });
    // No route for a name's family fails before any wait (D398 §2); a lost packet waits it out.
    expect(told.map((each) => endingOf(each.outcome))).toEqual([
      "noAnswer",
      "refused",
      "unreachable",
    ]);
    expect(endingOf("answered")).toBe("answered");
    expect(endingOf("not waited for")).toBe("notWaited");
  });

  it("sends the page's request to the core's name, path and query kept, with no cookie", async () => {
    const asked: string[] = [];
    const send = directFetch("https://v4.core.example", hosts({ "v4.core.example": 200 }, asked));
    const answer = await send(
      new Request("https://app.example/api/v1/me?x=1", {
        method: "POST",
        headers: new Headers([["Authorization", "Bearer t"]]),
        body: "{}",
      }),
    );
    expect(answer.status).toBe(200);
    expect(asked).toEqual(["POST https://v4.core.example/api/v1/me?x=1 omit"]);
  });
});

describe("whether a core opens its API to the page", () => {
  it("goes direct where the core answers its API to the page, refused or not, and not where the browser refuses the answer, as a core before D467 makes it refuse at the loader's origin", async () => {
    const answers = vi.fn(async () => await Promise.resolve(new Response("", { status: 401 })));
    expect(await opensItsAPIToThisPage("https://core.example:8443", answers)).toBe(true);
    expect(answers).toHaveBeenCalledWith("https://core.example:8443/api/v1/me", {
      mode: "cors",
      credentials: "omit",
    });
    const refuses = vi.fn(async () => await Promise.reject(new TypeError("Failed to fetch")));
    expect(await opensItsAPIToThisPage("https://core.example:8443", refuses)).toBe(false);
  });
});

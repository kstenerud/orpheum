// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { directFetch, firstDirectPath } from "./direct-path.ts";

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
      await firstDirectPath(
        ["https://lan.core.example", "https://v4.core.example:8443/x"],
        send,
        50,
      ),
    ).toBe(undefined);
    expect(
      await firstDirectPath(["https://lan.core.example", "https://v4.core.example"], send, 50),
    ).toBe("https://v4.core.example");
    expect(await firstDirectPath(["https://gone.core.example"], send, 50)).toBeUndefined();
    expect(await firstDirectPath([], send, 50)).toBeUndefined();
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

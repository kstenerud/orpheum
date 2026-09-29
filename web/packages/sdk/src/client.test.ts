// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { artUrl, createApi } from "./client.ts";

describe("the API client", () => {
  it("sends the session token in the Authorization header, and none when signed out", async () => {
    let token: string | undefined = "secret-token";
    const seen: (string | null)[] = [];
    const api = createApi({ origin: "http://orpheum.test", getToken: () => token });
    const answer = (request: Request): Promise<Response> => {
      seen.push(request.headers.get("Authorization"));
      expect(request.url).toBe("http://orpheum.test/api/v1/me");
      return Promise.resolve(
        new Response("{}", { status: 200, headers: { "Content-Type": "application/json" } }),
      );
    };
    await api.GET("/me", { fetch: answer });
    token = undefined;
    await api.GET("/me", { fetch: answer });
    expect(seen).toEqual(["Bearer secret-token", null]);
  });

  it("addresses an image by its hash and size, with nothing secret in the address", () => {
    expect(artUrl("ab12", "portrait-m")).toBe("/api/v1/bytes/art/ab12/portrait-m.webp");
    expect(artUrl("ab12", "landscape-l", "jpeg")).toBe("/api/v1/bytes/art/ab12/landscape-l.jpeg");
  });

  it("refuses, when it is compiled, a body key the API does not name (A12)", () => {
    const api = createApi({
      getToken: () => undefined,
      fetch: () => Promise.resolve(new Response(null, { status: 204 })),
    });
    const sendsARenamedField = (): unknown =>
      api.PUT("/files/{id}/preferences", {
        params: { path: { id: 1 } },
        // @ts-expect-error -- the body names subtitleChoice; subtitle was renamed (D227 §6)
        body: { subtitle: { isOff: true } },
      });
    expect(sendsARenamedField).toBeTypeOf("function");
  });
});

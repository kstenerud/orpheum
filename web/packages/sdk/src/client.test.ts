// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { artUrl, createApi } from "./client.ts";
import { ApiProblem, dataOrThrow } from "./problem.ts";

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

  it("sends a call's own Authorization in place of the session token: a membership's (D246 §3)", async () => {
    const seen: (string | null)[] = [];
    const api = createApi({ origin: "http://orpheum.test", getToken: () => "session-token" });
    await api.POST("/together/leave", {
      headers: { authorization: "Bearer membership" },
      fetch: (request: Request) => {
        seen.push(request.headers.get("Authorization"));
        return Promise.resolve(new Response(null, { status: 204 }));
      },
    });
    expect(seen).toEqual(["Bearer membership"]);
  });

  it("reads a proxy's answer for a server that is down as not answering, and keeps the server's own", async () => {
    const answered = async (response: Response): Promise<unknown> => {
      const api = createApi({
        origin: "http://orpheum.test",
        getToken: () => undefined,
        fetch: () => Promise.resolve(response),
      });
      try {
        dataOrThrow(await api.GET("/me"));
      } catch (cause: unknown) {
        return cause instanceof ApiProblem ? cause.code : cause;
      }
      return "no failure";
    };
    // Vite's dev proxy, and a reverse proxy, with nothing behind them (2026-10-06).
    for (const status of [502, 503, 504]) {
      expect(await answered(new Response("", { status }))).toBe("notAnswering");
      expect(await answered(new Response("<html>Bad Gateway</html>", { status }))).toBe(
        "notAnswering",
      );
    }
    expect(await answered(new Response("teapot", { status: 418 }))).toBe("unreadableAnswer");
    const migrating = JSON.stringify({ error: { code: "migrating", message: "starting" } });
    expect(
      await answered(
        new Response(migrating, { status: 503, headers: { "Content-Type": "application/json" } }),
      ),
    ).toBe("migrating");
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

// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { act, createElement } from "react";
import { describe, expect, it } from "vitest";

import { i18n } from "./i18n.ts";
import { ApiProblem, dataOrThrow, problemText } from "./problem.ts";
import { mount, refusal, servedBy } from "./test-dom.tsx";
import { useResource, type Resource } from "./use-resource.ts";

/** Where load leaves useResource, once it has settled. */
async function settledAfter(load: () => Promise<unknown>): Promise<Resource<unknown>> {
  const seen: Resource<unknown>[] = [];
  function Probe(): null {
    const [resource] = useResource("probe", load);
    seen.push(resource);
    return null;
  }
  const mounted = await mount(createElement(Probe));
  mounted.unmount();
  const last = seen.at(-1);
  if (last === undefined) {
    throw new Error("the probe never rendered");
  }
  return last;
}

/** Every state a probe polling load every few milliseconds shows while mounted for a moment. */
async function statesWhilePolling(load: () => Promise<number>): Promise<Resource<number>[]> {
  const seen: Resource<number>[] = [];
  function Probe(): null {
    const [resource] = useResource("probe", load, 5);
    seen.push(resource);
    return null;
  }
  const mounted = await mount(createElement(Probe));
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 60));
  });
  mounted.unmount();
  return seen;
}

describe("useResource", () => {
  it("asks again while polled, and keeps what it shows meanwhile", async () => {
    let asked = 0;
    const states = await statesWhilePolling(() => {
      asked += 1;
      return Promise.resolve(asked);
    });
    expect(asked).toBeGreaterThan(2);
    const firstLoaded = states.findIndex((state) => state.state === "loaded");
    expect(states.slice(firstLoaded).every((state) => state.state === "loaded")).toBe(true);
  });

  it("keeps the cause, and an error the server sent becomes an ApiProblem", async () => {
    const services = servedBy({ "GET /me": refusal(404, "not_found") });
    const fromServer = await settledAfter(async () => dataOrThrow(await services.api.GET("/me")));
    expect(fromServer.state === "failed" && fromServer.cause instanceof ApiProblem).toBe(true);
    const network = new TypeError("Failed to fetch");
    expect(await settledAfter(() => Promise.reject(network))).toEqual({
      state: "failed",
      cause: network,
    });
  });

  it("tells the person what went wrong in their words, with what to do", () => {
    const t = i18n.t.bind(i18n);
    const notFound = new ApiProblem({ code: "not_found", message: "no such title or series" });
    expect(problemText(t, notFound)).toBe(
      "That is no longer here: it may have been removed. Go back and try again.",
    );
    const ended = new ApiProblem({
      code: "auth_required",
      message: "sign in first",
      detail: { reason: "session_ended" },
    });
    expect(problemText(t, ended)).toContain("This device was signed out");
    expect(problemText(t, ended, { authRequired: "a code that does not work" })).toBe(
      "a code that does not work",
    );
    const unknown = problemText(t, new ApiProblem({ code: "brand_new", message: "odd" }));
    expect(unknown).toContain("Something went wrong");
    expect(unknown).not.toContain("odd");
    expect(problemText(t, new Error("the browser's own words"))).toBe(unknown);
    expect(problemText(t, new TypeError("Failed to fetch"))).toBe(
      "The server could not be reached. Check the connection and try again.",
    );
  });
});

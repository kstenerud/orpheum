// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { act, createElement } from "react";
import { describe, expect, it, vi } from "vitest";

import { i18n } from "./i18n.ts";
import type { Ask, AskOutcome, Poller } from "./polling.ts";
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

/**
 * A poller a test runs by hand: each ask a hook starts is kept, and the test runs it and reads what
 * it answered; polling.test.ts holds when backgroundChangesPoller runs them (D237).
 */
function handPoller(): Poller & { readonly asks: Ask[] } {
  const asks: Ask[] = [];
  return {
    asks,
    start: (ask) => {
      asks.push(ask);
      return () => undefined;
    },
  };
}

/** Runs the newest poll's ask once and returns whether it said what is shown changed. */
async function pollOnce(poller: { readonly asks: readonly Ask[] }): Promise<AskOutcome> {
  const ask = poller.asks.at(-1);
  if (ask === undefined) {
    throw new Error("nothing is polling");
  }
  let cameTo: AskOutcome | undefined;
  await act(async () => {
    ask((outcome) => {
      cameTo = outcome;
    });
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  if (cameTo === undefined) {
    throw new Error("the ask never answered");
  }
  return cameTo;
}

describe("useResource", () => {
  it("polls as its poller says, says whether each answer changed what it shows, and keeps it meanwhile", async () => {
    const poller = handPoller();
    const answers = [1, 1, 2, 2];
    const seen: Resource<number>[] = [];
    function Probe(): null {
      const [resource] = useResource("probe", () => Promise.resolve(answers.shift() ?? 2), poller);
      seen.push(resource);
      return null;
    }
    const mounted = await mount(createElement(Probe));
    expect(seen.at(-1)).toEqual({ state: "loaded", value: 1 });
    expect(await pollOnce(poller)).toBe("unchanged"); // 1 again: the wait doubles
    expect(await pollOnce(poller)).toBe("changed"); // 2: back to 2 s
    expect(seen.at(-1)).toEqual({ state: "loaded", value: 2 });
    expect(await pollOnce(poller)).toBe("unchanged");
    const firstLoaded = seen.findIndex((state) => state.state === "loaded");
    expect(seen.slice(firstLoaded).every((state) => state.state === "loaded")).toBe(true);
    mounted.unmount();
  });

  it("says a failed read failed, so its poller asks again at the first wait (2026-10-06)", async () => {
    const poller = handPoller();
    const written = vi.spyOn(console, "error").mockImplementation(() => undefined);
    let isServerUp = false;
    const seen: Resource<number>[] = [];
    function Probe(): null {
      const [resource] = useResource(
        "probe",
        () => (isServerUp ? Promise.resolve(1) : Promise.reject(new TypeError("Failed to fetch"))),
        poller,
      );
      seen.push(resource);
      return null;
    }
    const mounted = await mount(createElement(Probe));
    expect(await pollOnce(poller)).toBe("failed");
    expect(seen.at(-1)).toMatchObject({ state: "failed", willAskAgain: true });
    isServerUp = true;
    expect(await pollOnce(poller)).toBe("changed");
    mounted.unmount();
    written.mockRestore();
  });

  it("hands a poll's read its signal, and drops the read its poll gave up, keeping what it shows (the owner, 2026-10-24)", async () => {
    const poller = handPoller();
    const written = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const signals: (AbortSignal | undefined)[] = [];
    const seen: Resource<number>[] = [];
    function Probe(): null {
      const [resource] = useResource(
        "probe",
        (signal) => {
          signals.push(signal);
          if (signal === undefined) {
            return Promise.resolve(1);
          }
          return new Promise<number>((_, fail) => {
            signal.addEventListener("abort", () => {
              fail(new Error("given up"));
            });
          });
        },
        poller,
      );
      seen.push(resource);
      return null;
    }
    const mounted = await mount(createElement(Probe));
    const ask = poller.asks.at(-1);
    const aborter = new AbortController();
    const outcomes: AskOutcome[] = [];
    await act(async () => {
      ask?.((outcome) => outcomes.push(outcome), aborter.signal);
      aborter.abort();
      await new Promise((resolve) => setTimeout(resolve, 0));
    });
    expect(signals.at(-1)).toBe(aborter.signal);
    // The poll reported it and asks again; the read's own failure is neither shown nor written.
    expect(outcomes).toEqual([]);
    expect(seen.at(-1)).toEqual({ state: "loaded", value: 1 });
    expect(written).not.toHaveBeenCalled();
    mounted.unmount();
    written.mockRestore();
  });

  it("keeps the cause, and an error the server sent becomes an ApiProblem", async () => {
    const services = servedBy({ "GET /me": refusal(404, "notFound") });
    const fromServer = await settledAfter(async () => dataOrThrow(await services.api.GET("/me")));
    expect(fromServer.state === "failed" && fromServer.cause instanceof ApiProblem).toBe(true);
    const network = new TypeError("Failed to fetch");
    expect(await settledAfter(() => Promise.reject(network))).toEqual({
      state: "failed",
      cause: network,
    });
  });

  it("writes a failed load's cause to the console, as the person's words leave it out", async () => {
    const written = vi.spyOn(console, "error").mockImplementation(() => undefined);
    const cause = new SyntaxError("JSON.parse: unexpected end of data");
    await settledAfter(() => Promise.reject(cause));
    expect(written.mock.calls.map((call): unknown => call.at(-1))).toEqual([cause]);
    vi.restoreAllMocks();
  });

  it("tells the person what went wrong in their words, with what to do", () => {
    const t = i18n.t.bind(i18n);
    const notFound = new ApiProblem({ code: "notFound", message: "no such title or series" });
    expect(problemText(t, notFound)).toBe(
      "That is no longer here: it may have been removed. Go back and try again.",
    );
    const ended = new ApiProblem({
      code: "authRequired",
      message: "sign in first",
      detail: { reason: "sessionEnded" },
    });
    expect(problemText(t, ended)).toContain("This device was signed out");
    expect(problemText(t, ended, { authRequired: "a code that does not work" })).toBe(
      "a code that does not work",
    );
    const unknown = problemText(t, new ApiProblem({ code: "brand_new", message: "odd" }));
    expect(unknown).toContain("Something went wrong");
    // A proxy's answer for a server that is down is expected, with words of its own; any other
    // answer not in the server's shape is not, and gets the words that ask for a fix.
    expect(problemText(t, new ApiProblem({ code: "notAnswering", message: "502" }))).toBe(
      "The server is not answering: it may be restarting, or the machine it runs on may be off.",
    );
    expect(problemText(t, new ApiProblem({ code: "unreadableAnswer", message: "418" }))).toBe(
      unknown,
    );
    expect(unknown).not.toContain("odd");
    expect(problemText(t, new Error("the browser's own words"))).toBe(unknown);
    expect(problemText(t, new TypeError("Failed to fetch"))).toBe(
      "The server could not be reached. Check the connection and try again.",
    );
  });
});

// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { Jobs, type Job } from "./jobs.tsx";
import { parseRoute, routeHref } from "./router.ts";
import { click, mount, refusal, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

const preparing: Job = {
  id: 5,
  kind: "prepare",
  renditionId: 3,
  state: "running",
  progressFraction: 0.4,
  reason: "someone is waiting to watch it",
  attempts: 1,
  createdAt: 1,
};

const lookedUp: Job = {
  id: 4,
  kind: "fetch",
  itemId: 9,
  state: "failed",
  reason: "a new title",
  lastError: "TMDB did not answer",
  attempts: 3,
  createdAt: 1,
  finishedAt: 2,
};

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

describe("the job view", () => {
  it("is the admin's page at /jobs", () => {
    expect(parseRoute("/jobs", "")).toEqual({ screen: "jobs" });
    expect(routeHref({ screen: "jobs" })).toBe("/jobs");
  });

  it("shows each job's kind, state, progress, reason and error, and cancels one not yet ended", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /jobs": [
          [200, { jobs: [preparing, lookedUp] }],
          [200, { jobs: [{ ...preparing, state: "cancelled" }, lookedUp] }],
        ],
        "POST /jobs/5/cancel": [204, undefined],
      },
      heard,
    );
    mounted = await mount(withServices(<Jobs />, services));
    const shown = mounted.container.textContent;
    expect(shown).toContain("Preparing a copy #3");
    expect(shown).toContain("Running");
    expect(shown).toContain("Why: someone is waiting to watch it");
    expect(shown).toContain("Last error: TMDB did not answer");
    expect(mounted.container.querySelector("progress")?.getAttribute("value")).toBe("0.4");
    const cancels = mounted.container.querySelectorAll("button");
    expect(cancels).toHaveLength(1); // the failed job has ended: nothing to cancel
    await click(cancels[0]);
    expect(heard.map((each) => each.asked)).toEqual([
      "GET /jobs",
      "POST /jobs/5/cancel",
      "GET /jobs",
    ]);
    expect(mounted.container.textContent).toContain("Cancelled");
    expect(mounted.container.querySelectorAll("button")).toHaveLength(0);
  });

  it("says so when the job had already ended", async () => {
    const services = servedBy({
      "GET /jobs": [200, { jobs: [preparing] }],
      "POST /jobs/5/cancel": refusal(409, "conflict"),
    });
    mounted = await mount(withServices(<Jobs />, services));
    await click(mounted.container.querySelector("button"));
    expect(mounted.container.textContent).toContain("That job has already ended.");
  });
});

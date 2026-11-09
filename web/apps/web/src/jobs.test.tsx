// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { camelKey } from "./catalogue-key.ts";
import { i18n } from "./i18n.ts";
import { Jobs, targetId, type Job } from "./jobs.tsx";
import { parseRoute, routeHref } from "./router.ts";
import { choose, click, mount, refusal, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/**
 * Every kind of job the API names. The type makes it complete: a kind the server adds, or a
 * renamed one, fails to compile here until the screen can say it.
 */
const everyKind = [
  "fetchMetadata",
  "makeArtworkSizes",
  "extractSubtitle",
  "convertFetchedSubtitle",
  "readThrough",
  "makeChunks",
  "scan",
  "examine",
  "backup",
  "renewCertificate",
  "hashWholeFile",
  "makeSystemVideos",
] as const satisfies readonly Job["kind"][];

/** True only when everyKind names every kind: a kind left out makes this fail to compile. */
const isEveryKind: Exclude<Job["kind"], (typeof everyKind)[number]> extends never ? true : never =
  true;

const making: Job = {
  id: 5,
  kind: "makeChunks",
  chunkStoreId: 3,
  state: "running",
  progressFraction: 0.4,
  reason: "someone is waiting to watch it",
  attemptCount: 1,
  createdAt: 1,
};

const lookedUp: Job = {
  id: 4,
  kind: "fetchMetadata",
  itemId: 9,
  state: "failed",
  reason: "a new title",
  lastFailure: "TMDB did not answer",
  attemptCount: 3,
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
          [200, { jobs: [making, lookedUp] }],
          [200, { jobs: [{ ...making, state: "cancelled" }, lookedUp] }],
        ],
        "POST /jobs/5/cancel": [204, undefined],
      },
      heard,
    );
    mounted = await mount(withServices(<Jobs />, services));
    const shown = mounted.container.textContent;
    expect(shown).toContain("Making a film's chunks #3");
    expect(shown).toContain("Running");
    expect(shown).toContain("Why: someone is waiting to watch it");
    expect(shown).toContain("Last error: TMDB did not answer");
    expect(mounted.container.querySelector("progress")?.getAttribute("value")).toBe("0.4");
    const cancels = mounted.container.querySelectorAll("button");
    expect(cancels).toHaveLength(1); // the failed job has ended: nothing to cancel
    await click(cancels[0]);
    expect(heard.map((each) => each.methodAndPath)).toEqual([
      "GET /jobs",
      "POST /jobs/5/cancel",
      "GET /jobs",
    ]);
    expect(mounted.container.textContent).toContain("Cancelled");
    expect(mounted.container.querySelectorAll("button")).toHaveLength(0);
  });

  it("says what a waiting job waits for, why, since when, and when it is looked at next (D231 §12)", async () => {
    const waitingOnTMDB: Job = {
      id: 6,
      kind: "fetchMetadata",
      itemId: 9,
      state: "queued",
      reason: "a new title",
      attemptCount: 0,
      createdAt: 1,
      destination: "tmdb-api",
      waitingFor: {
        destination: "tmdb-api",
        name: "TMDB",
        reason: "unreachable",
        downAt: 1_790_000_000_000,
        nextCheckAt: 1_790_000_480_000,
        isChecking: false,
      },
    };
    const waitingOnNode: Job = {
      ...making,
      id: 7,
      state: "queued",
      destination: "node:1",
      waitingFor: {
        destination: "node:1",
        name: "node local",
        reason: "not connected",
        downAt: 1_790_000_000_000,
        isChecking: false,
      },
    };
    const services = servedBy({ "GET /jobs": [200, { jobs: [waitingOnTMDB, waitingOnNode] }] });
    mounted = await mount(withServices(<Jobs />, services));
    const waiting = [...mounted.container.querySelectorAll(".jobs__waiting")].map(
      (line) => line.textContent,
    );
    expect(waiting).toHaveLength(2);
    expect(waiting[0]).toMatch(/^Waiting for TMDB: unreachable since .+, next check .+$/);
    expect(waiting[1]).toMatch(/^Waiting for node local: not connected since .+$/);
    expect(waiting[1]).not.toContain("next check"); // its hello brings it back, not a check
  });

  it("says where a node holds the work: waiting for a slot, or paused for a film (D240 §6)", async () => {
    const waitingForSlot: Job = { ...making, id: 8, onNode: "waiting" };
    const paused: Job = {
      ...making,
      id: 9,
      kind: "readThrough",
      fileId: 2,
      onNode: "paused",
    };
    const running: Job = { ...making, id: 10, onNode: "running" };
    const services = servedBy({ "GET /jobs": [200, { jobs: [waitingForSlot, paused, running] }] });
    mounted = await mount(withServices(<Jobs />, services));
    const waiting = [...mounted.container.querySelectorAll(".jobs__waiting")].map(
      (line) => line.textContent,
    );
    expect(waiting).toEqual([
      "Waiting for a free slot on the media machine",
      "Paused while someone watches",
    ]);
  });

  // ui-audit-2026-11-04.md §3, Jobs; the filter runs on the newest jobs sent (the owner, Option A).
  it("lists running then waiting jobs first, folds the ended ones by day, and filters by state and kind", async () => {
    const at = (day: number, hour: number): number => new Date(2026, 10, day, hour).getTime();
    const waiting: Job = { ...making, id: 1, state: "queued" };
    const doneToday: Job = { ...lookedUp, id: 2, state: "done", finishedAt: at(4, 10) };
    const failedYesterday: Job = { ...lookedUp, id: 3, finishedAt: at(3, 22) };
    const scannedYesterday: Job = {
      id: 6,
      kind: "scan",
      scanId: 2,
      state: "done",
      reason: "a folder changed",
      attemptCount: 1,
      createdAt: 1,
      finishedAt: at(3, 9),
    };
    const jobs = [doneToday, waiting, failedYesterday, making, scannedYesterday];
    mounted = await mount(withServices(<Jobs />, servedBy({ "GET /jobs": [200, { jobs }] })));
    const page = mounted.container;
    const named = (within: Element | null | undefined): string[] =>
      [...(within?.querySelectorAll("strong") ?? [])].map((name) => name.textContent);
    expect(named(page.querySelector("h2 + ul.jobs"))).toEqual([
      "Making a film's chunks #3",
      "Making a film's chunks #3",
    ]);
    expect(page.querySelector("h2 + ul.jobs")?.textContent).toMatch(/Running.*Waiting/u);
    const days = [...page.querySelectorAll("details.jobs__day")];
    expect(days.map((day) => day.hasAttribute("open"))).toEqual([false, false]);
    expect(
      days.map((day) => /\((\d+)\)$/u.exec(day.querySelector("summary")?.textContent ?? "")?.[1]),
    ).toEqual(["1", "2"]);
    expect(named(days[0])).toEqual(["Looking up #9"]);
    expect(named(days[1])).toEqual(["Looking up #9", "Listing a folder #2"]);
    const select = (label: string): Element | null | undefined =>
      [...page.querySelectorAll("label")]
        .find((each) => each.textContent.startsWith(label))
        ?.querySelector("select");
    await choose(select("State"), "failed");
    expect(named(page)).toEqual(["Looking up #9"]);
    expect(page.querySelector("h2 + ul.jobs")).toBeNull();
    await choose(select("State"), "");
    await choose(select("Kind"), "scan");
    expect(named(page)).toEqual(["Listing a folder #2"]);
    await choose(select("State"), "running");
    expect(page.textContent).toContain("No job is of that state and kind.");
  });

  it("says so when the job had already ended", async () => {
    const services = servedBy({
      "GET /jobs": [200, { jobs: [making] }],
      "POST /jobs/5/cancel": refusal(409, "conflict"),
    });
    mounted = await mount(withServices(<Jobs />, services));
    await click(mounted.container.querySelector("button"));
    expect(mounted.container.textContent).toContain("That job has already ended.");
  });
});

describe("each kind of job", () => {
  it("names what it works on: a scan's listing its scan, its reading the batch, a run its store", () => {
    const started = { state: "running", reason: "a scan", attemptCount: 1, createdAt: 1 } as const;
    const scanning: Job = { ...started, id: 6, kind: "scan", scanId: 4 };
    const examining: Job = { ...started, id: 7, kind: "examine", scanBatchId: 9 };
    const making: Job = { ...started, id: 8, kind: "makeChunks", chunkStoreId: 3 };
    expect([targetId(scanning), targetId(examining), targetId(making)]).toEqual([4, 9, 3]);
  });

  it("names a backup by its kind alone: it works on the whole server (D243 §11)", async () => {
    const backingUp: Job = {
      id: 8,
      kind: "backup",
      state: "queued",
      reason: "an admin asked for a backup now",
      attemptCount: 0,
      createdAt: 1,
    };
    mounted = await mount(
      withServices(<Jobs />, servedBy({ "GET /jobs": [200, { jobs: [backingUp] }] })),
    );
    expect(mounted.container.querySelector("strong")?.textContent).toBe("Backing up");
    expect(mounted.container.querySelector("button")?.getAttribute("aria-label")).toBe(
      "Cancel Backing up",
    );
  });

  it("has words on the jobs screen", () => {
    expect(isEveryKind).toBe(true);
    for (const kind of everyKind) {
      const key = `jobs.kind.${camelKey(kind)}`;
      expect(i18n.exists(key), key).toBe(true);
    }
  });
});

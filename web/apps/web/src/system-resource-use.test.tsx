// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { SystemResourceUse } from "./system-resource-use.tsx";
import { mount, servedBy, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const core = { isNode: false, nodeName: "" };
const nas = { isNode: true, nodeName: "nas" };

it("shows what the server and each node used, and each hour beside what it was doing", async () => {
  const services = servedBy({
    "GET /system/resource-use": [
      200,
      {
        summaries: [
          {
            ...core,
            since: 1_790_000_000_000,
            cpuPercent: 2.5,
            busiestCpuPercent: 12,
            heldBytes: 150_000_000,
            cacheBytes: 20_000_000,
            peakBytes: 0,
          },
          {
            ...nas,
            since: 1_790_000_000_000,
            heldBytes: 900_000_000,
            cacheBytes: 3_000_000_000,
            peakBytes: 1_200_000_000,
          },
        ],
        hours: [
          {
            ...core,
            start: 1_790_000_000_000,
            cpuPercent: 2.5,
            busiestCpuPercent: 12,
            heldBytes: 150_000_000,
            cacheBytes: 20_000_000,
            peakBytes: 0,
            plays: 2,
            running: {},
          },
          {
            ...nas,
            start: 1_790_000_000_000,
            cpuPercent: 310,
            busiestCpuPercent: 395.5,
            heldBytes: 900_000_000,
            cacheBytes: 3_000_000_000,
            peakBytes: 1_200_000_000,
            plays: 0,
            // The wire names a kind as the server does, in snake case.
            running: Object.fromEntries([["make_chunks", 2]]),
          },
        ],
      },
    ],
  });
  mounted = await mount(withServices(<SystemResourceUse />, services));
  const text = mounted.container.textContent;
  expect(text).toContain("The server");
  expect(text).toContain("2.5% (busiest minute 12%)");
  expect(text).toContain("150 MB held, 20 MB cached");
  expect(text).toContain("2 playing");
  expect(text).toContain("Media node nas");
  expect(text).toContain("CPU not yet known");
  expect(text).toContain("900 MB held, 3 GB cached, 1.2 GB at most");
  expect(text).toContain("310% (busiest minute 395.5%)");
  expect(text).toContain("Making a film's chunks × 2");
  // Each process's hours are folded, closed, under their count (ui-audit-2026-11-04.md §3).
  const folds = [...mounted.container.querySelectorAll("details")];
  expect(folds.map((each) => each.open)).toEqual([false, false]);
  expect(folds.map((each) => each.querySelector("summary")?.textContent)).toEqual([
    "Its hours of the last day (1)",
    "Its hours of the last day (1)",
  ]);
  expect(folds.every((each) => each.querySelector("table") !== null)).toBe(true);
});

it("says nothing is measured before the first minute", async () => {
  const services = servedBy({ "GET /system/resource-use": [200, { summaries: [], hours: [] }] });
  mounted = await mount(withServices(<SystemResourceUse />, services));
  expect(mounted.container.textContent).toContain("Nothing is measured yet");
});

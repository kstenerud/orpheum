// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { NodeDevices, NodeWorkView, QueueHealth } from "./system-work.tsx";
import {
  buttonWithText,
  click,
  enter,
  mount,
  servedBy,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

it("shows how many of each kind of job wait, since when, and what failed in the last day (D233 §8)", async () => {
  mounted = await mount(
    withServices(
      <QueueHealth
        queue={[
          {
            kind: "read_through",
            queuedCount: 3,
            oldestQueuedAt: 1_790_000_000_000,
            failedLastDayCount: 0,
          },
          { kind: "prepare", queuedCount: 0, failedLastDayCount: 2 },
        ]}
      />,
      servedBy({}),
    ),
  );
  const text = mounted.container.textContent;
  expect(text).toContain("Reading a video through 3 waiting since");
  expect(text).toContain("2 failed in the last day");
  expect(text).not.toContain("0 failed");
});

it("shows what a machine is doing, and the faults the server saw in what it holds (D234 §4)", async () => {
  mounted = await mount(
    withServices(
      <NodeWorkView
        work={{
          runningCount: 2,
          slotCount: 6,
          waitingCount: 1,
          lastMovedAt: 1_790_000_000_000,
          brokenInvariantCount: 1,
        }}
      />,
      servedBy({}),
    ),
  );
  expect(mounted.container.textContent).toContain(
    "2 of 6 jobs it runs at once are running; 1 waiting.",
  );
  expect(mounted.container.querySelector('[role="alert"]')?.textContent).toContain(
    "1 times it held work",
  );
});

it("sets how many may read a disk at once, and gives it back to its kind's (D240 §5)", async () => {
  const heard: Heard[] = [];
  const services = servedBy({ "PUT /nodes/device-reader-limit": [204, null] }, heard);
  mounted = await mount(
    withServices(
      <NodeDevices
        nodeName="local"
        devices={[
          {
            key: "zfs:tank",
            kind: "spinning",
            filesystem: "zfs",
            disks: ["sda", "sdb"],
            mountPaths: ["/tank"],
            readerLimit: 1,
            isReaderLimitSet: false,
          },
        ]}
      />,
      services,
    ),
  );
  const container = mounted.container;
  expect(container.textContent).toContain("1 may read it at once, as for its kind.");
  await enter(container.querySelector("input"), "3");
  await click(buttonWithText(container, "Set"));
  await click(buttonWithText(container, "Use its kind's"));
  expect(heard.map((each) => each.body)).toEqual([
    { nodeName: "local", deviceKey: "zfs:tank", readerLimit: 3 },
    { nodeName: "local", deviceKey: "zfs:tank" },
  ]);
  expect(container.textContent).toContain("as for its kind.");
});

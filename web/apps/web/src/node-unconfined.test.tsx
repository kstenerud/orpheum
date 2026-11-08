// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { components } from "@orpheum/sdk";
import { afterEach, expect, it } from "vitest";

import { NodeUnconfinedLeave } from "./node-unconfined.tsx";
import { buttonWithText, click, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const locked: components["schemas"]["SystemNodeBody"] = {
  name: "nas",
  connection: "locked",
  isConnected: false,
  isServersBuild: false,
  uid: 0,
  gid: 0,
  confinement: [
    { mechanism: "landlock", isInForce: false, causeCode: "disabled_at_boot" },
    { mechanism: "seccomp", isInForce: true },
  ],
  disks: [],
  devices: [],
  stuck: [],
};

it("shows a locked machine's risk, and an admin's explicit leave to run it unconfined, confirmed first (D450 §1)", async () => {
  const heard: Heard[] = [];
  let changes = 0;
  mounted = await mount(
    withServices(
      <NodeUnconfinedLeave
        node={locked}
        onChanged={() => {
          changes += 1;
        }}
      />,
      servedBy({ "PUT /nodes/nas/unconfined": [204, null] }, heard),
    ),
  );
  const container = mounted.container;
  expect(container.textContent).toContain("This machine is locked");
  expect(container.textContent).toContain("A film made to attack FFmpeg could");
  await click(buttonWithText(container, "Run without confinement…"));
  expect(heard).toEqual([]);
  await click(buttonWithText(container, "Accept the risk and run it"));
  expect(heard.map((each) => each.methodAndPath)).toEqual(["PUT /nodes/nas/unconfined"]);
  expect(changes).toBe(1);
});

it("says who let a machine run unconfined and when, and withdraws it (D450 §1)", async () => {
  const heard: Heard[] = [];
  mounted = await mount(
    withServices(
      <NodeUnconfinedLeave
        node={{
          ...locked,
          connection: "connected",
          isConnected: true,
          unconfinedLeave: { acceptedBy: "karl", acceptedAt: 1_790_000_000_000 },
        }}
      />,
      servedBy({ "DELETE /nodes/nas/unconfined": [204, null] }, heard),
    ),
  );
  const container = mounted.container;
  expect(container.textContent).toContain("Runs without confinement: karl accepted the risk on");
  await click(buttonWithText(container, "Withdraw, and lock it again"));
  expect(heard.map((each) => each.methodAndPath)).toEqual(["DELETE /nodes/nas/unconfined"]);
});

it("shows nothing for a machine that is neither locked nor let run unconfined", async () => {
  mounted = await mount(
    withServices(
      <NodeUnconfinedLeave node={{ ...locked, connection: "connected", isConnected: true }} />,
      servedBy({}),
    ),
  );
  expect(mounted.container.textContent).toBe("");
});

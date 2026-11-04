// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { SystemReachability } from "./system-reachability.tsx";
import { buttonWithText, click, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

it("checks the public name and the relay when asked, and names the hop that failed with what to look at (D243 §7)", async () => {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "POST /system/reachability": [
        200,
        {
          checkedAt: 1_790_000_000_000,
          name: "media.orpheum-app.com",
          addresses: ["203.0.113.7"],
          https: { outcome: "reached" },
          relayUdp: { outcome: "notAnswering", detail: "i/o timeout" },
          relayTcp: { outcome: "reached" },
        },
      ],
    },
    heard,
  );
  mounted = await mount(withServices(<SystemReachability />, services));
  const container = mounted.container;
  expect(heard).toEqual([]);
  await click(buttonWithText(container, "Check now"));
  expect(heard.map((each) => each.methodAndPath)).toEqual(["POST /system/reachability"]);
  const text = container.textContent;
  expect(text).toContain("media.orpheum-app.com points at 203.0.113.7.");
  expect(text).toContain("HTTPS to media.orpheum-app.com: reached this server.");
  expect(text).toContain(
    "The voice relay over UDP: nothing answered. Check that the VPS is up, that it forwards the port, and that the tunnel is connected. The cause: i/o timeout",
  );
  expect(text).toContain("The voice relay over TCP: reached this server.");
});

it("puts Check now in the section's heading row (ui-audit-2026-11-04.md §3)", async () => {
  mounted = await mount(withServices(<SystemReachability />, servedBy({})));
  const container = mounted.container;
  const row = container.querySelector("#system-reachability")?.parentElement;
  expect(row?.contains(buttonWithText(container, "Check now") ?? null)).toBe(true);
  expect(row?.querySelector("p")).toBeNull();
});

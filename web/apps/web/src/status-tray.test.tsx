// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, expect, it } from "vitest";

import { NeedsAttention } from "./needs-attention.tsx";
import { StatusTray } from "./status-tray.tsx";
import { click, mount, servedBy, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const nothingWrong = {
  isShown: false,
  rootCount: 0,
  diskCount: 0,
  waitingRootCount: 0,
  unreadCount: 0,
};
const somethingWrong = { ...nothingWrong, isShown: true, waitingRootCount: 1, unreadCount: 2 };

it("shows nothing to someone who neither hosts nor runs the server", async () => {
  mounted = await mount(
    withServices(<StatusTray isHostingTogether={false} isAdmin={false} />, servedBy({})),
  );
  expect(mounted.container.querySelector(".status-tray")).toBeNull();
});

it("shows the host Watch Together is on, and opens its controls over the screen without leaving it (D280)", async () => {
  mounted = await mount(
    withServices(<StatusTray isHostingTogether isAdmin={false} />, servedBy({})),
  );
  const sign = mounted.container.querySelector<HTMLButtonElement>(
    '[aria-label="Watch Together is on: its controls"]',
  );
  expect(sign).not.toBeNull();
  expect(mounted.container.querySelector("dialog")).toBeNull();
  await click(sign);
  expect(mounted.container.querySelector("dialog")?.textContent).toContain("Watch Together");
  expect(sign?.getAttribute("aria-expanded")).toBe("true");
  await act(async () => {
    document.dispatchEvent(new KeyboardEvent("keydown", { key: "Escape" }));
    await Promise.resolve();
  });
  expect(mounted.container.querySelector("dialog")).toBeNull();
});

it("shows an admin the yellow sign when something needs them, leading to the page of it, and not otherwise", async () => {
  mounted = await mount(
    withServices(
      <StatusTray isHostingTogether={false} isAdmin />,
      servedBy({ "GET /offline-reminder": [200, somethingWrong] }),
    ),
  );
  const link = mounted.container.querySelector('[aria-label="Something needs your attention"]');
  expect(link?.getAttribute("href")).toBe("/needs-attention");
  expect(link?.querySelector(".warning-icon")).not.toBeNull();
  mounted.unmount();
  mounted = await mount(
    withServices(
      <StatusTray isHostingTogether={false} isAdmin />,
      servedBy({ "GET /offline-reminder": [200, nothingWrong] }),
    ),
  );
  expect(
    mounted.container.querySelector('[aria-label="Something needs your attention"]'),
  ).toBeNull();
});

it("the page of what needs the admin counts each kind, a placeholder until it is designed", async () => {
  mounted = await mount(
    withServices(<NeedsAttention />, servedBy({ "GET /offline-reminder": [200, somethingWrong] })),
  );
  const text = mounted.container.textContent;
  expect(text).toContain("1 library folders' scans wait for you");
  expect(text).toContain("2 files or folders could not be read.");
  expect(text).not.toContain("offline");
});

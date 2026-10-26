// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, expect, it, vi } from "vitest";

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
  rootCount: 0,
  diskCount: 0,
  waitingRootCount: 0,
  unreadCount: 0,
  matchCount: 0,
};
const somethingWrong = { ...nothingWrong, waitingRootCount: 1, unreadCount: 2 };

it("shows nothing to someone who neither hosts nor runs the server", async () => {
  mounted = await mount(
    withServices(<StatusTray isHostingTogether={false} isAdmin={false} />, servedBy({})),
  );
  expect(mounted.container.querySelector(".status-tray")).toBeNull();
});

it("shows the host Watch Together is on, and opens its controls over the screen without leaving it (D280)", async () => {
  const showModal = vi.spyOn(HTMLDialogElement.prototype, "showModal");
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
  // A modal drop-down: the page takes no press while it is open (the owner, 2026-10-11).
  expect(showModal).toHaveBeenCalled();
  // Escape, which the browser tells a modal as cancel, shuts it.
  await act(async () => {
    mounted?.container
      .querySelector("dialog")
      ?.dispatchEvent(new Event("cancel", { cancelable: true }));
    await Promise.resolve();
  });
  expect(mounted.container.querySelector("dialog")).toBeNull();
  // A press outside it, on its backdrop, shuts it; one inside does not.
  await click(sign);
  const panel = mounted.container.querySelector("dialog");
  panel!.getBoundingClientRect = () => new DOMRect(100, 50, 300, 400);
  await act(async () => {
    panel?.dispatchEvent(new MouseEvent("click", { bubbles: true, clientX: 200, clientY: 100 }));
    await Promise.resolve();
  });
  expect(mounted.container.querySelector("dialog")).not.toBeNull();
  await act(async () => {
    panel?.dispatchEvent(new MouseEvent("click", { bubbles: true, clientX: 20, clientY: 600 }));
    await Promise.resolve();
  });
  expect(mounted.container.querySelector("dialog")).toBeNull();
  showModal.mockRestore();
});

it("shows the session's controls the moment they open, having asked for them as the sign appeared (the owner, 2026-10-12)", async () => {
  window.sessionStorage.setItem(
    "orpheum.together.host",
    JSON.stringify({ sessionId: 7, credential: "host-credential" }),
  );
  const members = [{ id: 1, name: "Karl", isHost: true, devices: [] }];
  const session = {
    sessionId: 7,
    nextRights: { canPlayPause: true, canSeek: true },
    links: [],
    members,
  };
  mounted = await mount(
    withServices(
      <StatusTray isHostingTogether isAdmin={false} />,
      servedBy({ "GET /together/7": [200, session] }),
    ),
  );
  const sign = mounted.container.querySelector<HTMLButtonElement>(
    '[aria-label="Watch Together is on: its controls"]',
  );
  // The controls' code, asked for as the sign appeared, is here by the time a person presses it.
  await act(async () => {
    await vi.dynamicImportSettled();
  });
  // Opened, before the server answers again: what it asked for as the sign appeared shows at once.
  act(() => {
    sign?.click();
  });
  expect(mounted.container.querySelector("dialog")?.textContent).toContain("Your session is on.");
  window.sessionStorage.clear();
});

it("shows an admin the yellow sign when something needs them, leading to the page of it, and not otherwise", async () => {
  mounted = await mount(
    withServices(
      <StatusTray isHostingTogether={false} isAdmin />,
      servedBy({ "GET /needs-attention": [200, somethingWrong] }),
    ),
  );
  const link = mounted.container.querySelector('[aria-label="Something needs your attention"]');
  expect(link?.getAttribute("href")).toBe("/needs-attention");
  expect(link?.querySelector(".warning-icon")).not.toBeNull();
  mounted.unmount();
  mounted = await mount(
    withServices(
      <StatusTray isHostingTogether={false} isAdmin />,
      servedBy({ "GET /needs-attention": [200, nothingWrong] }),
    ),
  );
  expect(
    mounted.container.querySelector('[aria-label="Something needs your attention"]'),
  ).toBeNull();
});

it("the page of what needs the admin counts each kind, a placeholder until it is designed", async () => {
  mounted = await mount(
    withServices(<NeedsAttention />, servedBy({ "GET /needs-attention": [200, somethingWrong] })),
  );
  const text = mounted.container.textContent;
  expect(text).toContain("1 library folders' scans wait for you");
  expect(text).toContain("2 files or folders could not be read.");
  expect(text).not.toContain("offline");
});

it("tells an admin of a series whose episodes were likely read wrongly, leading to it (D306 §6)", async () => {
  const readWrongly = {
    ...nothingWrong,
    seriesReadWrongly: [{ seriesId: 606, name: "Quillwick", unplacedCount: 24 }],
  };
  mounted = await mount(
    withServices(
      <StatusTray isHostingTogether={false} isAdmin />,
      servedBy({ "GET /needs-attention": [200, readWrongly] }),
    ),
  );
  expect(
    mounted.container.querySelector('[aria-label="Something needs your attention"]'),
  ).not.toBeNull();
  mounted.unmount();
  mounted = await mount(
    withServices(<NeedsAttention />, servedBy({ "GET /needs-attention": [200, readWrongly] })),
  );
  expect(mounted.container.querySelector('a[href="/series/606"]')?.textContent).toBe("Quillwick");
  expect(mounted.container.textContent).toContain(": 24 of its episodes have no number");
});

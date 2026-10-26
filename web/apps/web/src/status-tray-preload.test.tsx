// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

/**
 * Kept apart from status-tray.test.tsx (the owner, 2026-10-26): WatchTogetherControls is loaded
 * once per test file, by whichever test first opens the tray, and its loaded state then stays
 * resolved for every test after that in the same file — so a test placed after one that already
 * opened the tray cannot tell "loaded as the sign appears" apart from "loaded only once opened"
 * (e59f3dba). Alone in its own file, this is the first and only time the tray ever opens, so the
 * difference shows.
 */

import "./i18n.ts";

import { act } from "react";
import { afterEach, expect, it, vi } from "vitest";

import { StatusTray } from "./status-tray.tsx";
import { mount, servedBy, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  window.sessionStorage.clear();
});

it("asks for Watch Together's controls' code as the sign appears, so the tray shows them the moment it opens (the owner, 2026-10-12)", async () => {
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
  // The sign is up, the tray still closed: nothing has been clicked yet, so only the sign's own
  // mount could have asked for the controls' code by now.
  await act(async () => {
    await vi.dynamicImportSettled();
  });
  // Opened only now, before the server is asked again: what was asked for as the sign appeared
  // shows at once, with no "Loading…" in between.
  act(() => {
    sign?.click();
  });
  expect(mounted.container.querySelector("dialog")?.textContent).toContain("Your session is on.");
});

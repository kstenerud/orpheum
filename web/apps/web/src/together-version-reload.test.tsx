// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import type { StringStorage } from "./browser-storage.ts";
import { createNavigation } from "./navigation.ts";
import { forgetProtocolVersionReload, reloadsForProtocolVersion } from "./newer-client.ts";
import { currentNotice, dismissNotice } from "./notice.ts";
import { buttonWithText, click, mount, servedBy, type Mounted } from "./test-dom.tsx";
import { fakePlace, withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";
import { TogetherViewer } from "./together-viewer.tsx";

const welcome = {
  type: "welcome",
  seq: 1,
  sessionId: 1,
  memberId: 2,
  deviceId: 8,
  group: { state: "idle", positionMs: 0, rate: 1, fileId: 0 },
  roster: [
    { id: 1, name: "Karl", isHost: true, devices: [{ id: 3, role: "video" }] },
    { id: 2, name: "Gran", devices: [{ id: 8, role: "video" }] },
  ],
};

const membership = JSON.stringify({ sessionId: 1, credential: "viewer-credential" });

const olderPage = "This page is older than the server; reload it.";

beforeEach(() => {
  window.sessionStorage.clear();
  window.localStorage.clear();
  const left = currentNotice();
  if (left !== undefined) {
    dismissNotice(left);
  }
});

afterEach(() => {
  vi.restoreAllMocks();
});

/** A storage of the page's own, in memory, or one that keeps nothing, as a private window may. */
function memoryStorage(canKeep = true): StringStorage {
  const kept = new Map<string, string>();
  return {
    getItem: (key) => kept.get(key) ?? null,
    setItem: (key, value) => {
      if (canKeep) {
        kept.set(key, value);
      }
    },
    removeItem: (key) => {
      kept.delete(key);
    },
  };
}

/**
 * A guest by link alone, back in their session on a page loaded (again), whose socket the server
 * answers with said: the page, and how many times it reloaded.
 */
async function guestAnswered(
  said: Readonly<Record<string, unknown>>,
): Promise<{ readonly mounted: Mounted; readonly reloads: () => number }> {
  const reload = vi.spyOn(window.location, "reload").mockImplementation(() => undefined);
  window.localStorage.setItem("orpheum.together.viewer", membership);
  const server = fakeTogetherServer();
  const services = {
    ...servedBy({ "GET /me/together": [200, { sessions: [] }] }, [], undefined),
    navigation: createNavigation(fakePlace("/together")),
    openTogetherSocket: server.openSocket,
  };
  const mounted = await mount(withServices(<TogetherViewer />, services));
  await click(buttonWithText(mounted.container, "Tap to continue"));
  await server.accept();
  await server.say(said);
  return { mounted, reloads: () => reload.mock.calls.length };
}

// A tab left open across an update speaks the old protocol, and the core refuses it (cut-a §5.1):
// the page reloads, once, for the client the core serves; refused again, it says so and stops.
describe("a page the core refuses for its Watch Together version", () => {
  it("reloads once, keeping the membership; refused again after it, says the page is older and reloads no more", async () => {
    const refused = { type: "rejected", code: "versionMismatch" };
    const first = await guestAnswered(refused);
    expect(first.reloads()).toBe(1);
    expect(window.localStorage.getItem("orpheum.together.viewer")).toBe(membership);
    first.mounted.unmount();
    vi.restoreAllMocks();
    const again = await guestAnswered(refused); // the page after the reload, in the same tab
    expect(again.reloads()).toBe(0);
    expect(again.mounted.container.textContent).toContain(olderPage);
    expect(window.localStorage.getItem("orpheum.together.viewer")).toBe(membership);
    again.mounted.unmount();
  });

  it("forgets it reloaded once a connection is welcomed, so a later update reloads again", async () => {
    expect(reloadsForProtocolVersion(window.sessionStorage, () => undefined)).toBe(true);
    const welcomed = await guestAnswered(welcome);
    expect(window.sessionStorage.getItem("orpheum.together.reloadedForVersion")).toBeNull();
    welcomed.mounted.unmount();
  });

  it("reloads once in a tab, never where the tab keeps nothing, and again once forgotten", () => {
    const reload = vi.fn<() => void>();
    const tab = memoryStorage();
    expect(reloadsForProtocolVersion(tab, reload)).toBe(true);
    expect(reloadsForProtocolVersion(tab, reload)).toBe(false);
    expect(reload).toHaveBeenCalledTimes(1);
    forgetProtocolVersionReload(tab);
    expect(reloadsForProtocolVersion(tab, reload)).toBe(true);
    expect(reload).toHaveBeenCalledTimes(2);
    expect(reloadsForProtocolVersion(memoryStorage(false), reload)).toBe(false); // a private window
    expect(reload).toHaveBeenCalledTimes(2);
  });
});

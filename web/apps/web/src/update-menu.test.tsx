// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AppShell } from "./app-shell.tsx";
import { click, mount, servedBy, settleUntil, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { updateReadEveryMs, type Update } from "./update.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  vi.useRealTimers();
});

const latest: Update = { version: "v0.3.3", checkedAt: 1_793_000_000_000 };
const newer: Update = {
  version: "v0.3.3",
  latest: "v0.3.4",
  command: "docker run --rm ghcr.io/kstenerud/orpheum:v0.3.4 install-script | sh",
  checkedAt: 1_793_000_360_000,
};

/** The rest of an admin's shell's own asking: the libraries menu and the tray's sign. */
const shellAnswers = {
  "GET /libraries": [200, { libraries: [] }],
  "GET /needs-attention": [
    200,
    {
      rootCount: 0,
      diskCount: 0,
      waitingRootCount: 0,
      unreadCount: 0,
      matchCount: 0,
      seriesReadWronglyCount: 0,
      filmsWithoutSoundCount: 0,
    },
  ],
} as const;

async function mountShell(updates: readonly Update[], heard: Heard[]): Promise<Mounted> {
  return await mount(
    withServices(
      <AppShell
        isAdmin={true}
        canHostWatchTogether={false}
        currentScreen="films"
        onSignOut={() => undefined}
        signedInAs={undefined}
      >
        <p>screen</p>
      </AppShell>,
      servedBy(
        { ...shellAnswers, "GET /system/update": updates.map((each) => [200, each] as const) },
        heard,
      ),
    ),
  );
}

/** The menu's Update entry, as the open menu shows it. */
function updateEntry(shown: Mounted): string | undefined {
  return shown.container.querySelector('.list-box a[href="/update"]')?.textContent;
}

/** Opens the shell's menu, which draws its places only while it is open (drop-down.tsx). */
async function openMenu(shown: Mounted): Promise<void> {
  await click(shown.container.querySelector(".app-shell__menu-button"));
}

/** Runs the shell's timers on by ms, as its ten-minute read waits. */
async function waitOn(ms: number): Promise<void> {
  await act(async () => {
    vi.advanceTimersByTime(ms);
    await Promise.resolve();
  });
}

const updateReads = (heard: readonly Heard[]): number =>
  heard.filter((each) => each.methodAndPath === "GET /system/update").length;

// D462 §8, §10.9: Update is always in the admin's menu, "! Update" while an update is known, and the
// shell reads the update state again when the page is shown again and every ten minutes, so the mark
// comes without a page change.
describe("the menu's Update", () => {
  it("is always there for an admin, marked once a read finds an update, read again when the page is shown", async () => {
    const heard: Heard[] = [];
    mounted = await mountShell([latest, newer], heard);
    const shown = mounted;
    await openMenu(shown);
    await settleUntil(() => updateEntry(shown) !== undefined && updateReads(heard) === 1);
    expect(updateEntry(shown)).toBe("Update");
    await act(async () => {
      document.dispatchEvent(new Event("visibilitychange"));
      await Promise.resolve();
    });
    await settleUntil(() => updateEntry(shown) === "! Update");
    expect(updateReads(heard)).toBe(2);
  });

  it("reads again every ten minutes, keeping the mark through a read that fails", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const heard: Heard[] = [];
    mounted = await mount(
      withServices(
        <AppShell
          isAdmin={true}
          canHostWatchTogether={false}
          currentScreen="films"
          onSignOut={() => undefined}
          signedInAs={undefined}
        >
          <p>screen</p>
        </AppShell>,
        servedBy(
          {
            ...shellAnswers,
            "GET /system/update": [
              [200, newer],
              [500, { error: { code: "internal", message: "the server failed" } }],
              [200, latest],
            ],
          },
          heard,
        ),
      ),
    );
    const shown = mounted;
    await openMenu(shown);
    await settleUntil(() => updateEntry(shown) === "! Update");
    await waitOn(updateReadEveryMs - 1);
    expect(updateReads(heard)).toBe(1);
    await waitOn(1);
    await settleUntil(() => updateReads(heard) === 2);
    expect(updateEntry(shown)).toBe("! Update");
    await waitOn(updateReadEveryMs);
    await settleUntil(() => updateEntry(shown) === "Update");
    expect(updateReads(heard)).toBe(3);
  });
});

// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AppShell } from "./app-shell.tsx";
import {
  mount,
  servedBy,
  settleUntil,
  type Answer,
  type Answers,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { alertsReadEveryMs, UnknownFailuresNote } from "./unknown-failures-note.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  vi.useRealTimers();
});

/** Two kinds of work past the alert's count in the last hour. */
const raised: Answer = [
  200,
  {
    alerts: [
      { kind: "fetch_metadata", lastHour: 24 },
      { kind: "examine", lastHour: 21 },
    ],
  },
];

const none: Answer = [200, { alerts: [] }];

const banner =
  "Failures of unknown cause are piling up: Looking up, 24 in the last hour; Reading new files, 21 in the last hour.";

const alertReads = (heard: readonly Heard[]): number =>
  heard.filter((each) => each.methodAndPath === "GET /unknown-failures/alerts").length;

async function mountNote(
  isAdmin: boolean,
  alerts: Answers[string],
  heard: Heard[] = [],
): Promise<Mounted> {
  return await mount(
    withServices(
      <UnknownFailuresNote isAdmin={isAdmin} />,
      servedBy({ "GET /unknown-failures/alerts": alerts }, heard),
    ),
  );
}

// D509 §3, D511 §7: while a kind's failures of unknown cause pass the alert's count in an hour, an
// admin is shown a banner above every page naming the kinds, with the way to Needs attention; nobody
// else sees it, nor is the server asked for them.
describe("UnknownFailuresNote", () => {
  it("shows an admin each kind raised with its count in the last hour, and the way to Needs attention", async () => {
    mounted = await mountNote(true, raised);
    const shown = mounted;
    await settleUntil(() => shown.container.textContent.includes(banner));
    const link = shown.container.querySelector('a[href="/needs-attention"]');
    expect(link?.textContent).toBe("See them on Needs attention");
  });

  it("is no banner while no kind's alert is raised", async () => {
    const heard: Heard[] = [];
    mounted = await mountNote(true, none, heard);
    await settleUntil(() => alertReads(heard) === 1);
    expect(mounted.container.textContent).toBe("");
  });

  it("shows anyone but an admin nothing, and asks the server nothing for them", async () => {
    const heard: Heard[] = [];
    mounted = await mountNote(false, raised, heard);
    expect(mounted.container.textContent).toBe("");
    expect(heard).toEqual([]);
  });

  it("comes when the page is shown again after an alert is raised, and goes once it clears", async () => {
    const heard: Heard[] = [];
    mounted = await mountNote(true, [none, raised, none], heard);
    const shown = mounted;
    await settleUntil(() => alertReads(heard) === 1);
    expect(shown.container.textContent).toBe("");
    await act(async () => {
      document.dispatchEvent(new Event("visibilitychange"));
      await Promise.resolve();
    });
    await settleUntil(() => shown.container.textContent.includes(banner));
    await act(async () => {
      document.dispatchEvent(new Event("visibilitychange"));
      await Promise.resolve();
    });
    await settleUntil(() => alertReads(heard) === 3 && shown.container.textContent === "");
  });

  it("reads again every five minutes while the page shows", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const heard: Heard[] = [];
    mounted = await mountNote(true, [none, raised], heard);
    const shown = mounted;
    await settleUntil(() => alertReads(heard) === 1);
    await act(async () => {
      vi.advanceTimersByTime(alertsReadEveryMs - 1);
      await Promise.resolve();
    });
    expect(alertReads(heard)).toBe(1);
    await act(async () => {
      vi.advanceTimersByTime(1);
      await Promise.resolve();
    });
    await settleUntil(() => shown.container.textContent.includes(banner));
  });

  it("stands above every page of an admin's shell", async () => {
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
        servedBy({
          "GET /unknown-failures/alerts": raised,
          // The rest of the shell's own asking: the update, the libraries menu and the tray's sign.
          "GET /system/update": [200, { checkedAt: 0, version: "dev" }],
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
        }),
      ),
    );
    const shown = mounted;
    await settleUntil(() => shown.container.textContent.includes(banner));
    expect(shown.container.querySelector("main")?.textContent).toContain(banner);
  });
});

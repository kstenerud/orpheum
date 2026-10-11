// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { OfflineReminder, type OfflineReminderReport } from "./offline-reminder.tsx";
import { buttonWithText, click, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const twoFoldersAndADisk: OfflineReminderReport = {
  isShown: true,
  rootCount: 2,
  diskCount: 1,
  waitingRootCount: 0,
  unreadCount: 0,
  offlineAt: Date.UTC(2026, 8, 12, 9, 0),
};

it("the reminder says how many folders and disks are offline since when, and leads to the System page", async () => {
  mounted = await mount(
    withServices(
      <OfflineReminder />,
      servedBy({ "GET /offline-reminder": [200, twoFoldersAndADisk] }),
    ),
  );
  const text = mounted.container.textContent;
  expect(text).toContain("2 library folders and 1 disk offline since Sep 12, 2026.");
  expect(mounted.container.querySelector("a")?.getAttribute("href")).toBe("/system");
});

it("dismissing the reminder tells the server and hides it", async () => {
  const heard: Heard[] = [];
  mounted = await mount(
    withServices(
      <OfflineReminder />,
      servedBy(
        {
          "GET /offline-reminder": [200, twoFoldersAndADisk],
          "POST /offline-reminder/dismissal": [204, undefined],
        },
        heard,
      ),
    ),
  );
  await click(buttonWithText(mounted.container, "Remind me later"));
  expect(heard.map((request) => request.methodAndPath)).toContain(
    "POST /offline-reminder/dismissal",
  );
  expect(mounted.container.textContent).not.toContain("offline since");
});

it("the reminder shows nothing when the server says not to", async () => {
  mounted = await mount(
    withServices(
      <OfflineReminder />,
      servedBy({ "GET /offline-reminder": [200, { isShown: false, rootCount: 1, diskCount: 0 }] }),
    ),
  );
  expect(mounted.container.textContent).toBe("");
});

it("the reminder says when a scan waits for the admin and what could not be read, with nothing offline (D277)", async () => {
  mounted = await mount(
    withServices(
      <OfflineReminder />,
      servedBy({
        "GET /offline-reminder": [
          200,
          { isShown: true, rootCount: 0, diskCount: 0, waitingRootCount: 1, unreadCount: 2 },
        ],
      }),
    ),
  );
  const text = mounted.container.textContent;
  expect(text).toContain("A scan waits for you before removing files.");
  expect(text).toContain("2 files or folders could not be read.");
  expect(text).not.toContain("offline");
});

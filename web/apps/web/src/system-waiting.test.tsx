// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { components } from "@orpheum/sdk";
import { afterEach, expect, it } from "vitest";

import { WhatWaits } from "./system-waiting.tsx";
import { buttonWithText, click, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

type SystemReport = components["schemas"]["SystemOutputBody"];

const report = {
  nodes: [{ name: "sandbox", uid: 1000, gid: 100 }],
  waiting: [
    {
      libraryId: 5,
      libraryName: "Series",
      nodeName: "sandbox",
      rootPath: "/media/shows",
      reason: "the scan would remove 7 of the root's 95 files",
      waitingAt: Date.UTC(2026, 9, 11, 10, 45),
      fileCount: 12,
      firstPaths: ["/media/shows/Gus Doyle/S04E01.mkv", "/media/shows/Gus Doyle/S04E02.mkv"],
    },
  ],
  unread: [
    {
      libraryId: 5,
      libraryName: "Series",
      nodeName: "sandbox",
      path: "/media/shows/Gus Doyle/S04",
      code: "permissionDenied",
      foundAt: 1,
      errorDetail: {
        text: "open Gus Doyle/S04: permission denied",
        ownerUid: 0,
        groupGid: 0,
        modeText: "drwxr-x---",
        occurrenceCount: 1,
      },
    },
  ],
} as unknown as SystemReport;

it("names what a held-back scan would remove, and removes it on the admin's word (D277 §2)", async () => {
  const heard: Heard[] = [];
  let changedCount = 0;
  mounted = await mount(
    withServices(
      <WhatWaits
        report={report}
        onChanged={() => {
          changedCount += 1;
        }}
      />,
      servedBy({ "POST /libraries/5/roots/removals": [204, undefined] }, heard),
    ),
  );
  const text = mounted.container.textContent;
  expect(text).toContain("sandbox: /media/shows, in Series");
  expect(text).toContain("would remove 12 files");
  expect(text).toContain("/media/shows/Gus Doyle/S04E01.mkv");
  expect(text).toContain("and 10 more");
  await click(buttonWithText(mounted.container, "Remove these 12 files"));
  // It cannot be undone: it is asked again (D278 §4).
  await click(buttonWithText(mounted.container, "Remove them"));
  expect(
    heard.find((each) => each.methodAndPath === "POST /libraries/5/roots/removals")?.body,
  ).toEqual({
    nodeName: "sandbox",
    path: "/media/shows",
  });
  expect(changedCount).toBe(1);
});

it("says what could not be read, whose it is, and how to make it readable to the node (D277 §3)", async () => {
  mounted = await mount(withServices(<WhatWaits report={report} />, servedBy({})));
  const text = mounted.container.textContent;
  expect(text).toContain("/media/shows/Gus Doyle/S04");
  expect(text).toContain("It belongs to user 0 and group 0, with mode drwxr-x---.");
  expect(text).toContain("The node reads as user 1000 and group 100");
  expect(text).toContain("chown -R 1000:100");
});

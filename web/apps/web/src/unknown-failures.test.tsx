// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it, vi } from "vitest";

import { NeedsAttention } from "./needs-attention.tsx";
import {
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import type { UnknownFailuresReport } from "./unknown-failures.tsx";
import { IsAdminContext } from "./warning-sign.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
});

const nothingWrong = {
  rootCount: 0,
  diskCount: 0,
  waitingRootCount: 0,
  unreadCount: 0,
  matchCount: 0,
  seriesReadWronglyCount: 0,
  filmsWithoutSoundCount: 0,
  copyRefusedAt: 0,
  copyRefusedFrom: "",
  stoodAsideAt: 0,
};

const noUpdate = { "GET /system/update": [200, { version: "v0.9.0", checkedAt: 1 }] } as const;

/** An error and an output longer than any line, which the page must show whole. */
const longError = `provider answered: not JSON ${"x".repeat(400)} (end of error)`;
const longOutput = `<html>\n${"<p>an answer that is no JSON</p>\n".repeat(30)}</html> (end of output)`;

/** Two kinds: the lookups' raised, with two failures, and the readings' not, with one. */
const report: UnknownFailuresReport = {
  at: Date.UTC(2026, 10, 17, 9, 30),
  version: "v0.9.0",
  startId: "start 7",
  kinds: [
    {
      kind: "fetch_metadata",
      lastHour: 24,
      sinceStart: 31,
      lastAt: Date.UTC(2026, 10, 17, 9, 29),
      isRaised: true,
      failures: [
        {
          subject: "item 12",
          error: longError,
          output: longOutput,
          version: "v0.9.0",
          firstAt: Date.UTC(2026, 10, 17, 8, 0),
          lastAt: Date.UTC(2026, 10, 17, 9, 29),
          count: 3,
          isThisStart: true,
        },
        {
          subject: "item 13",
          error: "unexpected end of input",
          output: "",
          version: "v0.8.2",
          firstAt: Date.UTC(2026, 10, 16, 8, 0),
          lastAt: Date.UTC(2026, 10, 16, 8, 0),
          count: 1,
          isThisStart: false,
        },
      ],
    },
    {
      kind: "examine",
      lastHour: 0,
      sinceStart: 2,
      lastAt: Date.UTC(2026, 10, 17, 7, 0),
      isRaised: false,
      failures: [
        {
          subject: "file 12",
          error: "ffprobe exited 1",
          output: "moov atom not found",
          version: "v0.9.0",
          firstAt: Date.UTC(2026, 10, 17, 7, 0),
          lastAt: Date.UTC(2026, 10, 17, 7, 0),
          count: 2,
          isThisStart: true,
        },
      ],
    },
  ],
};

async function mountPage(
  isAdmin: boolean,
  failures: UnknownFailuresReport,
  heard: Heard[] = [],
): Promise<Mounted> {
  return await mount(
    withServices(
      <IsAdminContext value={isAdmin}>
        <NeedsAttention />
      </IsAdminContext>,
      servedBy(
        {
          "GET /needs-attention": [200, nothingWrong],
          ...noUpdate,
          "GET /unknown-failures": [200, failures],
        },
        heard,
      ),
    ),
  );
}

/** The entries of the group of failures of unknown cause, one per kind. */
function kindEntries(page: Mounted): Element[] {
  return [...page.container.querySelectorAll(".unknown-failures > ul > li")];
}

// D509 §4, D511 §6: the Needs attention page lists the failures of unknown cause per kind, with its
// counts, when one last failed and whether its alert is raised, each kind opening on its failures in
// full; nobody but an admin sees them, nor is the server asked for them (D511 §7).
describe("the Needs attention page's failures of unknown cause", () => {
  it("lists each kind with its counts, when one last failed and whether its alert is raised", async () => {
    mounted = await mountPage(true, report);
    const entries = kindEntries(mounted);
    expect(entries).toHaveLength(2);
    const [lookups, readings] = entries.map((entry) => entry.querySelector("summary")?.textContent);
    expect(lookups).toContain("Looking up");
    expect(lookups).toContain("24 in the last hour, 31 since the server started, the last");
    expect(lookups).toContain("Its alert is raised.");
    expect(entries[0]?.querySelector(".warning-icon")).not.toBeNull();
    expect(readings).toContain("Reading new files");
    expect(readings).toContain("0 in the last hour, 2 since the server started");
    expect(readings).not.toContain("Its alert is raised.");
    expect(mounted.container.textContent).not.toContain("Nothing needs your attention.");
  });

  it("opens each kind on its failures, the error and the output in full", async () => {
    mounted = await mountPage(true, report);
    const [lookups] = kindEntries(mounted);
    const failures = [...(lookups?.querySelectorAll(".unknown-failures__failure") ?? [])];
    expect(failures.map((each) => each.querySelector("strong")?.textContent)).toEqual([
      "item 12",
      "item 13",
    ]);
    const blocks = [...(failures[0]?.querySelectorAll("pre") ?? [])].map(
      (each) => each.textContent,
    );
    expect(blocks).toEqual([longError, longOutput]);
    expect(failures[0]?.textContent).toContain("Failed 3 times, on v0.9.0, the first");
    expect(failures[1]?.textContent).toContain("unexpected end of input");
    expect(failures[1]?.textContent).toContain("No output.");
    expect(failures[1]?.textContent).toContain("From an earlier start of the server.");
    expect(failures[0]?.textContent).not.toContain("From an earlier start of the server.");
  });

  // D511 §6: Copy report gives the whole of GET /unknown-failures's answer, for the lead to pull from
  // a server and a user to send.
  it("copies the whole report as pretty-printed JSON, and says it was copied", async () => {
    const writes: string[] = [];
    vi.stubGlobal("isSecureContext", true);
    vi.spyOn(globalThis.navigator.clipboard, "writeText").mockImplementation((text: string) =>
      Promise.resolve(void writes.push(text)),
    );
    mounted = await mountPage(true, report);
    await click(buttonWithText(mounted.container, "Copy report"));
    expect(writes).toEqual([JSON.stringify(report, null, 2)]);
    expect(mounted.container.textContent).toContain("Copied: every failure, in full.");
  });

  it("shows the report to select when the browser refuses the copy", async () => {
    vi.stubGlobal("isSecureContext", false);
    mounted = await mountPage(true, report);
    const shown = mounted;
    expect(shown.container.querySelector(".unknown-failures__report")).toBeNull();
    await click(buttonWithText(shown.container, "Copy report"));
    await settleUntil(() => shown.container.textContent.includes("wouldn't copy it"));
    expect(shown.container.querySelector(".unknown-failures__report")?.textContent).toBe(
      JSON.stringify(report, null, 2),
    );
  });

  it("is no group while there are none", async () => {
    mounted = await mountPage(true, { ...report, kinds: [] });
    expect(mounted.container.querySelector(".unknown-failures")).toBeNull();
    expect(mounted.container.textContent).toContain("Nothing needs your attention.");
  });

  it("shows anyone but an admin nothing of them, and asks the server nothing for them", async () => {
    const heard: Heard[] = [];
    mounted = await mountPage(false, report, heard);
    expect(mounted.container.querySelector(".unknown-failures")).toBeNull();
    expect(heard.map((each) => each.methodAndPath)).not.toContain("GET /unknown-failures");
  });
});

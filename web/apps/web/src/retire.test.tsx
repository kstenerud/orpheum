// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { LibraryPage } from "./library-manage.tsx";
import { System, type SystemReport } from "./system.tsx";
import {
  buttonNamed,
  buttonWithText,
  click,
  mount,
  servedBy,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** The backups section's answer when there are none (D242 §5). */
const noBackups = [200, { directory: "/srv/orpheum/backups", backups: [] }] as const;
const noDiscovery = [
  200,
  {
    issuer: "",
    published: [],
    probe: [],
    claimExpiresAt: 0,
    lastHeartbeatAt: 0,
    portMapping: { isWanted: false, checkedAt: 0, mappings: [], problems: [] },
  },
] as const;

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const films = {
  id: 3,
  name: "Films",
  kind: "film",
  metadataSource: "nfo",
  changeDetection: "manual",
  isWritable: false,
  createdAt: 1,
  roots: [{ nodeName: "local", rootPath: "/media/films", lastScanOutcome: "unreadable" }],
} as const;

const report: SystemReport = {
  server: {
    build: { name: "v0.3.0", version: "v0.3.0", hasUncommittedChanges: false },
    host: { platform: "linux/amd64" },
    goVersion: "go1.26.1",
    startedAt: 1_790_000_000_000,
    isMultipathTcpOn: false,
    addressAtHome: { address: "", origin: "" },
    disks: [],
    databases: [],
  },
  findings: [],
  nodes: [
    {
      name: "attic",
      connection: "awaitingHello",
      isConnected: false,
      isServersBuild: false,
      uid: 0,
      gid: 0,
      confinement: [],
      disks: [],
      devices: [],
      stuck: [],
    },
  ],
  destinations: [],
  providers: [],
  queue: [],
  offline: [
    {
      libraryId: 3,
      libraryName: "Films",
      nodeName: "local",
      rootPath: "/media/films",
      offlineAt: 1_790_000_000_000,
    },
    {
      libraryId: 3,
      libraryName: "Films",
      nodeName: "local",
      rootPath: "/media/films",
      mountPath: "/media/films/disk1",
      offlineAt: 1_790_000_000_000,
    },
  ],
  waiting: [],
  unread: [],
};

it("retires a root only once the admin confirms, and keeping it sends nothing (D231 §7)", async () => {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "GET /libraries": [
        [200, { libraries: [films] }],
        [200, { libraries: [{ ...films, roots: [] }] }],
      ],
      "GET /nodes": [200, { nodes: [] }],
      "POST /libraries/3/roots/retire": [200, { tombstonedFileCount: 40, cancelledJobCount: 2 }],
    },
    heard,
  );
  mounted = await mount(withServices(<LibraryPage libraryId={3} />, services));
  const container = mounted.container;
  // Under the folder's own "…" (ui-audit-2026-11-04.md §3), its dialog's action marked dangerous, so
  // focus arrives on Keep it (D421; dangerous-choices.browser.test.tsx).
  const repairs = (): HTMLButtonElement | undefined =>
    buttonNamed(container, "More for local: /media/films");
  await click(repairs());
  await click(buttonWithText(container, "This folder is gone for good…"));
  expect(container.querySelector("dialog")?.textContent).toContain(
    "it is never scanned again; what was watched is kept.",
  );
  expect(container.querySelector("dialog [data-dangerous-action]")?.textContent).toBe(
    "Retire the folder",
  );
  // The safe choice first in the row, too (D421).
  expect([...container.querySelectorAll("dialog p button")].map((b) => b.textContent)).toEqual([
    "Keep it",
    "Retire the folder",
  ]);
  await click(buttonWithText(container, "Keep it"));
  expect(container.querySelector("dialog")).toBeNull();
  await click(repairs());
  await click(buttonWithText(container, "This folder is gone for good…"));
  await click(buttonWithText(container, "Retire the folder"));
  expect(heard.filter((each) => each.methodAndPath.startsWith("POST"))).toEqual([
    {
      methodAndPath: "POST /libraries/3/roots/retire",
      body: { nodeName: "local", path: "/media/films" },
    },
  ]);
  expect(container.textContent).not.toContain("local: /media/films");
});

// ui-audit-2026-11-04.md §3, A library: what is done most is first, and the folders' repairs wait
// under their "…".
it("puts Scan now first on a library's page, focus arriving on it, its folders' repairs under a \"…\"", async () => {
  const services = servedBy({
    "GET /libraries": [200, { libraries: [films] }],
    "GET /nodes": [200, { nodes: [] }],
  });
  mounted = await mount(withServices(<LibraryPage libraryId={3} />, services));
  const page = mounted.container.querySelector("article.library");
  const firstControl = page?.querySelector("button, a, select, input");
  expect(firstControl?.textContent).toBe("Scan now");
  expect(firstControl?.getAttribute("data-focus-start")).toBe("arrival");
  expect(page?.textContent).not.toContain("This folder is gone for good…");
  expect(buttonNamed(mounted.container, "More for local: /media/films")).toBeDefined();
});

it("deletes a library only once the admin confirms (D231 §7)", async () => {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "GET /libraries": [
        [200, { libraries: [films] }],
        [200, { libraries: [] }],
      ],
      "GET /nodes": [200, { nodes: [] }],
      "DELETE /libraries/3": [200, { tombstonedFileCount: 40, cancelledJobCount: 2 }],
    },
    heard,
  );
  mounted = await mount(withServices(<LibraryPage libraryId={3} />, services));
  await click(buttonWithText(mounted.container, "Delete the library…"));
  expect(heard.map((each) => each.methodAndPath)).not.toContain("DELETE /libraries/3");
  // The confirmation says what deleting does: nothing of the media or of what was watched goes (D363 §10).
  expect(mounted.container.textContent).toContain("Nothing is deleted");
  await click(buttonWithText(mounted.container, "Delete it"));
  expect(heard.map((each) => each.methodAndPath)).toContain("DELETE /libraries/3");
  // Deleted from its own page, the admin is back at the list (D361 §2).
  expect(services.navigation.route()).toEqual({ screen: "libraries" });
});

it("the System page says since when each folder and disk is offline, and retires a machine once confirmed (D41, D231 §7)", async () => {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "GET /backups": noBackups,
      "GET /system/resource-use": [200, { summaries: [], hours: [] }],
      "GET /config/unused": [200, { unusedSettings: [] }],
      "GET /system/discovery": noDiscovery,
      "GET /system/update": [200, { version: "dev", checkedAt: 0 }],
      "GET /system/network": [409, { error: { code: "conflict", message: "no network settings" } }],
      "GET /now-playing": [200, { playbacks: [] }],
      "GET /filing-reports": [200, { reports: [] }],
      "GET /system": [
        [200, report],
        [200, { ...report, nodes: [] }],
      ],
      "POST /nodes/attic/retire": [200, { tombstonedFileCount: 3, cancelledJobCount: 1 }],
    },
    heard,
  );
  mounted = await mount(withServices(<System />, services));
  const text = mounted.container.textContent;
  expect(text).toContain("local: /media/films, in Films: offline since");
  expect(text).toContain("What was mounted at local: /media/films/disk1, in Films: missing since");
  await click(buttonWithText(mounted.container, "This machine is gone for good…"));
  await click(buttonWithText(mounted.container, "Retire the machine"));
  expect(heard.filter((each) => each.methodAndPath.startsWith("POST"))).toEqual([
    { methodAndPath: "POST /nodes/attic/retire", body: undefined },
  ]);
  expect(mounted.container.textContent).not.toContain("Machine: attic");
});

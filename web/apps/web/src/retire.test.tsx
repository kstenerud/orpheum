// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { Libraries } from "./libraries.tsx";
import { System, type SystemReport } from "./system.tsx";
import { buttonWithText, click, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** The backups section's answer when there are none (D242 §5). */
const noBackups = [200, { directory: "/srv/orpheum/backups", backups: [] }] as const;
const noDiscovery = [
  200,
  {
    baseUrl: "",
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
  kind: "movies",
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
    disks: [],
    databases: [],
  },
  findings: [],
  nodes: [
    {
      name: "attic",
      connection: "awaiting_hello",
      isConnected: false,
      isServersBuild: false,
      uid: 0,
      gid: 0,
      confinement: [],
      disks: [],
      recentPrepares: [],
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
  mounted = await mount(withServices(<Libraries />, services));
  const container = mounted.container;
  await click(buttonWithText(container, "This folder is gone for good…"));
  expect(container.textContent).toContain("it is never scanned again; what was watched is kept.");
  await click(buttonWithText(container, "Keep it"));
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
  mounted = await mount(withServices(<Libraries />, services));
  await click(buttonWithText(mounted.container, "Delete the library…"));
  expect(heard.map((each) => each.methodAndPath)).not.toContain("DELETE /libraries/3");
  await click(buttonWithText(mounted.container, "Delete it"));
  expect(heard.map((each) => each.methodAndPath)).toContain("DELETE /libraries/3");
  expect(mounted.container.textContent).toContain("No libraries yet.");
});

it("the System page says since when each folder and disk is offline, and retires a machine once confirmed (D41, D231 §7)", async () => {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "GET /backups": noBackups,
      "GET /system/discovery": noDiscovery,
      "GET /now-playing": [200, { playbacks: [] }],
      "GET /system": [
        [200, report],
        [200, { ...report, nodes: [] }],
      ],
      "POST /nodes/retire": [200, { tombstonedFileCount: 3, cancelledJobCount: 1 }],
    },
    heard,
  );
  mounted = await mount(withServices(<System />, services));
  const text = mounted.container.textContent;
  expect(text).toContain("local: /media/films, in Films: offline since");
  expect(text).toContain("The disk at local: /media/films/disk1, in Films: missing since");
  await click(buttonWithText(mounted.container, "This machine is gone for good…"));
  await click(buttonWithText(mounted.container, "Retire the machine"));
  expect(heard.filter((each) => each.methodAndPath.startsWith("POST"))).toEqual([
    { methodAndPath: "POST /nodes/retire", body: { nodeName: "attic" } },
  ]);
  expect(mounted.container.textContent).not.toContain("Machine: attic");
});

// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { CreateLibraryPage } from "./create-library.tsx";
import { Libraries } from "./libraries.tsx";
import {
  buttonWithText,
  click,
  enter,
  mount,
  servedBy,
  settleAnswers,
  submit,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const films = {
  id: 3,
  name: "Films",
  kind: "film",
  metadataSource: "tmdb",
  changeDetection: "interval",
  checkEveryHours: 6,
  isWritable: false,
  createdAt: 1,
  fileCountsByFiling: { listed: 40 },
  rescan: { isScanning: false, itemCount: 40, matchedCount: 12 },
  roots: [
    {
      nodeName: "nas",
      rootPath: "/tank/media/films",
      lastScanOutcome: "unreadable",
      scan: {
        isListed: false,
        listedCount: 0,
        toExamineCount: 0,
        examinedCount: 0,
        isAnotherScanQueued: false,
      },
    },
    { nodeName: "nas", rootPath: "/tank/media/more-films", lastScanOutcome: "complete" },
  ],
} as const;

describe("the admin's Libraries pages (D361)", () => {
  it("lists each library at a glance, leading to its own page to manage it", async () => {
    const services = servedBy({ "GET /libraries": [200, { libraries: [films] }] });
    mounted = await mount(withServices(<Libraries />, services));
    const page = mounted.container;
    const row = page.querySelector("li.libraries__library");
    expect(row?.querySelector("h2")?.textContent).toBe("Films");
    expect(row?.textContent).toContain("Films · Folders: 2 · checked every 6 hours · scanning now");
    expect(row?.textContent).toContain("nas: /tank/media/films");
    expect(row?.textContent).toContain("nas: /tank/media/more-films");
    // The folder that could not be read carries its warning sign, the other none.
    expect(row?.querySelectorAll(".warning-sign")).toHaveLength(1);
    expect(row?.textContent).toContain("Listed: 40");
    expect(row?.textContent).toContain("Rescanning… 12 of 40 films and series matched again.");
    // What changes a library is on its own page, not in the list.
    expect(buttonWithText(page, "Delete the library…")).toBeUndefined();
    expect(buttonWithText(page, "Scan now")).toBeUndefined();
    expect(buttonWithText(page, "Rescan everything")).toBeDefined();
  });

  it("leads to creating a library, and to each library's own page", async () => {
    const services = servedBy({ "GET /libraries": [200, { libraries: [films] }] });
    mounted = await mount(withServices(<Libraries />, services));
    const page = mounted.container;
    const create = [...page.querySelectorAll("a")].find(
      (link) => link.textContent === "Create a library",
    );
    expect(create?.getAttribute("href")).toBe("/libraries/new");
    const manage = [...page.querySelectorAll("a")].find(
      (link) => link.textContent === "Manage this library",
    );
    expect(manage?.getAttribute("href")).toBe("/libraries/3");
    await click(manage);
    expect(services.navigation.route()).toEqual({ screen: "library", libraryId: 3 });
  });

  it("says so when there is no library yet", async () => {
    const services = servedBy({ "GET /libraries": [200, { libraries: [] }] });
    mounted = await mount(withServices(<Libraries />, services));
    expect(mounted.container.textContent).toContain("No libraries yet.");
  });

  it("creates a library on its own page, then shows the new library's page in its place", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /nodes": [200, { nodes: [{ name: "nas", isConnected: true, rootPaths: ["/tank"] }] }],
        "GET /config": [
          200,
          {
            settings: [
              {
                key: "new_library_default_scan_every_hours",
                kind: "integer",
                value: 24,
                startingValue: 24,
                isModified: false,
                isImportant: false,
                section: "libraries",
                description: "",
                setAt: 1,
              },
            ],
          },
        ],
        "POST /libraries": [201, { id: 7 }],
      },
      heard,
    );
    services.navigation.go({ screen: "libraries" });
    services.navigation.go({ screen: "new-library" });
    mounted = await mount(withServices(<CreateLibraryPage />, services));
    const page = mounted.container;
    await settleAnswers();
    expect(page.querySelector("h1")?.textContent).toBe("Create a library");
    await enter(page.querySelector("input:not([type])"), "Films");
    await click(page.querySelector('input[type="checkbox"]'));
    await submit(page.querySelector("form"));
    expect(heard.filter((each) => each.methodAndPath === "POST /libraries")).toHaveLength(1);
    expect(services.navigation.route()).toEqual({ screen: "library", libraryId: 7 });
    // The form sent is replaced, so Back from the new library goes to the list.
    services.navigation.back();
    expect(services.navigation.route()).toEqual({ screen: "libraries" });
  });
});

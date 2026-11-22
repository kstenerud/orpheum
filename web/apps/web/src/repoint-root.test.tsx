// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { RepointDialog, type Place } from "./repoint-root.tsx";
import {
  buttonWithText,
  click,
  mount,
  servedBy,
  settleAnswers,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** A dry run of a repoint that would move two files and refuses nothing. */
const cleanReport = {
  livePlaceCount: 2,
  removedPlaceCount: 0,
  examples: [],
  roots: [],
  libraries: [],
  isCrossNode: false,
  targetState: "readable",
  targetFolders: [],
  conflictCount: 0,
  conflictExamples: [],
  lookAlikeCount: 0,
  refusals: [],
};

const places = [
  { nodeName: "devnas", path: "/media/films" },
  { nodeName: "devnas", path: "/mnt/films" },
  { nodeName: "attic", path: "/films" },
];

/**
 * Chooses the place of the machine named in Choose: opened, at the folder's own place, up to the
 * places, into the machine's one, and used.
 */
async function chooseAtThePlaces(container: HTMLElement, machine: string): Promise<void> {
  await click(buttonWithText(container, "Choose a folder inside one…"));
  await settleAnswers();
  await click(buttonWithText(container, "Places"));
  const group = [...container.querySelectorAll(".file-browser__group")].find(
    (each) => each.querySelector(".file-browser__group-heading")?.textContent === machine,
  );
  await click(group?.querySelector(".file-browser__row") ?? null);
  await settleAnswers();
  await click(buttonWithText(container, "Use films"));
}

/** Mounts the dialog for /media/films on devnas, the server answering each dry run in turn. */
async function mountDialog(
  reports: readonly unknown[],
  moves: Place[],
  heard: Heard[] = [],
): Promise<Mounted> {
  return await mount(
    withServices(
      <RepointDialog
        from={{ nodeName: "devnas", path: "/media/films" }}
        places={places}
        onRepoint={(to) => moves.push(to)}
        onClose={() => undefined}
      />,
      servedBy(
        {
          "POST /repoints/preview": reports.map((report) => [200, report] as const),
          "GET /nodes/devnas/folders": [
            200,
            { path: "/media/films", state: "readable", folders: [] },
          ],
          "GET /nodes/attic/folders": [200, { path: "/films", state: "readable", folders: [] }],
        },
        heard,
      ),
    ),
  );
}

describe("saying a folder moved (D429 §2)", () => {
  it("offers the other folders, checks the move with a dry run, and moves it only once checked and confirmed", async () => {
    const moves: Place[] = [];
    const heard: Heard[] = [];
    mounted = await mountDialog([cleanReport], moves, heard);
    // It starts at the first other folder; another is chosen in Choose, at the places.
    expect(mounted.container.querySelector("output")?.textContent).toBe("devnas: /mnt/films");
    await chooseAtThePlaces(mounted.container, "attic");
    expect(mounted.container.querySelector("output")?.textContent).toBe("attic: /films");
    expect(mounted.container.textContent).not.toContain("Use the new place");
    await click(buttonWithText(mounted.container, "Check what moves"));
    await settleAnswers();
    const previews = heard.filter((each) => each.methodAndPath === "POST /repoints/preview");
    expect(previews.map((each) => each.body)).toEqual([
      { nodeName: "devnas", path: "/media/films", newNodeName: "attic", newPath: "/films" },
    ]);
    expect(mounted.container.textContent).toContain("2 files and folders move there");
    expect(moves).toEqual([]);
    await click(buttonWithText(mounted.container, "Use the new place"));
    expect(moves).toEqual([{ nodeName: "attic", path: "/films" }]);
  });

  it("offers no move when the dry run refuses it, and shows what is in the way", async () => {
    const moves: Place[] = [];
    const refused = {
      ...cleanReport,
      refusals: ["targetHoldsKnown"],
      conflictCount: 7,
      conflictExamples: ["/mnt/films/Arkon.mkv"],
    };
    mounted = await mountDialog([refused], moves);
    await click(buttonWithText(mounted.container, "Check what moves"));
    await settleAnswers();
    const text = mounted.container.textContent;
    expect(text).toContain("a move never merges into them");
    expect(text).toContain("7 in all");
    expect(text).toContain("/mnt/films/Arkon.mkv");
    expect(text).not.toContain("Use the new place");
  });

  it("forgets the check when another place is chosen", async () => {
    const moves: Place[] = [];
    mounted = await mountDialog([cleanReport], moves);
    await click(buttonWithText(mounted.container, "Check what moves"));
    await settleAnswers();
    expect(mounted.container.textContent).toContain("Use the new place");
    await chooseAtThePlaces(mounted.container, "attic");
    expect(mounted.container.textContent).not.toContain("Use the new place");
  });
});

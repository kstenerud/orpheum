// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { RepointDialog, type Place } from "./repoint-root.tsx";
import {
  buttonWithText,
  choose,
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

/** Mounts the dialog for /media/films on devnas, the server answering each dry run in turn. */
async function mountDialog(
  reports: readonly unknown[],
  moves: Place[],
  heard: Heard[] = [],
): Promise<Mounted> {
  return mount(
    withServices(
      <RepointDialog
        from={{ nodeName: "devnas", path: "/media/films" }}
        places={places}
        onRepoint={(to) => moves.push(to)}
        onClose={() => undefined}
      />,
      servedBy(
        { "POST /repoints/preview": reports.map((report) => [200, report] as const) },
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
    const options = [...mounted.container.querySelectorAll("option")].map(
      (option) => option.textContent,
    );
    expect(options).toEqual(["devnas: /mnt/films", "attic: /films"]);
    await choose(mounted.container.querySelector("select"), "1");
    expect(mounted.container.textContent).not.toContain("Use the new place");
    await click(buttonWithText(mounted.container, "Check what moves"));
    await settleAnswers();
    expect(heard.map((each) => each.body)).toEqual([
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
    await choose(mounted.container.querySelector("select"), "1");
    expect(mounted.container.textContent).not.toContain("Use the new place");
  });
});

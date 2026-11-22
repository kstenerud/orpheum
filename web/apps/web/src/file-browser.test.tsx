// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { AddRoot } from "./add-root.tsx";
import { CreateLibraryForm, foldersOffered } from "./create-library.tsx";
import { Choose, ChooseOpener } from "./file-browser-choose.tsx";
import {
  FileBrowser,
  folderAbove,
  folderInside,
  inNaturalOrder,
  sidePanelRestMs,
  type Place,
} from "./file-browser.tsx";
import { blankLibrary, type NewLibrary } from "./library-draft.ts";
import { RepointDialog } from "./repoint-root.tsx";
import {
  buttonNamed,
  buttonWithText,
  click,
  enter,
  fire,
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

/** A node's answer: the folders in a folder, readable, with the videos counted in some. */
function readable(
  path: string,
  folders: readonly string[],
  videoCounts: Readonly<Record<string, number>> = {},
): readonly [number, unknown] {
  return [200, { path, state: "readable", folders, videoCounts }];
}

/** The paths the folders were asked for in, in order. */
function askedPaths(heard: readonly Heard[]): string[] {
  return heard
    .filter((each) => each.methodAndPath === "GET /nodes/nas/folders")
    .map((each) => new URLSearchParams(each.query).get("path") ?? "");
}

/** The names of the rows shown, in order. */
function rowNames(container: HTMLElement): (string | null)[] {
  return [...container.querySelectorAll(".file-browser__row")].map((row) =>
    row.getAttribute("aria-label"),
  );
}

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

const places: readonly Place[] = [{ nodeName: "nas", path: "/tank/media" }];

/** Choose over places, open, what it chooses kept in chosen. */
async function mountChoose(
  services: ReturnType<typeof servedBy>,
  chosen: Place[],
  over: { readonly places?: readonly Place[]; readonly away?: string[] } = {},
): Promise<Mounted> {
  return await mount(
    withServices(
      <Choose
        heading="Pick a folder"
        places={over.places ?? places}
        away={over.away}
        onChoose={(place) => chosen.push(place)}
        onClose={() => undefined}
      />,
      services,
    ),
  );
}

describe("the file browser's Choose (D539 §1, §2)", () => {
  it("lists every page of a folder's folders, following each page's cursor (D432 §3)", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /nodes/nas/folders": [
          [
            200,
            {
              path: "/tank/media",
              state: "readable",
              folders: ["music"],
              videoCounts: { music: 0 },
              nextCursor: "next",
            },
          ],
          readable("/tank/media", ["video"], { video: 12 }),
        ],
      },
      heard,
    );
    mounted = await mountChoose(services, []);
    await settleAnswers();
    expect(rowNames(mounted.container)).toEqual(["music", "video"]);
    expect(mounted.container.textContent).toContain("12 videos");
    const cursors = heard
      .filter((each) => each.methodAndPath === "GET /nodes/nas/folders")
      .map((each) => new URLSearchParams(each.query).get("cursor"));
    expect(cursors).toEqual([null, "next"]);
  });

  it("shows no count for a folder the machine has not counted, never 0", async () => {
    const services = servedBy({
      "GET /nodes/nas/folders": [readable("/tank/media", ["counted", "slow"], { counted: 3 })],
    });
    mounted = await mountChoose(services, []);
    await settleAnswers();
    const details = [...mounted.container.querySelectorAll(".file-browser__row")].map(
      (row) => row.querySelector(".file-browser__details")?.textContent ?? "",
    );
    expect(details).toEqual(["3 videos", ""]);
  });

  it("puts folders in natural order, ignoring case", () => {
    expect(inNaturalOrder(["Season 10", "season 2", "Extras", "Season 1"], (name) => name)).toEqual(
      ["Extras", "Season 1", "season 2", "Season 10"],
    );
  });

  it("browses into the folders beneath a place, up as far as the place, and chooses one", async () => {
    const heard: Heard[] = [];
    const chosen: Place[] = [];
    const services = servedBy(
      {
        "GET /nodes/nas/folders": [
          readable("/tank/media", ["music", "video"]),
          readable("/tank/media/video", ["movies", "shows"]),
          readable("/tank/media", ["music", "video"]),
          readable("/tank/media/video", ["movies", "shows"]),
          readable("/tank/media/video/movies", []),
        ],
      },
      heard,
    );
    mounted = await mountChoose(services, chosen);
    await settleAnswers();
    await click(buttonNamed(mounted.container, "video"));
    await settleAnswers();
    expect(rowNames(mounted.container)).toEqual(["movies", "shows"]);
    await click(buttonNamed(mounted.container, "Up one folder"));
    await settleAnswers();
    await click(buttonNamed(mounted.container, "video"));
    await settleAnswers();
    await click(buttonNamed(mounted.container, "movies"));
    await settleAnswers();
    expect(mounted.container.textContent).toContain("No folders in movies");
    await click(buttonWithText(mounted.container, "Use movies"));
    expect(chosen).toEqual([{ nodeName: "nas", path: "/tank/media/video/movies" }]);
    expect(askedPaths(heard)).toEqual([
      "/tank/media",
      "/tank/media/video",
      "/tank/media",
      "/tank/media/video",
      "/tank/media/video/movies",
    ]);
  });

  it("starts at the places, grouped by machine, one not answering said in their place", async () => {
    const chosen: Place[] = [];
    mounted = await mountChoose(servedBy({}), chosen, {
      places: [
        { nodeName: "nas", path: "/tank/media" },
        { nodeName: "nas", path: "/tank/films" },
        { nodeName: "attic", path: "/films" },
      ],
      away: ["cellar"],
    });
    const headings = [...mounted.container.querySelectorAll(".file-browser__group-heading")].map(
      (heading) => heading.textContent,
    );
    expect(headings).toEqual(["attic", "nas", "cellar"]);
    expect(rowNames(mounted.container)).toEqual(["films", "films", "media"]);
    expect(mounted.container.textContent).toContain("cellar isn't answering");
    expect(mounted.container.querySelector("select")).toBeNull();
    await click(buttonWithText(mounted.container, "Use this folder"));
    expect(mounted.container.textContent).toContain("Open a folder first");
    expect(chosen).toEqual([]);
  });

  it("says a folder whose share does not answer is not answering, and asks again on Try again", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /nodes/nas/folders": [
          [200, { path: "/tank/media", state: "notAnswering", folders: [] }],
          readable("/tank/media", ["video"]),
        ],
      },
      heard,
    );
    mounted = await mountChoose(services, []);
    await settleAnswers();
    expect(mounted.container.textContent).toContain("nas isn't answering");
    await click(buttonWithText(mounted.container, "Try again"));
    await settleAnswers();
    expect(buttonNamed(mounted.container, "video")).toBeDefined();
    expect(askedPaths(heard)).toHaveLength(2);
  });

  it("opens at a folder that has gone as the nearest folder above it that is there", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /nodes/nas/folders": [
          [200, { path: "/tank/media/old", state: "missing", folders: [] }],
          readable("/tank/media", ["video"]),
        ],
      },
      heard,
    );
    mounted = await mount(
      withServices(
        <Choose
          heading="Pick a folder"
          places={places}
          startAt={{ nodeName: "nas", path: "/tank/media/old" }}
          onChoose={() => undefined}
          onClose={() => undefined}
        />,
        services,
      ),
    );
    await settleAnswers();
    await settleAnswers();
    expect(askedPaths(heard)).toEqual(["/tank/media/old", "/tank/media"]);
    expect(rowNames(mounted.container)).toEqual(["video"]);
  });

  it("opens a typed path with Go to…, and refuses one outside the places", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      {
        "GET /nodes/nas/folders": [readable("/tank/media", []), readable("/tank/media/video", [])],
      },
      heard,
    );
    mounted = await mountChoose(services, []);
    await settleAnswers();
    await click(buttonWithText(mounted.container, "Go to…"));
    const field = mounted.container.querySelector<HTMLInputElement>(".file-browser__go-to input");
    expect(field?.value).toBe("/tank/media");
    const type = async (value: string): Promise<void> => {
      await enter(field, value);
      await fire(
        mounted?.container.querySelector(".file-browser__go-to"),
        new Event("submit", { bubbles: true, cancelable: true }),
      );
    };
    await type("/etc");
    expect(mounted.container.textContent).toContain(
      "That path is not inside any of the places here.",
    );
    await type("/tank/media/video/");
    await settleAnswers();
    expect(askedPaths(heard)).toEqual(["/tank/media", "/tank/media/video"]);
    expect(mounted.container.querySelector(".file-browser__go-to")).toBeNull();
  });

  it("chooses in a modal dialog over the page, which choosing a folder closes (D361 §3)", async () => {
    const chosen: Place[] = [];
    const services = servedBy({
      "GET /nodes/nas/folders": [readable("/tank/media", ["video"])],
    });
    mounted = await mount(
      withServices(
        <ChooseOpener
          opener="Pick…"
          heading="Pick a folder"
          places={places}
          onChoose={(place) => chosen.push(place)}
        />,
        services,
      ),
    );
    const page = mounted.container;
    expect(page.querySelector("dialog")).toBeNull();
    await click(buttonWithText(page, "Pick…"));
    const dialog = page.querySelector("dialog");
    expect(dialog?.open).toBe(true);
    expect(dialog?.getAttribute("aria-label")).toBe("Pick a folder");
    expect(dialog?.classList.contains("modal-dialog--large")).toBe(true);
    await settleAnswers();
    await click(buttonWithText(page, "Use media"));
    expect(chosen).toEqual([{ nodeName: "nas", path: "/tank/media" }]);
    expect(page.querySelector("dialog")).toBeNull();
  });

  it("closes with nothing chosen on Cancel, and on Escape only at its places (D361 §3, D539 §4)", async () => {
    const chosen: Place[] = [];
    mounted = await mount(
      withServices(
        <ChooseOpener
          opener="Pick…"
          heading="Pick a folder"
          places={places}
          onChoose={(place) => chosen.push(place)}
        />,
      ),
    );
    const page = mounted.container;
    await click(buttonWithText(page, "Pick…"));
    await click(buttonWithText(page, "Cancel"));
    expect(page.querySelector("dialog")).toBeNull();
    await click(buttonWithText(page, "Pick…"));
    // Escape is the browser's cancel on its modal dialog: inside a place it goes up to the places.
    await fire(page.querySelector("dialog"), new Event("cancel", { cancelable: true }));
    expect(page.querySelector("dialog")).not.toBeNull();
    expect(rowNames(page)).toEqual(["media"]);
    await fire(page.querySelector("dialog"), new Event("cancel", { cancelable: true }));
    expect(page.querySelector("dialog")).toBeNull();
    expect(chosen).toEqual([]);
  });

  it("shows in its side panel the row focus rested on, once it has rested 150 ms (D539 §5)", async () => {
    mounted = await mount(
      withServices(
        <FileBrowser
          use="browse"
          crumbs={[{ key: "here", name: "Films", onOpen: () => undefined }]}
          listLabel="Films"
          shown={{
            kind: "listed",
            groups: [
              {
                key: "here",
                rows: [
                  {
                    key: "a",
                    kind: "video",
                    name: "a.mkv",
                    under: "Arkon",
                    onOpen: () => undefined,
                  },
                ],
              },
            ],
            empty: { kind: "empty", text: "" },
          }}
          side={(rested) => (rested === undefined ? null : <p>{rested.under}</p>)}
        />,
      ),
    );
    const side = mounted.container.querySelector(".file-browser__side");
    mounted.container.querySelector<HTMLElement>(".file-browser__row")?.focus();
    expect(side?.textContent).toBe("");
    await new Promise((resolve) => setTimeout(resolve, sidePanelRestMs + 50));
    await settleAnswers();
    expect(side?.textContent).toBe("Arkon");
  });

  it("goes no higher than the place it began at", () => {
    expect(folderAbove("/tank/media", "/tank/media")).toBeUndefined();
    expect(folderAbove("/tank/media/video/movies", "/tank/media")).toBe("/tank/media/video");
    expect(folderAbove("/media", "/")).toBe("/");
    expect(folderInside("/", "media")).toBe("/media");
    expect(folderInside("/tank/media", "video")).toBe("/tank/media/video");
  });

  it("ticks a folder chosen inside a root in the new-library form, beside the roots (D359 §1)", async () => {
    const drafts: NewLibrary[] = [];
    const services = servedBy({
      "GET /nodes/nas/folders": [
        readable("/tank/media", ["video"]),
        readable("/tank/media/video", []),
      ],
    });
    mounted = await mount(
      withServices(
        <CreateLibraryForm
          draft={{ ...blankLibrary(), name: "Films", nodeName: "nas" }}
          nodes={[{ name: "nas", isConnected: true, rootPaths: ["/tank/media"] }]}
          checkEveryHoursToStart={24}
          problem=""
          onDraftChange={(draft) => drafts.push(draft)}
          onSubmit={() => undefined}
        />,
        services,
      ),
    );
    expect(mounted.container.querySelector("select")).toBeNull();
    await click(buttonWithText(mounted.container, "Choose a folder inside one…"));
    await settleAnswers();
    await click(buttonNamed(mounted.container, "video"));
    await settleAnswers();
    await click(buttonWithText(mounted.container, "Use video"));
    expect(drafts.at(-1)?.rootPaths).toEqual(["/tank/media/video"]);
    expect(foldersOffered(["/tank/media"], ["/tank/media/video"])).toEqual([
      "/tank/media",
      "/tank/media/video",
    ]);
    expect(foldersOffered(["/tank/media"], ["/tank/media"])).toEqual(["/tank/media"]);
  });

  it("starts a library's folders on another machine when one of its folders is ticked", async () => {
    const drafts: NewLibrary[] = [];
    mounted = await mount(
      withServices(
        <CreateLibraryForm
          draft={{ ...blankLibrary(), name: "Films", nodeName: "nas", rootPaths: ["/tank/media"] }}
          nodes={[
            { name: "nas", isConnected: true, rootPaths: ["/tank/media"] },
            { name: "attic", isConnected: true, rootPaths: ["/films"] },
          ]}
          checkEveryHoursToStart={24}
          problem=""
          onDraftChange={(draft) => drafts.push(draft)}
          onSubmit={() => undefined}
        />,
      ),
    );
    const attic = [...mounted.container.querySelectorAll("label")].find((label) =>
      label.textContent.includes("attic"),
    );
    await click(attic?.querySelector("input") ?? null);
    expect(drafts.at(-1)).toMatchObject({ nodeName: "attic", rootPaths: ["/films"] });
  });

  it("adds a folder chosen inside a root to a library (D359 §3)", async () => {
    const added: Place[] = [];
    const services = servedBy({
      "GET /nodes/nas/folders": [
        readable("/tank/media", ["anime"]),
        readable("/tank/media/anime", []),
      ],
    });
    mounted = await mount(
      withServices(<AddRoot places={places} onAdd={(place) => added.push(place)} />, services),
    );
    await click(buttonWithText(mounted.container, "Add a folder…"));
    await settleAnswers();
    await click(buttonNamed(mounted.container, "anime"));
    await settleAnswers();
    await click(buttonWithText(mounted.container, "Use anime"));
    expect(added).toEqual([{ nodeName: "nas", path: "/tank/media/anime" }]);
  });

  it("moves a folder to one chosen inside a root, once checked and confirmed (D429 §2, D359 §3)", async () => {
    const moves: Place[] = [];
    const services = servedBy({
      "GET /nodes/nas/folders": [
        [200, { path: "/tank/movies", state: "missing", folders: [] }],
        readable("/tank/media", ["films"]),
        readable("/tank/media/films", []),
      ],
      "POST /repoints/preview": [[200, cleanReport]],
    });
    mounted = await mount(
      withServices(
        <RepointDialog
          from={{ nodeName: "nas", path: "/tank/movies" }}
          places={[...places, { nodeName: "nas", path: "/tank/movies" }]}
          onRepoint={(to) => moves.push(to)}
          onClose={() => undefined}
        />,
        services,
      ),
    );
    expect(mounted.container.querySelector("select")).toBeNull();
    expect(mounted.container.textContent).toContain("nas: /tank/media");
    await click(buttonWithText(mounted.container, "Choose a folder inside one…"));
    await settleAnswers();
    // It opens at the folder's own place, which has gone and has nothing above it: so to the places.
    expect(mounted.container.textContent).toContain("movies isn't there any more");
    await click(buttonWithText(mounted.container, "Places"));
    await click(buttonNamed(mounted.container, "media"));
    await settleAnswers();
    await click(buttonNamed(mounted.container, "films"));
    await settleAnswers();
    await click(buttonWithText(mounted.container, "Use films"));
    expect(mounted.container.textContent).toContain("nas: /tank/media/films");
    expect(moves).toEqual([]);
    await click(buttonWithText(mounted.container, "Check what moves"));
    await settleAnswers();
    await click(buttonWithText(mounted.container, "Use the new place"));
    expect(moves).toEqual([{ nodeName: "nas", path: "/tank/media/films" }]);
  });
});

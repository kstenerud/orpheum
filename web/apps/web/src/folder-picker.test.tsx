// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { AddRoot } from "./add-root.tsx";
import {
  blankLibrary,
  CreateLibraryForm,
  foldersOffered,
  type NewLibrary,
} from "./create-library.tsx";
import {
  FolderBrowser,
  FolderPicker,
  folderAbove,
  folderInside,
  type Place,
} from "./folder-picker.tsx";
import { RepointDialog } from "./repoint-root.tsx";
import {
  buttonWithText,
  choose,
  click,
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

/** A node's answer: the folders in a folder, readable. */
function readable(path: string, folders: readonly string[]): readonly [number, unknown] {
  return [200, { path, state: "readable", folders }];
}

/** The paths the folders were asked for in, in order. */
function askedPaths(heard: readonly Heard[]): string[] {
  return heard
    .filter((each) => each.methodAndPath === "GET /nodes/nas/folders")
    .map((each) => new URLSearchParams(each.query).get("path") ?? "");
}

const starts: readonly Place[] = [{ nodeName: "nas", path: "/tank/media" }];

describe("choosing a library folder (D359)", () => {
  it("browses from a node's root into the folders beneath it, up as far as the root, and chooses one", async () => {
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
    mounted = await mount(
      withServices(
        <FolderPicker starts={starts} onChoose={(place) => chosen.push(place)} />,
        services,
      ),
    );
    await click(buttonWithText(mounted.container, "Browse nas: /tank/media"));
    await settleAnswers();
    await click(buttonWithText(mounted.container, "video"));
    await settleAnswers();
    expect(mounted.container.textContent).toContain("nas: /tank/media/video");
    await click(buttonWithText(mounted.container, "Up one folder"));
    await settleAnswers();
    await click(buttonWithText(mounted.container, "video"));
    await settleAnswers();
    await click(buttonWithText(mounted.container, "movies"));
    await settleAnswers();
    expect(mounted.container.textContent).toContain("No folders in it.");
    await click(buttonWithText(mounted.container, "Choose this folder"));
    expect(chosen).toEqual([{ nodeName: "nas", path: "/tank/media/video/movies" }]);
    expect(askedPaths(heard)).toEqual([
      "/tank/media",
      "/tank/media/video",
      "/tank/media",
      "/tank/media/video",
      "/tank/media/video/movies",
    ]);
  });

  it("says a folder whose share does not answer is not responding, and asks again on Try again", async () => {
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
    mounted = await mount(
      withServices(<FolderPicker starts={starts} onChoose={() => undefined} />, services),
    );
    await click(buttonWithText(mounted.container, "Browse nas: /tank/media"));
    await settleAnswers();
    expect(mounted.container.textContent).toContain("This folder is not responding");
    await click(buttonWithText(mounted.container, "Try again"));
    await settleAnswers();
    expect(buttonWithText(mounted.container, "video")).toBeDefined();
    expect(askedPaths(heard)).toHaveLength(2);
  });

  it("browses in a modal dialog over the page, which choosing a folder closes (D361 §3)", async () => {
    const chosen: Place[] = [];
    const services = servedBy({
      "GET /nodes/nas/folders": [readable("/tank/media", ["video"])],
    });
    mounted = await mount(
      withServices(
        <FolderBrowser
          opener="Pick…"
          heading="Pick a folder"
          starts={starts}
          onChoose={(place) => chosen.push(place)}
        />,
        services,
      ),
    );
    const page = mounted.container;
    expect(page.querySelector("dialog")).toBeNull();
    expect(buttonWithText(page, "Browse nas: /tank/media")).toBeUndefined();
    await click(buttonWithText(page, "Pick…"));
    const dialog = page.querySelector("dialog");
    expect(dialog?.open).toBe(true);
    expect(dialog?.getAttribute("aria-label")).toBe("Pick a folder");
    await click(buttonWithText(page, "Browse nas: /tank/media"));
    await settleAnswers();
    await click(buttonWithText(page, "Choose this folder"));
    expect(chosen).toEqual([{ nodeName: "nas", path: "/tank/media" }]);
    expect(page.querySelector("dialog")).toBeNull();
  });

  it("closes the folder browser with nothing chosen on Cancel or Escape (D361 §3)", async () => {
    const chosen: Place[] = [];
    mounted = await mount(
      withServices(
        <FolderBrowser
          opener="Pick…"
          heading="Pick a folder"
          starts={starts}
          onChoose={(place) => chosen.push(place)}
        />,
      ),
    );
    const page = mounted.container;
    await click(buttonWithText(page, "Pick…"));
    await click(buttonWithText(page, "Cancel"));
    expect(page.querySelector("dialog")).toBeNull();
    await click(buttonWithText(page, "Pick…"));
    // Escape is the browser's cancel on its modal dialog.
    await fire(page.querySelector("dialog"), new Event("cancel", { cancelable: true }));
    expect(page.querySelector("dialog")).toBeNull();
    expect(chosen).toEqual([]);
  });

  it("goes no higher than the root it began at", () => {
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
    expect(mounted.container.textContent).toContain("Choose a folder inside one…");
    await click(buttonWithText(mounted.container, "Choose a folder inside one…"));
    await click(buttonWithText(mounted.container, "Browse nas: /tank/media"));
    await settleAnswers();
    await click(buttonWithText(mounted.container, "video"));
    await settleAnswers();
    await click(buttonWithText(mounted.container, "Choose this folder"));
    expect(drafts.at(-1)?.rootPaths).toEqual(["/tank/media/video"]);
    expect(foldersOffered(["/tank/media"], ["/tank/media/video"])).toEqual([
      "/tank/media",
      "/tank/media/video",
    ]);
    expect(foldersOffered(["/tank/media"], ["/tank/media"])).toEqual(["/tank/media"]);
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
      withServices(<AddRoot places={starts} onAdd={(place) => added.push(place)} />, services),
    );
    await click(buttonWithText(mounted.container, "Add a folder…"));
    await click(buttonWithText(mounted.container, "Browse nas: /tank/media"));
    await settleAnswers();
    await click(buttonWithText(mounted.container, "anime"));
    await settleAnswers();
    await click(buttonWithText(mounted.container, "Choose this folder"));
    expect(added).toEqual([{ nodeName: "nas", path: "/tank/media/anime" }]);
  });

  it("moves a folder to one chosen inside a root, once confirmed (D241 §6, D359 §3)", async () => {
    const moves: Place[] = [];
    const services = servedBy({
      "GET /nodes/nas/folders": [
        readable("/tank/media", ["films"]),
        readable("/tank/media/films", []),
      ],
    });
    mounted = await mount(
      withServices(
        <RepointDialog
          from={{ nodeName: "nas", path: "/tank/movies" }}
          places={starts}
          onRepoint={(to) => moves.push(to)}
          onClose={() => undefined}
        />,
        services,
      ),
    );
    await click(buttonWithText(mounted.container, "Choose a folder inside one…"));
    await click(buttonWithText(mounted.container, "Browse nas: /tank/media"));
    await settleAnswers();
    await click(buttonWithText(mounted.container, "films"));
    await settleAnswers();
    await click(buttonWithText(mounted.container, "Choose this folder"));
    const options = [...mounted.container.querySelectorAll("option")].map(
      (option) => option.textContent,
    );
    expect(options).toEqual(["nas: /tank/media", "nas: /tank/media/films"]);
    await choose(mounted.container.querySelector("select"), "1");
    expect(moves).toEqual([]);
    await click(buttonWithText(mounted.container, "Use the new place"));
    expect(moves).toEqual([{ nodeName: "nas", path: "/tank/media/films" }]);
  });
});

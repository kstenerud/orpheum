// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { components } from "@orpheum/sdk";
import type { JSX } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  blankLibrary,
  checkEveryHoursToStart,
  CreateLibraryForm,
  hasARoot,
  libraryToCreate,
  onAConnectedNode,
  withChangeDetection,
  withRootChosen,
} from "./create-library.tsx";
import type { Library } from "./libraries.tsx";
import { LibraryManagement, rootAnswerRequest } from "./library-manage.tsx";
import { withServices } from "./test-services.tsx";

type Root = components["schemas"]["RootBody"];

const ignore = (): void => undefined;

describe("libraries", () => {
  it("shows a library's roots, its files by filing, and the admin's answers when removals wait", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <LibraryManagement
          note="Asked 1 roots to scan."
          onScan={ignore}
          onAnswer={ignore}
          onDelete={ignore}
          library={{
            id: 3,
            name: "Films",
            kind: "movie",
            metadataSource: "nfo_then_tmdb",
            changeDetection: "manual",
            isWritable: false,
            createdAt: 1,
            fileCountsByFiling: { listed: 40, errored: 2 },
            roots: [
              {
                nodeName: "local",
                rootPath: "/media/films",
                lastScanOutcome: "withheld",
                withheldReason: "a third of the files went",
                missingMountPaths: ["/media/films/disk1"],
              },
            ],
          }}
        />,
      ),
    );
    expect(markup).toContain("local: /media/films");
    expect(markup).toContain("removals wait for you");
    expect(markup).toContain("Remove what the last scan did not find");
    expect(markup).toContain(
      "Nothing is mounted any more at <code>/media/films/disk1</code> <button",
    );
    expect(markup).toContain("What was mounted here is gone for good");
    expect(markup).toContain("Listed: 40 · Could not be read: 2");
    expect(markup).toContain('href="/libraries/3/files"');
    expect(markup).toContain("<output>Asked 1 roots to scan.</output>");
  });

  it("says whether a watched library's roots are watched, and why not (D272 §9)", () => {
    const library = (changeDetection: "watch" | "manual"): JSX.Element =>
      withServices(
        <LibraryManagement
          note=""
          onScan={ignore}
          onAnswer={ignore}
          onDelete={ignore}
          library={{
            id: 3,
            name: "Films",
            kind: "movie",
            metadataSource: "nfo",
            changeDetection,
            isWritable: false,
            createdAt: 1,
            roots: [
              {
                nodeName: "nas",
                rootPath: "/tank/films",
                lastScanOutcome: "complete",
                missingMountPaths: [],
                watch: { isWatching: true, cause: "" },
              },
              {
                nodeName: "nas",
                rootPath: "/mnt/share",
                lastScanOutcome: "complete",
                missingMountPaths: [],
                watch: { isWatching: false, cause: "network-filesystem", reason: "nfs4" },
              },
            ],
          }}
        />,
      );
    const watched = renderToStaticMarkup(library("watch"));
    expect(watched).toContain("watched for changes");
    expect(watched).toContain("Not watched: it is on a network share or a shared drive");
    expect(watched).toContain("Windows on a WSL drive");
    expect(watched).toContain("Its periodic scan finds what changes.");
    expect(watched).toContain("nfs4");
    const manual = renderToStaticMarkup(library("manual"));
    expect(manual).not.toContain("watched for changes");
    expect(manual).not.toContain("Not watched");
  });

  it("shows a root's scan as it runs, and offers one more after it (D236 §2, §3)", () => {
    const card = (scan: Root["scan"]): string =>
      renderToStaticMarkup(
        withServices(
          <LibraryManagement
            note=""
            onScan={ignore}
            onAnswer={ignore}
            onDelete={ignore}
            library={{
              id: 3,
              name: "Films",
              kind: "movie",
              metadataSource: "nfo_then_tmdb",
              changeDetection: "manual",
              isWritable: false,
              createdAt: 1,
              roots: [
                {
                  nodeName: "local",
                  rootPath: "/media/films",
                  lastScanOutcome: "complete",
                  missingMountPaths: [],
                  ...(scan === undefined ? {} : { scan }),
                },
              ],
            }}
          />,
        ),
      );
    const idle = card(undefined);
    expect(idle).toContain("Scan now");
    expect(idle).not.toContain("Scanning");
    const listing = card({
      isListed: false,
      listedCount: 0,
      toExamineCount: 0,
      examinedCount: 0,
      isAnotherScanQueued: false,
    });
    expect(listing).toContain("Scanning… listing the folders.");
    expect(listing).toContain("Scan again when this one ends");
    const reading = card({
      isListed: true,
      listedCount: 312,
      toExamineCount: 40,
      examinedCount: 12,
      isAnotherScanQueued: true,
    });
    expect(reading).toContain("Scanning… 312 files found, 12 of 40 new or changed read.");
    expect(reading).toContain("It scans again when this one ends.");
  });

  it("offers a rescan of a library of films or of series, never an other one, and shows how far it is (D357)", () => {
    const card = (kind: Library["kind"], rescan?: Library["rescan"]): string =>
      renderToStaticMarkup(
        withServices(
          <LibraryManagement
            note=""
            onScan={ignore}
            onRescan={ignore}
            onAnswer={ignore}
            onDelete={ignore}
            library={{
              id: 3,
              name: "Films",
              kind,
              metadataSource: kind === "other" ? "nfo" : "tmdb",
              changeDetection: "manual",
              isWritable: false,
              createdAt: 1,
              ...(rescan === undefined ? {} : { rescan }),
            }}
          />,
        ),
      );
    expect(card("movie")).toContain(">Rescan</button>");
    expect(card("series")).toContain(">Rescan</button>");
    expect(card("other")).not.toContain(">Rescan</button>");
    expect(card("movie")).not.toContain("Rescanning");
    expect(card("movie", { isScanning: true, itemCount: 0, matchedCount: 0 })).toContain(
      "Rescanning… scanning first.",
    );
    expect(card("movie", { isScanning: false, itemCount: 40, matchedCount: 12 })).toContain(
      "Rescanning… 12 of 40 films and series matched again.",
    );
  });

  it("says a disk is gone by its mount point's path, and approves removals by the root's (D208)", () => {
    const root: Root = {
      nodeName: "local",
      rootPath: "/media/films",
      lastScanOutcome: "complete",
      missingMountPaths: ["/media/films/disk1", "/media/films/disk2"],
    };
    expect(rootAnswerRequest(root, { kind: "diskGone", mountPath: "/media/films/disk2" })).toEqual({
      endpoint: "/libraries/{id}/mounts/gone",
      body: { nodeName: "local", path: "/media/films/disk2" },
    });
    expect(rootAnswerRequest(root, { kind: "approveRemovals" })).toEqual({
      endpoint: "/libraries/{id}/removals",
      body: { nodeName: "local", path: "/media/films" },
    });
    expect(rootAnswerRequest(root, { kind: "retire" })).toEqual({
      endpoint: "/libraries/{id}/roots/retire",
      body: { nodeName: "local", path: "/media/films" },
    });
    const markup = renderToStaticMarkup(
      withServices(
        <LibraryManagement
          note=""
          onScan={ignore}
          onAnswer={ignore}
          onDelete={ignore}
          library={{
            id: 3,
            name: "Films",
            kind: "movie",
            metadataSource: "nfo",
            changeDetection: "manual",
            isWritable: false,
            createdAt: 1,
            roots: [root],
          }}
        />,
      ),
    );
    expect(markup.match(/What was mounted here is gone for good/gu)).toHaveLength(2);
    expect(markup).not.toContain("Remove what the last scan did not find");
  });

  it("asks the three questions with every answer explained, read-only by default (D184)", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <CreateLibraryForm
          draft={{ ...blankLibrary(), rootPaths: ["/media/films"] }}
          nodes={[
            {
              name: "local",
              isConnected: true,
              rootPaths: ["/media/films", "/media/shows"],
            },
            { name: "attic", isConnected: false, rootPaths: [] },
          ]}
          checkEveryHoursToStart={24}
          problem=""
          onDraftChange={ignore}
          onSubmit={ignore}
        />,
      ),
    );
    for (const legend of [
      "Where information comes from",
      "How changes are noticed",
      "What it holds",
    ]) {
      expect(markup).toContain(`<legend>${legend}</legend>`);
    }
    expect(markup).toContain("TMDB only; NFO files are ignored");
    expect(markup).toMatch(/type="checkbox"(?![^>]*checked)/u);
    expect(markup).toContain("attic (not connected)");
    expect(markup).toMatch(/checked=""[^>]*\/><code>\/media\/films<\/code>/u);
    expect(markup).toContain("<code>/media/shows</code>");
    expect(withRootChosen(["/a"], "/b", true)).toEqual(["/a", "/b"]);
    expect(withRootChosen(["/a", "/b"], "/a", false)).toEqual(["/b"]);
  });

  it("starts an interval at the server's new_library_default_scan_every_hours and sends hours only with one", () => {
    const setting = {
      key: "new_library_default_scan_every_hours",
      kind: "integer" as const,
      value: 6,
      startingValue: 24,
      isModified: true,
      isImportant: false,
      section: "libraries" as const,
      description: "",
      setAt: 1,
    };
    expect(checkEveryHoursToStart([setting])).toBe(6);
    expect(() => checkEveryHoursToStart([])).toThrow("new_library_default_scan_every_hours");
    const interval = withChangeDetection(blankLibrary(), "interval", 6);
    expect(interval.checkEveryHours).toBe(6);
    expect(
      withChangeDetection({ ...interval, checkEveryHours: 3 }, "interval", 6).checkEveryHours,
    ).toBe(3);
    expect("checkEveryHours" in withChangeDetection(interval, "watch", 6)).toBe(false);
    const markup = renderToStaticMarkup(
      withServices(
        <CreateLibraryForm
          draft={{ ...blankLibrary(), changeDetection: "interval" }}
          nodes={[]}
          checkEveryHoursToStart={6}
          problem=""
          onDraftChange={ignore}
          onSubmit={ignore}
        />,
      ),
    );
    expect(markup).toMatch(/type="number"[^>]*value="6"/u);
  });

  it("says where its folders come from, and creates nothing until one is ticked", () => {
    const form = (rootPaths: string[]): string =>
      renderToStaticMarkup(
        withServices(
          <CreateLibraryForm
            draft={{ ...blankLibrary(), name: "movies", rootPaths }}
            nodes={[{ name: "local", isConnected: true, rootPaths: ["/media/films"] }]}
            checkEveryHoursToStart={24}
            problem=""
            onDraftChange={ignore}
            onSubmit={ignore}
          />,
        ),
      );
    expect(form([])).toContain("<legend>Folders this library reads</legend>");
    expect(form([])).toContain("to add one, list it there and restart the node.");
    expect(form([])).toMatch(/<button type="submit" disabled="">Create and scan<\/button>/u);
    expect(form(["/media/films"])).toContain('<button type="submit">Create and scan</button>');
    expect(hasARoot(blankLibrary())).toBe(false);
  });

  // ui-audit-2026-11-04.md §3, Create a library: grouped, with Create and scan after the last field.
  it("groups the form as the library, its source and its scanning, with Create and scan after the last", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <CreateLibraryForm
          draft={{ ...blankLibrary(), changeDetection: "interval" }}
          nodes={[{ name: "local", isConnected: true, rootPaths: ["/media/films"] }]}
          checkEveryHoursToStart={24}
          problem=""
          onDraftChange={ignore}
          onSubmit={ignore}
        />,
      ),
    );
    const groups = markup.split('<section class="create-library__group"').slice(1);
    expect(groups.map((group) => /<h2[^>]*>([^<]*)<\/h2>/u.exec(group)?.[1])).toEqual([
      "The library",
      "Its source",
      "Scanning",
    ]);
    const [what = "", source = "", scanning = ""] = groups;
    expect(what).toContain("<legend>What it holds</legend>");
    expect(source).toContain("Machine");
    expect(source).toContain("<legend>Folders this library reads</legend>");
    expect(source).toContain("<legend>Where information comes from</legend>");
    expect(source).toContain("May the server write beside the media");
    expect(scanning).toContain("<legend>How changes are noticed</legend>");
    expect(scanning).toMatch(/Hours between checks<input type="number"/u);
    // Nothing to fill in after the last group's fields but the button.
    expect(scanning.slice(scanning.lastIndexOf("<input"))).toMatch(
      /^<input[^>]*\/><\/label><\/section><button type="submit"[^>]*>Create and scan<\/button><\/form>$/u,
    );
  });

  it("starts on the first connected machine, whatever it is called, and lists its folders (2026-10-06)", () => {
    const nodes = [
      { name: "attic", isConnected: false, rootPaths: [] },
      { name: "sandbox", isConnected: true, rootPaths: ["/home/media"] },
    ];
    const markup = renderToStaticMarkup(
      withServices(
        <CreateLibraryForm
          draft={blankLibrary()}
          nodes={nodes}
          checkEveryHoursToStart={24}
          problem=""
          onDraftChange={ignore}
          onSubmit={ignore}
        />,
      ),
    );
    expect(markup).toContain("<code>/home/media</code>");
    expect(onAConnectedNode(blankLibrary(), nodes).nodeName).toBe("sandbox");
    const onAway = { ...blankLibrary(), nodeName: "attic", rootPaths: ["/old"] };
    expect(onAConnectedNode(onAway, nodes)).toMatchObject({ nodeName: "sandbox", rootPaths: [] });
    const chosen = { ...blankLibrary(), nodeName: "sandbox", rootPaths: ["/home/media"] };
    expect(onAConnectedNode(chosen, nodes)).toBe(chosen);
  });

  it("says a connected machine has not listed its folders yet, rather than show an empty list", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <CreateLibraryForm
          draft={blankLibrary()}
          nodes={[{ name: "sandbox", isConnected: true, rootPaths: [] }]}
          checkEveryHoursToStart={24}
          problem=""
          onDraftChange={ignore}
          onSubmit={ignore}
        />,
      ),
    );
    expect(markup).toContain("sandbox has not said which folders it offers yet.");
  });

  it("says to start a node when none is connected, rather than offer nothing to choose", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <CreateLibraryForm
          draft={blankLibrary()}
          nodes={[{ name: "local", isConnected: false, rootPaths: [] }]}
          checkEveryHoursToStart={24}
          problem=""
          onDraftChange={ignore}
          onSubmit={ignore}
        />,
      ),
    );
    expect(markup).toContain("No node has connected since the server started.");
  });

  it("offers other videos, asked no source, as they are never looked up online (D349)", () => {
    const other = { ...blankLibrary(), kind: "other" as const, rootPaths: ["/media/home"] };
    const markup = renderToStaticMarkup(
      withServices(
        <CreateLibraryForm
          draft={other}
          nodes={[{ name: "local", isConnected: true, rootPaths: ["/media/home"] }]}
          checkEveryHoursToStart={24}
          problem=""
          onDraftChange={ignore}
          onSubmit={ignore}
        />,
      ),
    );
    expect(markup).toContain("Other videos: home movies, concerts, lectures, clips");
    expect(markup).not.toContain("<legend>Where information comes from</legend>");
    expect(markup).toContain("Other videos are never looked up online");
    expect(libraryToCreate(other)).not.toHaveProperty("metadataSource");
    expect(libraryToCreate(blankLibrary()).metadataSource).toBe("nfo_then_tmdb");
  });
});

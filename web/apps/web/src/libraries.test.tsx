// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { components } from "@orpheum/sdk";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  blankLibrary,
  checkEveryHoursToStart,
  CreateLibraryForm,
  hasARoot,
  withChangeDetection,
  withRootChosen,
} from "./create-library.tsx";
import { LibraryCard, rootAnswerRequest } from "./libraries.tsx";
import { withServices } from "./test-services.tsx";

type Root = components["schemas"]["RootBody"];

const ignore = (): void => undefined;

describe("libraries", () => {
  it("shows a library's roots, its files by filing, and the admin's answers when removals wait", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <LibraryCard
          note="Asked 1 roots to scan."
          onScan={ignore}
          onAnswer={ignore}
          onDelete={ignore}
          library={{
            id: 3,
            name: "Films",
            kind: "movies",
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
    expect(markup).toContain("A disk is missing from <code>/media/films/disk1</code> <button");
    expect(markup).toContain("This disk is gone for good");
    expect(markup).toContain("Listed: 40 · Could not be read: 2");
    expect(markup).toContain('href="/libraries/3/files"');
    expect(markup).toContain("<output>Asked 1 roots to scan.</output>");
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
        <LibraryCard
          note=""
          onScan={ignore}
          onAnswer={ignore}
          onDelete={ignore}
          library={{
            id: 3,
            name: "Films",
            kind: "movies",
            metadataSource: "nfo",
            changeDetection: "manual",
            isWritable: false,
            createdAt: 1,
            roots: [root],
          }}
        />,
      ),
    );
    expect(markup.match(/This disk is gone for good/gu)).toHaveLength(2);
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

  it("starts an interval at the server's library_check_every_hours and sends hours only with one", () => {
    const setting = {
      key: "library_check_every_hours",
      kind: "integer" as const,
      value: 6,
      startingValue: 24,
      isModified: true,
      isExposed: false,
      description: "",
      setAt: 1,
    };
    expect(checkEveryHoursToStart([setting])).toBe(6);
    expect(() => checkEveryHoursToStart([])).toThrow("library_check_every_hours");
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
});

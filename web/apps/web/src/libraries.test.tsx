// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { blankLibrary, CreateLibraryForm, toggled } from "./create-library.tsx";
import { LibraryCard } from "./libraries.tsx";
import { withServices } from "./test-services.tsx";

const ignore = (): void => undefined;

describe("libraries", () => {
  it("shows a library's roots, its files by filing, and the admin's answers when removals wait", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <LibraryCard
          note="Asked 1 roots to scan."
          onScan={ignore}
          onApprove={ignore}
          onGone={ignore}
          library={{
            id: 3,
            name: "Films",
            kind: "movies",
            metadataSource: "nfo_then_tmdb",
            changeDetection: "manual",
            isWritable: false,
            createdAt: 1,
            filings: { listed: 40, errored: 2 },
            roots: [
              {
                node: "local",
                path: "/media/films",
                outcome: "withheld",
                withheldReason: "a third of the files went",
              },
            ],
          }}
        />,
      ),
    );
    expect(markup).toContain("local: /media/films");
    expect(markup).toContain("removals wait for you");
    expect(markup).toContain("Remove what the last scan did not find");
    expect(markup).toContain("This disk is gone for good");
    expect(markup).toContain("Listed: 40 · Could not be read: 2");
    expect(markup).toContain('href="/libraries/3/files"');
    expect(markup).toContain("<output>Asked 1 roots to scan.</output>");
  });

  it("asks the three questions with every answer explained, read-only by default (D184)", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <CreateLibraryForm
          draft={{ ...blankLibrary(), roots: ["/media/films"] }}
          nodes={[
            {
              name: "local",
              isConnected: true,
              roots: ["/media/films", "/media/shows"],
            },
            { name: "attic", isConnected: false, roots: [] },
          ]}
          problem=""
          onDraft={ignore}
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
    expect(toggled(["/a"], "/b", true)).toEqual(["/a", "/b"]);
    expect(toggled(["/a", "/b"], "/a", false)).toEqual(["/b"]);
  });

  it("says to start a node when none is connected, rather than offer nothing to choose", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <CreateLibraryForm
          draft={blankLibrary()}
          nodes={[{ name: "local", isConnected: false, roots: [] }]}
          problem=""
          onDraft={ignore}
          onSubmit={ignore}
        />,
      ),
    );
    expect(markup).toContain("No node has connected since the server started.");
  });
});

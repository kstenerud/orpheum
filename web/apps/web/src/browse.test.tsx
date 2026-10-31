// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { ApiProblem } from "./problem.ts";
import { BrowseView, watchedPercentOf, type Entry } from "./browse.tsx";
import { itemRoute } from "./router.ts";
import type { Page } from "./use-pages.ts";
import type { Resource } from "./use-resource.ts";
import { withServices } from "./test-services.tsx";

const ignore = (): void => undefined;

function renderBrowseView(loadedEntries: Resource<Page<Entry>>): string {
  return renderToStaticMarkup(
    withServices(
      <BrowseView
        kind="movie"
        loadedEntries={loadedEntries}
        isLoadingMore={false}
        onLoadMore={ignore}
      />,
    ),
  );
}

describe("browse", () => {
  it("shows each entry's poster, name and year, leading to its page", () => {
    const markup = renderBrowseView({
      state: "loaded",
      value: {
        nextCursor: "cursor",
        items: [
          {
            id: 10,
            name: "Metropolis",
            kind: "movie",
            year: 1995,
            identificationState: "identified",
            addedAt: 1,
            posterSha256: "ab12",
          },
          {
            id: 11,
            name: "Untitled",
            kind: "movie",
            identificationState: "unidentified",
            addedAt: 2,
          },
        ],
      },
    });
    expect(markup).toContain('href="/titles/10"');
    expect(markup).toContain('src="/api/v1/bytes/art/ab12/portrait-m.webp"');
    expect(markup).toContain(
      '<span class="poster-card__name">Metropolis</span><span class="poster-card__detail">1995</span>',
    );
    expect(markup).toContain("No poster");
    expect(markup).toContain('class="end-of-list"');
    expect(markup).not.toContain("Show more");
    expect(markup).not.toContain("This may be identified incorrectly");
  });

  it("shows a film's still in its poster's place while it has no poster (D362 §2)", () => {
    const markup = renderBrowseView({
      state: "loaded",
      value: {
        nextCursor: "",
        items: [
          {
            id: 12,
            name: "Home Reel",
            kind: "movie",
            identificationState: "unidentified",
            addedAt: 1,
            noPosterStillSha256: "5711",
          },
        ],
      },
    });
    expect(markup).toContain(
      '<img class="poster__still" src="/api/v1/bytes/art/5711/landscape-s.webp"',
    );
    expect(markup).toContain("No poster");
  });

  it("puts an uncertain match's cue in its poster's tray, under a warning's (D301 §4)", () => {
    const markup = renderBrowseView({
      state: "loaded",
      value: {
        nextCursor: "",
        items: [
          {
            id: 12,
            name: "Alcazar",
            kind: "movie",
            year: 1992,
            identificationState: "identified",
            isGuessed: true,
            addedAt: 1,
            unavailable: { reason: "node_away", nodeName: "devnas" },
          },
        ],
      },
    });
    const tray = markup.slice(markup.indexOf('poster-card__cues"'));
    expect(tray.indexOf("This cannot be played now")).toBeGreaterThan(0);
    expect(tray.indexOf("This may be identified incorrectly")).toBeGreaterThan(
      tray.indexOf("This cannot be played now"),
    );
  });

  it("says what an empty list means, and a failure's cause", () => {
    const empty = renderBrowseView({ state: "loaded", value: { nextCursor: "", items: [] } });
    expect(empty).toContain("Once a library is scanned");
    expect(empty).not.toContain('class="end-of-list"');
    expect(
      renderBrowseView({
        state: "failed",
        cause: new ApiProblem({ code: "migrating", message: "the server is migrating" }),
      }),
    ).toContain("The server is starting up, or updating. It will answer in a moment.");
    expect(itemRoute("series", 4)).toEqual({ screen: "series-page", collectionId: 4 });
  });

  it("says a server down behind its proxy is not answering, and that the list asks again by itself", () => {
    const shown = renderBrowseView({
      state: "failed",
      cause: new ApiProblem({ code: "not_answering", message: "502" }),
      willAskAgain: true,
    });
    expect(shown).toContain("The server is not answering: it may be restarting");
    expect(shown).toContain("This page tries again by itself.");
    expect(shown).not.toContain("Something went wrong");
  });
});

describe("a title that cannot be played now (D241 §3)", () => {
  it("stays in the list, marked", () => {
    const markup = renderBrowseView({
      state: "loaded",
      value: {
        nextCursor: "",
        items: [
          {
            id: 10,
            name: "Metropolis",
            kind: "movie",
            identificationState: "identified",
            addedAt: 1,
            unavailable: { reason: "disk_missing", nodeName: "devnas", path: "/media/usb" },
          },
        ],
      },
    });
    expect(markup).toContain("Metropolis");
    expect(markup).toContain("Cannot be played now");
    expect(markup).toContain("what was mounted at /media/usb on devnas is missing");
    expect(markup).toContain("poster-card__link--dimmed");
  });
});

describe("how far in a poster's film or series is watched (the owner, 2026-10-25)", () => {
  const base: Entry = {
    id: 1,
    name: "A",
    kind: "movie",
    identificationState: "identified",
    addedAt: 1,
  };

  it("is a film's partway file's percent, and a series' episodes finished of those it has", () => {
    expect(watchedPercentOf({ ...base, watchedPercent: 40 })).toBe(40);
    expect(watchedPercentOf({ ...base, episodeCount: 8, finishedEpisodeCount: 2 })).toBe(25);
  });

  it("is nothing for one not begun, or a series with no episodes", () => {
    expect(watchedPercentOf(base)).toBeUndefined();
    expect(watchedPercentOf({ ...base, episodeCount: 0, finishedEpisodeCount: 0 })).toBeUndefined();
  });
});

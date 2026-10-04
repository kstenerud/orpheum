// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  ContinueWatchingView,
  rowName,
  setItemHidden,
  watchedPercent,
  type EpisodeNaming,
  type ContinueWatchingRow,
} from "./continue-watching.tsx";
import { servedBy, type Heard } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

const nothing = (): void => undefined;

const heat: ContinueWatchingRow = {
  id: 10,
  name: "Metropolis",
  libraryId: 1,
  fileId: 3,
  positionMs: 500_000,
  durationMs: 1_000_000,
  fileName: "Metropolis (1995) - 720p.mkv",
};

describe("Continue Watching", () => {
  it("leads a row that cannot be played now to its page, which says why (D241 §3)", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <ContinueWatchingView
          rows={[{ ...heat, unavailable: { reason: "node_away", nodeName: "devnas" } }]}
          hiddenName={undefined}
          onHide={nothing}
          onUndo={nothing}
        />,
      ),
    );
    expect(markup).not.toContain('href="/play/3"');
    expect(markup).toContain(`href="/titles/${String(heat.id)}"`);
    expect(markup).toContain("Cannot be played now");
  });

  it("plays each work's file at once, says how far along, which copy, and offers to hide it", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <ContinueWatchingView
          rows={[heat]}
          hiddenName={undefined}
          onHide={nothing}
          onUndo={nothing}
        />,
      ),
    );
    expect(markup).toContain('href="/play/3"');
    expect(markup).toContain("Metropolis (1995) - 720p.mkv");
    expect(markup).toContain('value="50"');
    expect(markup).toContain('aria-label="Hide Metropolis from Continue watching"');
  });

  it("offers to undo a hide, and shows nothing when there is nothing", () => {
    const hidden = renderToStaticMarkup(
      withServices(
        <ContinueWatchingView rows={[]} hiddenName="Metropolis" onHide={nothing} onUndo={nothing} />,
      ),
    );
    expect(hidden).toContain("Metropolis is hidden until you watch it again.");
    expect(hidden).toContain("Undo");
    const empty = renderToStaticMarkup(
      withServices(
        <ContinueWatchingView rows={[]} hiddenName={undefined} onHide={nothing} onUndo={nothing} />,
      ),
    );
    expect(empty).toBe("");
  });

  it("names an episode by its series and place, and a film by its name", () => {
    const formatEpisodeName = (place: EpisodeNaming): string =>
      `${place.seriesName} ${String(place.seasonNumber)}x${String(place.episodeNumber)} ${place.episodeName}`;
    expect(rowName(heat, formatEpisodeName)).toBe("Metropolis");
    expect(
      rowName(
        {
          name: "Pilot",
          episode: { seriesId: 20, seriesName: "Brass Arena", seasonNumber: 1, episodeNumber: 1 },
        },
        formatEpisodeName,
      ),
    ).toBe("Brass Arena 1x1 Pilot");
  });

  it("gives no percentage for a file of unknown length", () => {
    expect(watchedPercent({ positionMs: 60_000, durationMs: 0 })).toBeUndefined();
    expect(watchedPercent({ positionMs: 250, durationMs: 1000 })).toBe(25);
  });

  it("hides with POST and undoes with DELETE", async () => {
    const heard: Heard[] = [];
    const { api } = servedBy(
      { "POST /items/10/hide": [204, undefined], "DELETE /items/10/hide": [204, undefined] },
      heard,
    );
    await setItemHidden(api, { itemId: 10, isHidden: true }, "click-1");
    await setItemHidden(api, { itemId: 10, isHidden: false }, "click-2");
    expect(heard.map((request) => request.methodAndPath)).toEqual([
      "POST /items/10/hide",
      "DELETE /items/10/hide",
    ]);
  });
});

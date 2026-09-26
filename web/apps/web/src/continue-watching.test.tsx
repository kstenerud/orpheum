// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { createApi } from "@orpheum/sdk";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import {
  ContinueWatchingView,
  rowName,
  changeHidden,
  watchedPercent,
  type EpisodeNaming,
  type InProgress,
} from "./continue-watching.tsx";
import { withServices } from "./test-services.tsx";

const nothing = (): void => undefined;

const heat: InProgress = {
  id: 10,
  name: "Metropolis",
  libraryId: 1,
  fileId: 3,
  positionMs: 500_000,
  durationMs: 1_000_000,
  fileName: "Metropolis (1995) - 720p.mkv",
};

describe("Continue Watching", () => {
  it("plays each work's file at once, says how far along, which copy, and offers to hide it", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <ContinueWatchingView rows={[heat]} hidden={undefined} onHide={nothing} onUndo={nothing} />,
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
        <ContinueWatchingView rows={[]} hidden="Metropolis" onHide={nothing} onUndo={nothing} />,
      ),
    );
    expect(hidden).toContain("Metropolis is hidden until you watch it again.");
    expect(hidden).toContain("Undo");
    const empty = renderToStaticMarkup(
      withServices(
        <ContinueWatchingView rows={[]} hidden={undefined} onHide={nothing} onUndo={nothing} />,
      ),
    );
    expect(empty).toBe("");
  });

  it("names an episode by its series and place, and a film by its name", () => {
    const episodeName = (place: EpisodeNaming): string =>
      `${place.series} ${String(place.season)}x${String(place.number)} ${place.episode}`;
    expect(rowName(heat, episodeName)).toBe("Metropolis");
    expect(
      rowName(
        {
          name: "Pilot",
          episode: { seriesId: 20, seriesName: "Brass Arena", season: 1, number: 1 },
        },
        episodeName,
      ),
    ).toBe("Brass Arena 1x1 Pilot");
  });

  it("gives no percentage for a file of unknown length", () => {
    expect(watchedPercent({ positionMs: 60_000, durationMs: 0 })).toBeUndefined();
    expect(watchedPercent({ positionMs: 250, durationMs: 1000 })).toBe(25);
  });

  it("hides with POST and undoes with DELETE", async () => {
    const sent: string[] = [];
    const api = createApi({ origin: "http://orpheum.test", token: () => "token" });
    // Answered here, before any request leaves: tests never touch the network.
    api.use({
      onRequest({ request }) {
        sent.push(`${request.method} ${request.url}`);
        return new Response(null, { status: 204 });
      },
    });
    await changeHidden(api, 10, false);
    await changeHidden(api, 10, true);
    expect(sent).toEqual([
      "POST http://orpheum.test/api/v1/items/10/hide",
      "DELETE http://orpheum.test/api/v1/items/10/hide",
    ]);
  });
});

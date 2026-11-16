// SPDX-License-Identifier: MPL-2.0

/**
 * An other video's tile as a real browser lays it out (D349 §9.1): no poster shape fits it, so it
 * takes a 16:9 tile instead, in the poster's own place in the card.
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { LibraryBrowse } from "./library-browse.tsx";
import { PosterCard } from "./poster-card.tsx";
import { BackgroundChangesContext, type Poller } from "./polling.ts";
import { mount, servedBy, settleUntil, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A poller that never asks: the page's only reads are the ones the test answers. */
const neverAsks: Poller = { start: () => () => undefined };

describe("an other video's tile, laid out by a real browser", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("takes a 16:9 tile, wider than a poster is tall, where a poster would otherwise sit", async () => {
    const services = servedBy({
      "GET /libraries": [200, { libraries: [{ id: 5, name: "Home movies", kind: "other" }] }],
      "GET /items": [
        200,
        {
          items: [
            {
              id: 90,
              name: "Beach day",
              kind: "other",
              identificationState: "identified",
              libraryId: 5,
              addedAt: 1,
              recorded: { recordedAt: 1_700_000_000_000, from: "tag", fileName: "beach.mp4" },
            },
          ],
          nextCursor: "",
        },
      ],
    });
    mounted = await mount(
      <BackgroundChangesContext value={neverAsks}>
        {withServices(
          <LibraryBrowse
            library={5}
            view={undefined}
            folder={undefined}
            sort={undefined}
            query=""
          />,
          services,
        )}
      </BackgroundChangesContext>,
    );
    await settleUntil(() => mounted?.container.querySelector(".poster-card__tile") !== null);
    const tile = mounted.container.querySelector(".poster-card__tile");
    if (tile === null) {
      throw new Error("the card has no .poster-card__tile");
    }
    expect(getComputedStyle(tile).aspectRatio.replace(/\s/g, "")).toBe("16/9");
    const box = tile.getBoundingClientRect();
    expect(box.width).toBeGreaterThan(0);
    // Wider than tall, as a poster, built for a portrait shape, never is.
    expect(box.width / box.height).toBeGreaterThan(1.5);
    expect(mounted.container.querySelector(".poster")).toBeNull();
  });
});

/** An other video in library 5, the poster's sha given where it has one. */
function otherVideo(id: number, posterSha256?: string): Record<string, unknown> {
  return {
    id,
    name: `Clip ${String(id)}`,
    kind: "other",
    identificationState: "identified",
    libraryId: 5,
    addedAt: 1,
    ...(posterSha256 === undefined ? {} : { posterSha256 }),
    recorded: { recordedAt: 1_700_000_000_000, from: "tag", fileName: `clip-${String(id)}.mp4` },
  };
}

/** The other library's grid, holding items, drawn at 960 pixels wide. */
async function otherGrid(items: readonly Record<string, unknown>[]): Promise<Mounted> {
  const services = servedBy({
    "GET /libraries": [200, { libraries: [{ id: 5, name: "Home movies", kind: "other" }] }],
    "GET /items": [200, { items, nextCursor: "" }],
  });
  const mounted = await mount(
    <BackgroundChangesContext value={neverAsks}>
      <div style={{ width: "960px" }}>
        {withServices(
          <LibraryBrowse
            library={5}
            view={undefined}
            folder={undefined}
            sort={undefined}
            query=""
          />,
          services,
        )}
      </div>
    </BackgroundChangesContext>,
  );
  await settleUntil(
    () => mounted.container.querySelectorAll(".poster-grid .poster-card").length === items.length,
  );
  return mounted;
}

/** The box of the selector's first match within the card at place, by its place in the grid. */
function boxIn(mounted: Mounted, place: number, selector: string): DOMRect {
  const card = mounted.container.querySelectorAll(".poster-grid .poster-card")[place];
  const found = card?.querySelector(selector);
  if (found === null || found === undefined) {
    throw new Error(`card ${String(place)} has no ${selector}`);
  }
  return found.getBoundingClientRect();
}

describe("an other video's poster and the stills beside it (D495)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("shows a poster upright, and centres each still of its row in a poster's height", async () => {
    mounted = await otherGrid([otherVideo(1), otherVideo(2, "ab".repeat(32)), otherVideo(3)]);
    const poster = boxIn(mounted, 1, ".poster");
    // Upright, as a film's poster is.
    expect(poster.height).toBeGreaterThan(poster.width);
    for (const place of [0, 2]) {
      const art = boxIn(mounted, place, ".poster-card__art");
      const tile = boxIn(mounted, place, ".poster-card__tile");
      expect(art.height).toBeCloseTo(poster.height, 0);
      // The still's middle is the poster's middle.
      expect(tile.top + tile.height / 2).toBeCloseTo(poster.top + poster.height / 2, 0);
      // So the names under the row line up.
      expect(boxIn(mounted, place, ".poster-card__name").top).toBeCloseTo(
        boxIn(mounted, 1, ".poster-card__name").top,
        0,
      );
    }
  });

  it("keeps a row of stills alone as tall as its stills", async () => {
    mounted = await otherGrid([otherVideo(1), otherVideo(2), otherVideo(3)]);
    const art = boxIn(mounted, 0, ".poster-card__art");
    const tile = boxIn(mounted, 0, ".poster-card__tile");
    expect(art.height).toBeCloseTo(tile.height, 0);
    expect(art.width / art.height).toBeGreaterThan(1.5);
  });
});

describe("a poster card's status cues, over its poster (the owner, 2026-11-11)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("let a click on them through to the card's link, so no part of the poster is dead", async () => {
    mounted = await mount(
      withServices(
        <ul style={{ width: "12rem" }}>
          <PosterCard
            to={{ screen: "home" }}
            posterSha256={undefined}
            name="Metropolis"
            cues={
              <span className="poster-card__cues">
                <span className="poster-cue">!</span>
              </span>
            }
          />
        </ul>,
        servedBy({}),
      ),
    );
    const cue =
      mounted.container.querySelector(".poster-cue")?.getBoundingClientRect() ?? new DOMRect();
    expect(cue.width).toBeGreaterThan(0);
    const under = document.elementFromPoint(cue.left + cue.width / 2, cue.top + cue.height / 2);
    expect(under?.closest("a.poster-card__link")).not.toBeNull();
  });
});

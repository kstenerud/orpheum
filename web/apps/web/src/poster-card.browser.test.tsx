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
      "GET /titles": [
        200,
        {
          titles: [
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
      "GET /titles/letters": [200, { letters: [] }],
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

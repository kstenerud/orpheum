// SPDX-License-Identifier: MPL-2.0

/**
 * A season's episodes as a real browser lays them out (series.css): the watched check over an
 * episode's still, as a poster's tray draws it over a poster (D370 §2).
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { describe, expect, it } from "vitest";

import { SeasonView } from "./series.tsx";
import { mount } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A box, or a failure where the element is not drawn. */
function boxOf(element: Element | null | undefined): DOMRect {
  if (element === null || element === undefined) {
    throw new Error("an element the test measures is not drawn");
  }
  return element.getBoundingClientRect();
}

describe("a season's episodes, laid out by a real browser", () => {
  // The owner, 2026-10-28: "I think the checkmark should be over the screenshot".
  it("draws a watched episode's check inside its still's top-right corner, not beside the row", async () => {
    const mounted = await mount(
      withServices(
        <SeasonView
          loadedEpisodes={{
            state: "loaded",
            value: {
              items: [
                {
                  id: 31,
                  seasonNumber: 1,
                  episodeNumber: 1,
                  name: "Episode 1",
                  identificationState: "identified",
                  hasLiveFile: true,
                  isWatched: true,
                },
              ],
              nextCursor: "",
            },
          }}
          isLoadingMore={false}
          onLoadMore={() => undefined}
        />,
      ),
    );
    const still = boxOf(mounted.container.querySelector(".series__still"));
    const mark = boxOf(mounted.container.querySelector(".watched-row-mark"));
    expect(still.width).toBeGreaterThan(0);
    expect(mark.left).toBeGreaterThanOrEqual(still.left + still.width / 2);
    expect(mark.right).toBeLessThanOrEqual(still.right);
    // At its top, a little in from its edges, as a poster's tray is (poster-card.css).
    expect(mark.top).toBeGreaterThanOrEqual(still.top);
    expect(mark.top).toBeLessThanOrEqual(still.top + 8);
    expect(still.right - mark.right).toBeLessThanOrEqual(8);
    expect(mark.bottom).toBeLessThanOrEqual(still.bottom);
    mounted.unmount();
  });
});

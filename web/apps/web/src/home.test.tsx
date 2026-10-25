// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { Home } from "./home.tsx";
import { mount, servedBy, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A film or series as a list sends it. */
function entry(id: number, name: string): Record<string, unknown> {
  return { id, name, identificationState: "identified", libraryId: 1, addedAt: id };
}

describe("home (the owner, 2026-10-25)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("shows what is partway, then the newest films and series, every poster alike", async () => {
    const services = servedBy(
      {
        "GET /home": [
          200,
          {
            continueWatching: [
              { id: 7, name: "Metropolis", libraryId: 1, fileId: 70, positionMs: 250, durationMs: 1000 },
            ],
          },
        ],
        "GET /titles": [
          200,
          {
            titles: [{ ...entry(7, "Metropolis"), watchedPercent: 25 }, entry(8, "Nosferatu")],
            nextCursor: "",
          },
        ],
      },
      [],
      "token",
    );
    mounted = await mount(withServices(<Home />, services));
    const headings = [...mounted.container.querySelectorAll("h2")].map((each) => each.textContent);
    expect(headings).toEqual(["Continue watching", "New films", "New series"]);
    const newFilms = mounted.container.querySelector('[aria-labelledby="home-newest-movie"]');
    const cards = newFilms?.querySelectorAll(".poster-card") ?? [];
    expect(cards).toHaveLength(2);
    // A film partway shows how far in, in every list, as the server says it.
    expect(cards[0]?.querySelector("progress")?.getAttribute("value")).toBe("25");
    expect(cards[1]?.querySelector("progress")).toBeNull();
  });

  it("leaves out a row with nothing in it", async () => {
    const services = servedBy(
      {
        "GET /home": [200, { continueWatching: [] }],
        "GET /titles": [200, { titles: [], nextCursor: "" }],
      },
      [],
      "token",
    );
    mounted = await mount(withServices(<Home />, services));
    expect(mounted.container.querySelectorAll("h2")).toHaveLength(0);
    expect(mounted.container.querySelector("h1")?.textContent).toBe("Home");
  });
});

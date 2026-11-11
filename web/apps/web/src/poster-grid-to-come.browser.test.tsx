// SPDX-License-Identifier: MPL-2.0

/**
 * A grid's blank posters for the entries still to come, as a real browser lays them out: Down from
 * the last row of those come moves on into them at once, and one focused as its entry comes hands
 * focus to that entry (the owner, 2026-11-11: placeholders from the list's total).
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { act, useEffect, useState, type JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";

import { PosterGrid, type Entry } from "./browse.tsx";
import { geometricFocus, nextInContainers } from "./focus-engine.ts";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

function filmsFrom(first: number, count: number): Entry[] {
  return Array.from({ length: count }, (_, offset) => ({
    addedAt: 0,
    id: first + offset,
    identificationState: "identified",
    kind: "film",
    name: `Film ${String(first + offset)}`,
    year: 2000,
  }));
}

/** Sets the grid's entries and how many are to come, from the test. */
const control: { show?: (entries: Entry[], toCome: number) => void } = {};

function Grid(props: { readonly entries: Entry[]; readonly toCome: number }): JSX.Element {
  const [shown, setShown] = useState(props);
  useEffect(() => {
    control.show = (entries, toCome) => {
      setShown({ entries, toCome });
    };
  }, []);
  return <PosterGrid kind="film" entries={shown.entries} toCome={shown.toCome} />;
}

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** The grid's focusable cards, in its order: its entries' links, then its blanks. */
function cards(): HTMLElement[] {
  return [...document.querySelectorAll<HTMLElement>(".poster-grid > li > .poster-card__link")];
}

describe("a grid's posters still to come", () => {
  it("takes Down from the last row come into the blanks, and keeps focus on the place as its entry comes", async () => {
    await page.viewport(1280, 900);
    mounted = await mount(withServices(<Grid entries={filmsFrom(1, 6)} toCome={12} />));
    expect(cards()).toHaveLength(18);
    const across = cards().filter(
      (card) => card.getBoundingClientRect().top === cards()[0]?.getBoundingClientRect().top,
    ).length;
    const last = cards()[5];
    if (last === undefined) {
      throw new Error("no last card");
    }
    last.focus();
    const below = nextInContainers(last, "down", { root: document.body, engine: geometricFocus });
    const place = cards().indexOf(below ?? last);
    // The blank below the last entry: Down moves on at once, with nothing loading to wait for.
    expect(place).toBe(5 + across);
    expect(below?.closest("li")?.classList.contains("poster-card--to-come")).toBe(true);
    below?.focus();
    await act(async () => {
      control.show?.(filmsFrom(1, 18), 0);
      await Promise.resolve();
    });
    expect(cards()).toHaveLength(18);
    expect(document.activeElement).toBe(cards()[place]);
    expect(document.activeElement?.textContent).toContain(`Film ${String(place + 1)}`);
  });

  it("asks for the next page as the first blank nears the view, and not while one loads", async () => {
    const asked = vi.fn();
    mounted = await mount(
      withServices(
        <PosterGrid
          kind="film"
          entries={filmsFrom(1, 6)}
          toCome={12}
          isLoadingMore
          onNearEnd={asked}
        />,
      ),
    );
    await new Promise((resolve) => requestAnimationFrame(resolve));
    expect(asked).not.toHaveBeenCalled();
    mounted.unmount();
    mounted = await mount(
      withServices(
        <PosterGrid kind="film" entries={filmsFrom(1, 6)} toCome={12} onNearEnd={asked} />,
      ),
    );
    await vi.waitFor(() => {
      expect(asked).toHaveBeenCalled();
    });
  });
});

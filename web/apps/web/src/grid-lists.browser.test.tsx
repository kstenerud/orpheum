// SPDX-License-Identifier: MPL-2.0

/**
 * The three lists that moved onto the grid list (8c0f3ab1; the owner, 2026-11-05: "a grid UI widget
 * for alignment"), as a real browser lays them out: a title's cast, needs a match and the waiting
 * invitations. Each is drawn with rows whose earlier cells differ in width, and each column must
 * start at the same x in every row; the cast, which flows several entries to a line, gives every entry
 * one width. The widget alone is tested in grid-list.browser.test.tsx; these hold that each list
 * uses it.
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { page } from "vitest/browser";

import { TitlePeople, type Credits } from "./credits.tsx";
import { WaitingList } from "./identify.tsx";
import { InvitationsSection } from "./invitations-section.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** How far layout's rounding may put a box: a subpixel, never a cell's width. */
const roundingPx = 1;

let mounted: Mounted | undefined;

// A laptop's window, wide enough for every list drawn here.
beforeEach(async () => {
  await page.viewport(1280, 800);
});

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

const ignore = (): void => undefined;

/** A list drawn in a box of its own width, under the services every page has. */
async function mountIn(width: string, list: ReactNode): Promise<void> {
  mounted = await mount(withServices(<div style={{ inlineSize: width }}>{list}</div>));
}

/** Each drawn row's cells, in the columns' order. */
function cellsByRow(): HTMLElement[][] {
  return [...document.querySelectorAll(".grid-list__row")].map((row) => [
    ...row.querySelectorAll<HTMLElement>(".grid-list__cell"),
  ]);
}

/** The boxes of the text nodes that start with start, in document order. */
function textBoxes(start: string): DOMRect[] {
  const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  const boxes: DOMRect[] = [];
  for (let node = walker.nextNode(); node !== null; node = walker.nextNode()) {
    if (node.textContent?.trim().startsWith(start) === true) {
      const range = document.createRange();
      range.selectNodeContents(node);
      boxes.push(range.getBoundingClientRect());
    }
  }
  return boxes;
}

/** The left edges of the text that starts with start, one a row. */
function leftsOf(start: string, rows: number): number[] {
  const boxes = textBoxes(start);
  expect(boxes).toHaveLength(rows);
  return boxes.map((box) => box.left);
}

/** Expects every number to equal the first, within layout's rounding. */
function expectAligned(values: readonly number[]): void {
  expect(values.length).toBeGreaterThanOrEqual(2);
  for (const value of values) {
    expect(Math.abs(value - (values[0] ?? Number.NaN))).toBeLessThanOrEqual(roundingPx);
  }
}

/** Expects the names' text to differ in width, so alignment is not by chance. */
function expectUnlike(names: readonly string[]): void {
  const widths = names.flatMap((name) => textBoxes(name).map((box) => box.width));
  expect(widths).toHaveLength(names.length);
  expect(Math.max(...widths) - Math.min(...widths)).toBeGreaterThan(20);
}

describe("a title's cast, directors and writers", () => {
  const credits: Credits = {
    cast: [
      { personId: 1, name: "Al", character: "Vin" },
      { personId: 2, name: "Bernadette Featherstonehaugh", character: "The Long Part" },
      { personId: 3, name: "Cy Young" },
    ],
    directors: [],
    writers: [],
  };

  it("gives every entry one width, so those flowing along a line line up", async () => {
    await mountIn("48rem", <TitlePeople credits={credits} />);
    // Entries flow several to a line, each as wide as the others.
    const entries = [...document.querySelectorAll<HTMLElement>(".credit")];
    expect(entries).toHaveLength(3);
    expectAligned(entries.map((entry) => entry.getBoundingClientRect().width));
    expectAligned(entries.slice(0, 2).map((entry) => entry.getBoundingClientRect().top));
    const rows = [...document.querySelectorAll<HTMLElement>(".grid-list__row")];
    expect(rows).toHaveLength(3);
    expectAligned(rows.map((row) => row.getBoundingClientRect().width));
    // Each entry's name starts the same distance in from its own left edge.
    expectAligned(
      cellsByRow().map(
        (cells, index) =>
          (cells[1]?.getBoundingClientRect().left ?? 0) -
          (rows[index]?.getBoundingClientRect().left ?? 0),
      ),
    );
  });
});

describe("needs a match", () => {
  it("starts the year and the actions at one x in every row, whatever the names' lengths", async () => {
    const longName = "The Extraordinarily Long Name Of A Series";
    await mountIn(
      "60rem",
      <WaitingList
        waitingFilter="ambiguous"
        onChanged={ignore}
        items={[
          { id: 1, kind: "movie", name: "It", identificationState: "ambiguous", year: 1990 },
          { id: 2, kind: "series", name: longName, identificationState: "ambiguous", year: 2011 },
          { id: 3, kind: "movie", name: "Metropolis Two", identificationState: "ambiguous", year: 1995 },
        ]}
      />,
    );
    expectUnlike(["It", longName, "Metropolis Two"]);
    expectAligned(["1990", "2011", "1995"].map((year) => leftsOf(year, 1)[0] ?? Number.NaN));
    expectAligned(leftsOf("Choose its match", 3));
    expectAligned(leftsOf("Look again", 3));
  });
});

describe("the waiting invitations", () => {
  it("starts the role and the date issued at one x in every row, whatever the notes' lengths", async () => {
    const invitation = (id: number, adminNote: string, role: string) => ({
      id,
      adminNote,
      role,
      issuedAt: 1_700_000_000_000 + (id - 1) * 100_000_000,
      expiresAt: 0,
      liveLinks: 0,
      links: null,
    });
    const longNote = "A flatmate who watches on the television";
    await mountIn(
      "60rem",
      <InvitationsSection
        onOpen={ignore}
        invitations={[
          invitation(1, "Mum", "admin"),
          invitation(2, longNote, "guest"),
          invitation(3, "Dan", "guest"),
        ]}
      />,
    );
    expectUnlike(["Mum", longNote, "Dan"]);
    // Administrator and guest are not alike in width, so only the column's start can align them.
    expectAligned([...leftsOf("Administrator", 1), ...leftsOf("guest", 2)]);
    expectAligned(leftsOf("Issued", 3));
  });
});

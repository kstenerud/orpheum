// SPDX-License-Identifier: MPL-2.0

/**
 * The grid list as a real browser lays it out (grid-list.tsx, grid-list.css; the owner, 2026-11-05:
 * "a grid UI widget for alignment, which supports some columns wrapping and some not"): its columns
 * line up in every row, a column that stays on one line does, one that wraps wraps, and each row's
 * pressable target covers exactly what it draws.
 */

import "./tokens.css";
import "./base.css";

import type { JSX } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { page } from "vitest/browser";

import { GridList, type GridColumn, type GridTargetDrawn } from "./grid-list.tsx";
import { mount, type Mounted } from "./test-dom.tsx";

/** How far layout's rounding may put a box. */
const roundingPx = 1;

let mounted: Mounted | undefined;

// A laptop's window, wide enough for every list drawn here: a press off the page lands nowhere.
beforeEach(async () => {
  await page.viewport(1280, 800);
});

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

interface Person {
  readonly name: string;
  readonly note: string;
  readonly role: string;
}

const longNote = "Watches mostly on the television in the front room, and asks for subtitles.";

/** Rows of unlike lengths, so a column laid out per row would not line up. */
const people: readonly Person[] = [
  { name: "Al", note: "", role: "Administrator" },
  { name: "Bernadette Featherstonehaugh-Montgomery", note: longNote, role: "Can watch" },
  { name: "Cy", note: "Kids", role: "Guest" },
];

const personColumns: readonly GridColumn<Person>[] = [
  { key: "name", text: "truncate", cell: (person) => person.name },
  { key: "note", text: "wrap", cell: (person) => person.note },
  { key: "role", text: "fit", cell: (person) => person.role },
];

/** A row's target as a button, as the roles list draws it. */
function asButton(_person: Person, drawn: GridTargetDrawn): JSX.Element {
  return <button type="button" {...drawn} />;
}

/** A row's target as a link, as the accounts list draws it. */
function asLink(person: Person, drawn: GridTargetDrawn): JSX.Element {
  return (
    <a href={`#${person.name}`} className={drawn.className}>
      {drawn.children}
    </a>
  );
}

/** The people's list, in a box narrower than its longest row's one line. */
async function mountPeople(
  target: (person: Person, drawn: GridTargetDrawn) => JSX.Element,
  aside?: (person: Person) => JSX.Element | null,
): Promise<void> {
  mounted = await mount(
    <div style={{ inlineSize: "36rem" }}>
      <GridList
        rows={people}
        rowKey={(person) => person.name}
        columns={personColumns}
        target={target}
        {...(aside === undefined ? {} : { aside })}
      />
    </div>,
  );
}

/** Each drawn row's cells, in the columns' order. */
function cellsByRow(): HTMLElement[][] {
  return [...document.querySelectorAll(".grid-list__row")].map((row) => [
    ...row.querySelectorAll<HTMLElement>(".grid-list__cell"),
  ]);
}

/** Each column's left edges, one a row. */
function leftEdgesByColumn(rows: readonly (readonly HTMLElement[])[]): number[][] {
  const count = rows[0]?.length ?? 0;
  return Array.from({ length: count }, (_, column) =>
    rows.map((cells) => cells[column]?.getBoundingClientRect().left ?? Number.NaN),
  );
}

/** How many lines an element's text takes, by its height over its line's. */
function linesOf(element: HTMLElement): number {
  const lineHeight = Number.parseFloat(getComputedStyle(element).lineHeight);
  return Math.round(element.getBoundingClientRect().height / lineHeight);
}

/** Every target's points pressed (its centre and each corner, just inside) that land elsewhere. */
function missedPresses(): string[] {
  const missed: string[] = [];
  for (const target of document.querySelectorAll<HTMLElement>(".grid-list__target")) {
    const box = target.getBoundingClientRect();
    const inset = 2;
    const points = [
      [box.left + box.width / 2, box.top + box.height / 2],
      [box.left + inset, box.top + inset],
      [box.right - inset, box.top + inset],
      [box.left + inset, box.bottom - inset],
      [box.right - inset, box.bottom - inset],
    ] as const;
    for (const [x, y] of points) {
      const hit = document.elementFromPoint(x, y);
      if (hit === null || !target.contains(hit)) {
        missed.push(`${target.textContent} at ${String(Math.round(x))},${String(Math.round(y))}`);
      }
    }
  }
  return missed;
}

/** Every cell that reaches outside its row's target. */
function cellsOutsideTargets(): string[] {
  return [...document.querySelectorAll<HTMLElement>(".grid-list__target")].flatMap((target) => {
    const box = target.getBoundingClientRect();
    return [...target.querySelectorAll<HTMLElement>(".grid-list__cell")]
      .filter((cell) => {
        const drawn = cell.getBoundingClientRect();
        return (
          drawn.left < box.left - roundingPx ||
          drawn.right > box.right + roundingPx ||
          drawn.top < box.top - roundingPx ||
          drawn.bottom > box.bottom + roundingPx
        );
      })
      .map((cell) => cell.textContent);
  });
}

describe("a grid list", () => {
  for (const [kind, target] of [
    ["button", asButton],
    ["link", asLink],
  ] as const) {
    it(`lines its columns up in every row, its targets ${kind}s`, async () => {
      await mountPeople(target);
      const rows = cellsByRow();
      expect(rows).toHaveLength(3);
      for (const edges of leftEdgesByColumn(rows)) {
        const first = edges[0] ?? Number.NaN;
        for (const edge of edges) {
          expect(Math.abs(edge - first)).toBeLessThanOrEqual(roundingPx);
        }
      }
    });
  }

  it("keeps a truncated and a fitted column on one line and wraps a wrapping one", async () => {
    await mountPeople(asButton);
    const [name, note, role] = cellsByRow()[1] ?? [];
    if (name === undefined || note === undefined || role === undefined) {
      throw new Error("the long row is not drawn");
    }
    expect(linesOf(name)).toBe(1);
    // Cut, not wrapped: more text than shows.
    expect(name.scrollWidth).toBeGreaterThan(name.clientWidth);
    expect(linesOf(note)).toBeGreaterThan(1);
    expect(linesOf(role)).toBe(1);
    // Sized to its widest cell: every role whole, the widest filling the column.
    const roles = cellsByRow().map((cells) => cells[2]);
    for (const each of roles) {
      expect(each?.scrollWidth).toBeLessThanOrEqual((each?.clientWidth ?? 0) + roundingPx);
    }
  });

  for (const [kind, target] of [
    ["button", asButton],
    ["link", asLink],
  ] as const) {
    it(`takes a press anywhere on a row's ${kind} and holds every cell inside it`, async () => {
      await mountPeople(target);
      expect(missedPresses()).toEqual([]);
      expect(cellsOutsideTargets()).toEqual([]);
    });
  }

  it("draws an aside beside the target, outside it, in a column of its own", async () => {
    await mountPeople(asButton, (person) =>
      person.role === "Guest" ? <button type="button">!</button> : null,
    );
    const target = document.querySelectorAll<HTMLElement>(".grid-list__target")[2];
    const aside = document.querySelectorAll<HTMLElement>(".grid-list__aside")[2];
    if (target === undefined || aside === undefined) {
      throw new Error("the guest's row is not drawn");
    }
    expect(target.contains(aside)).toBe(false);
    expect(aside.getBoundingClientRect().left).toBeGreaterThanOrEqual(
      target.getBoundingClientRect().right,
    );
    // The other rows' targets end where the guest's does: the aside's column is every row's.
    for (const each of document.querySelectorAll(".grid-list__target")) {
      expect(
        Math.abs(each.getBoundingClientRect().right - target.getBoundingClientRect().right),
      ).toBeLessThanOrEqual(roundingPx);
    }
    expect(missedPresses()).toEqual([]);
  });

  it("flows short rows several to a line, their columns lined up within each", async () => {
    const cast = Array.from({ length: 9 }, (_, index) => ({
      name:
        index % 2 === 0 ? `Actor ${String(index)}` : `A Much Longer Actor Name ${String(index)}`,
      note: "",
      role: "",
    }));
    mounted = await mount(
      <div style={{ inlineSize: "50rem" }}>
        <GridList
          rows={cast}
          rowKey={(person) => person.name}
          columns={[
            {
              key: "face",
              text: "fit",
              cell: () => <span style={{ display: "inline-block", inlineSize: "2.75rem" }} />,
            },
            { key: "name", text: "wrap", cell: (person) => person.name },
          ]}
          target={asLink}
          isAcross
        />
      </div>,
    );
    const rows = cellsByRow();
    const lefts = rows.map((cells) => cells[1]?.getBoundingClientRect().left ?? Number.NaN);
    const tops = [...document.querySelectorAll(".grid-list__row")].map(
      (row) => row.getBoundingClientRect().top,
    );
    const onFirstLine = tops.filter((top) => Math.abs(top - (tops[0] ?? 0)) <= roundingPx).length;
    // Several to a line, and more than one line.
    expect(onFirstLine).toBeGreaterThan(1);
    expect(onFirstLine).toBeLessThan(rows.length);
    // Each entry under the one above it: the lines' names start at the same places.
    const firstLine = lefts.slice(0, onFirstLine);
    lefts.forEach((left, index) => {
      const above = firstLine[index % firstLine.length] ?? Number.NaN;
      expect(Math.abs(left - above)).toBeLessThanOrEqual(roundingPx);
    });
    expect(missedPresses()).toEqual([]);
    expect(cellsOutsideTargets()).toEqual([]);
  });
});

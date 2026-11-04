// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { listStateOf } from "./browse.tsx";
import { i18n } from "./i18n.ts";
import {
  clearedCriteria,
  criteriaKey,
  criteriaSummary,
  isNarrowed,
  type ListCriteria,
} from "./list-filters.ts";

const t = i18n.t.bind(i18n);

/** A list as it opens: by name, its own way, nothing typed, no chip. */
const byName: ListCriteria = {
  query: "",
  choice: { sort: "name", order: "ascending" },
  filters: {},
};

/** Every criterion set at once: the text, a sort run against its own way, and each chip. */
const everything: ListCriteria = {
  query: "  heat ",
  choice: { sort: "added", order: "ascending" },
  filters: { genre: 3, decade: 1990, isUnwatchedOnly: true },
};

describe("a list's Sort and filter's summary (D417 §1)", () => {
  it("names a sort run its own way by its field alone, and nothing else when nothing narrows", () => {
    expect(criteriaSummary(t, byName, undefined)).toBe("Name");
    expect(
      criteriaSummary(t, { ...byName, choice: { sort: "added", order: "descending" } }, undefined),
    ).toBe("Added");
  });

  it("names a sort run the other way by its way, then the text, the genre, the decade and Unwatched", () => {
    expect(criteriaSummary(t, everything, "Drama")).toBe(
      "Oldest added · “heat” · Drama · 1990s · Unwatched",
    );
  });

  it("leaves the genre out until its name is read", () => {
    expect(criteriaSummary(t, everything, undefined)).toBe(
      "Oldest added · “heat” · 1990s · Unwatched",
    );
  });
});

describe("a list's Sort and filter's Clear (D417)", () => {
  it("is offered while the text or a chip narrows the list, not for a sort alone", () => {
    expect(isNarrowed(byName)).toBe(false);
    expect(isNarrowed({ ...byName, query: "   " })).toBe(false);
    expect(isNarrowed({ ...byName, choice: { sort: "added", order: "ascending" } })).toBe(false);
    expect(isNarrowed({ ...byName, query: "h" })).toBe(true);
    expect(isNarrowed({ ...byName, filters: { decade: 1990 } })).toBe(true);
    expect(isNarrowed({ ...byName, filters: { isUnwatchedOnly: true } })).toBe(true);
  });

  it("clears the text and every chip, and keeps the sort", () => {
    const cleared = clearedCriteria(everything);
    expect(cleared).toEqual({
      query: "",
      choice: { sort: "added", order: "ascending" },
      filters: {},
    });
    expect(isNarrowed(cleared)).toBe(false);
  });
});

describe("a list's criteria's key", () => {
  it("is another for each criterion changed, and the same for text differing only by spaces", () => {
    const changed: ListCriteria[] = [
      byName,
      { ...byName, query: "heat" },
      { ...byName, choice: { sort: "name", order: "descending" } },
      { ...byName, filters: { genre: 3 } },
      { ...byName, filters: { decade: 1990 } },
      { ...byName, filters: { isUnwatchedOnly: true } },
    ];
    const keys = changed.map((each) => criteriaKey(each));
    expect(new Set(keys).size).toBe(keys.length);
    expect(criteriaKey({ ...byName, query: " heat  " })).toBe(
      criteriaKey({ ...byName, query: "heat" }),
    );
  });
});

// The owner, 2026-11-04: every criterion adds to the others, the search too, all in the address.
describe("a list's criteria in the address (listStateOf)", () => {
  it("holds each criterion set, together, the text trimmed", () => {
    expect(listStateOf(everything)).toEqual({
      sort: "added",
      order: "ascending",
      query: "heat",
      genre: 3,
      decade: 1990,
      isUnwatchedOnly: true,
    });
  });

  it("leaves out each at the list's own: by name, its own way, no text, no chip", () => {
    expect(listStateOf(byName)).toEqual({});
    expect(listStateOf({ ...byName, query: "  " })).toEqual({});
    expect(listStateOf({ ...byName, choice: { sort: "name", order: "descending" } })).toEqual({
      order: "descending",
    });
  });
});

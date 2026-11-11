// SPDX-License-Identifier: MPL-2.0

/**
 * A poster card's translations: one for the card, so a page of 60 does not build react-i18next 60
 * times over a copy of the i18n object for each of its parts (the Fire TV's trace, 2026-11-11).
 */

import "./i18n.ts";

import * as reactI18next from "react-i18next";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it, vi } from "vitest";

import { EntryPoster, type Entry } from "./browse.tsx";
import { withServices } from "./test-services.tsx";

vi.mock("react-i18next", async (original) => {
  const actual = await original<typeof reactI18next>();
  return { ...actual, useTranslation: vi.fn(actual.useTranslation) };
});

const heat: Entry = {
  id: 10,
  name: "Metropolis",
  kind: "film",
  year: 1995,
  identificationState: "identified",
  addedAt: 1,
};

describe("a poster card's translations", () => {
  it("is one for a card with a poster, its year and how far it is watched", () => {
    const translations = vi.mocked(reactI18next.useTranslation);
    translations.mockClear();
    const markup = renderToStaticMarkup(
      withServices(<EntryPoster entry={{ ...heat, posterSha256: "ab12", watchedPercent: 40 }} />),
    );
    expect(markup).toContain("1995");
    expect(translations).toHaveBeenCalledTimes(1);
  });

  it("adds one, for its caption, to a card with no poster", () => {
    const translations = vi.mocked(reactI18next.useTranslation);
    translations.mockClear();
    const markup = renderToStaticMarkup(withServices(<EntryPoster entry={heat} />));
    expect(markup).toContain("No poster");
    expect(translations).toHaveBeenCalledTimes(2);
  });
});

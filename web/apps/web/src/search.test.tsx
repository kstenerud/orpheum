// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { hitRoute, Search, SearchResults } from "./search.tsx";
import { withServices } from "./test-services.tsx";

describe("search", () => {
  it("leads each result to its page, and says what kind it is", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <SearchResults
          query="bad"
          hits={[
            { id: 20, kind: "series", name: "Brass Arena", year: 2008 },
            { id: 21, kind: "episode", name: "Pilot" },
          ]}
        />,
      ),
    );
    expect(markup).toContain('href="/series/20"');
    expect(markup).toContain('href="/titles/21"');
    expect(markup).toContain("Series");
    expect(markup).toContain("Episode");
    expect(hitRoute({ id: 5, kind: "movie", name: "Metropolis" })).toEqual({ screen: "title", id: 5 });
  });

  it("says when nothing matches, and keeps the query in the box", () => {
    expect(renderToStaticMarkup(withServices(<SearchResults query="zzz" hits={[]} />))).toContain(
      "Nothing matches “zzz”.",
    );
    const markup = renderToStaticMarkup(withServices(<Search query="heat" />));
    expect(markup).toContain('value="heat"');
    expect(markup).toContain('role="search"');
  });
});

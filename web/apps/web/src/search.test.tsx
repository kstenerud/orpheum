// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { itemRoute } from "./router.ts";
import { SearchResults } from "./search.tsx";
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
            { id: 22, kind: "other", name: "Beach day" },
          ]}
        />,
      ),
    );
    expect(markup).toContain('href="/series/20"');
    expect(markup).toContain('href="/titles/21"');
    expect(markup).toContain("Series");
    expect(markup).toContain("Episode");
    expect(markup).toContain('href="/titles/22"');
    expect(markup).toContain("Video");
    expect(itemRoute("movie", 5)).toEqual({
      screen: "title",
      itemId: 5,
    });
  });
});

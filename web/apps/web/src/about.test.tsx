// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { About, acknowledgements } from "./about.tsx";

describe("About", () => {
  it("credits TMDB with its logo and exact notice, and names what we are built on", () => {
    const markup = renderToStaticMarkup(<About />);
    expect(markup).toContain(
      "This product uses the TMDB API but is not endorsed or certified by TMDB.",
    );
    expect(markup).toContain('alt="The Movie Database (TMDB)"');
    expect(markup).toContain("tmdb-logo");
    for (const project of ["FFmpeg", "SQLite", "Go", "React"]) {
      expect(acknowledgements.map((acknowledged) => acknowledged.name)).toContain(project);
      expect(markup).toContain(`>${project}</a>`);
    }
  });

  it("is headed by the page's one h1, its sections by h2s (ui-audit-2026-11-04.md §3)", () => {
    const markup = renderToStaticMarkup(<About />);
    expect(markup.match(/<h1[ >]/g)).toHaveLength(1);
    expect(markup).toContain('<h1 id="about-heading">About</h1>');
    expect(markup).not.toContain("<h3");
  });
});

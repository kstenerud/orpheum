// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { App } from "./app.tsx";
import { testServices, withServices } from "./test-services.tsx";

describe("App", () => {
  it("asks to sign in until there is a session", () => {
    const markup = renderToStaticMarkup(withServices(<App />, testServices()));
    expect(markup).toContain('<main class="sign-in">');
    expect(markup).not.toContain("app-shell");
  });

  it("shows the shell and the route's screen once signed in", () => {
    const markup = renderToStaticMarkup(withServices(<App />, testServices("token", "/about")));
    expect(markup).toContain('class="app-shell"');
    expect(markup).toContain("not endorsed or certified by TMDB");
  });

  it("shows the screen each address names", () => {
    const films = renderToStaticMarkup(withServices(<App />, testServices("token", "/films")));
    expect(films).toContain('id="browse-heading">Films<');
    const search = renderToStaticMarkup(withServices(<App />, testServices("token", "/search")));
    expect(search).toContain('role="search"');
    const libraries = renderToStaticMarkup(
      withServices(<App />, testServices("token", "/libraries")),
    );
    expect(libraries).toContain('id="libraries-heading"');
    const files = renderToStaticMarkup(
      withServices(<App />, testServices("token", "/libraries/3/files")),
    );
    expect(files).toContain('id="files-heading"');
    const identify = renderToStaticMarkup(
      withServices(<App />, testServices("token", "/identify")),
    );
    expect(identify).toContain('id="identify-heading"');
  });

  it("says when an address names nothing", () => {
    const markup = renderToStaticMarkup(withServices(<App />, testServices("token", "/nowhere")));
    expect(markup).toContain("There is nothing at this address.");
  });
});

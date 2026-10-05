// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { AppShell } from "./app-shell.tsx";
import { withServices } from "./test-services.tsx";

const ignore = (): void => undefined;

describe("AppShell", () => {
  it("renders a banner with the product name and the menu, and one main landmark", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <AppShell
          isAdmin={false}
          currentScreen="films"
          onSignOut={ignore}
          signedInAs={{ name: "partner", deviceName: "Firefox on Mac" }}
        >
          <p>screen</p>
        </AppShell>,
      ),
    );
    expect(markup.match(/<main/gu)).toHaveLength(1);
    expect(markup).toContain(
      '<header class="app-shell__banner"><p class="app-shell__title">Orpheum</p>',
    );
    expect(markup).toContain('<li aria-current="page"><a href="/films">Films</a></li>');
    expect(markup).toContain('<a href="/watch-together">Watch Together</a>');
    expect(markup).not.toContain("/libraries");
    expect(markup).not.toContain("/jobs");
    expect(markup).toContain("partner on Firefox on Mac");
  });

  it("shows the admin's destinations to an admin", () => {
    const markup = renderToStaticMarkup(
      withServices(
        <AppShell isAdmin={true} currentScreen="about" onSignOut={ignore} signedInAs={undefined}>
          <p>screen</p>
        </AppShell>,
      ),
    );
    expect(markup).toContain('href="/libraries"');
    expect(markup).toContain('href="/identify"');
    expect(markup).toContain('href="/settings"');
    expect(markup).toContain('href="/jobs"');
    expect(markup).toContain('href="/system"');
  });
});

// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { App } from "./app.tsx";
import {
  buttonWithText,
  click,
  mount,
  refusal,
  servedBy,
  unreachable,
  type Answers,
} from "./test-dom.tsx";
import { testServices, withServices } from "./test-services.tsx";

/** What the sign-in page says after signing out, when the server answers DELETE /session so. */
async function noticeAfterSigningOut(answer: Answers[string]): Promise<string | undefined> {
  const services = servedBy(
    {
      "GET /me": [200, { id: 1, name: "owner", role: "admin", isAdmin: true }],
      "GET /home": [200, { continueWatching: [] }],
      "GET /titles": [200, { titles: [], nextCursor: "" }],
      "DELETE /session": answer,
    },
    [],
    "token",
  );
  const mounted = await mount(withServices(<App />, services));
  await click(buttonWithText(mounted.container, "Sign out"));
  const isSignedOut = services.session.token() === undefined;
  const notice = mounted.container.querySelector("output")?.textContent ?? undefined;
  mounted.unmount();
  return isSignedOut ? notice : "still signed in";
}

describe("App", () => {
  it("asks to sign in until there is a session", () => {
    const markup = renderToStaticMarkup(withServices(<App />, testServices()));
    expect(markup).toContain('<main class="credentials-form">');
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
    const jobs = renderToStaticMarkup(withServices(<App />, testServices("token", "/jobs")));
    expect(jobs).toContain('id="jobs-heading"');
    const system = renderToStaticMarkup(withServices(<App />, testServices("token", "/system")));
    expect(system).toContain("Loading…");
    expect(system).not.toContain("There is nothing at this address.");
  });

  it("says when an address names nothing", () => {
    const markup = renderToStaticMarkup(withServices(<App />, testServices("token", "/nowhere")));
    expect(markup).toContain("There is nothing at this address.");
  });

  it("signs out here whatever the server says, and says when the server could not be told", async () => {
    expect(await noticeAfterSigningOut([204, undefined])).toBeUndefined();
    expect(await noticeAfterSigningOut(unreachable)).toContain("the server could not be told");
    expect(await noticeAfterSigningOut(refusal(503, "migrating"))).toContain(
      "the server could not be told",
    );
  });
});

// SPDX-License-Identifier: MPL-2.0

/**
 * What the tab remembers of its pages (remembered.ts) is forgotten at each test's mount, as a new
 * tab would start: a page mounted in one walk showed, in Firefox, the details an earlier walk in
 * its file left under the same id (the series agent, 2026-11-04), since a page shows what the tab
 * remembers of it while it loads again.
 */

import "./i18n.ts";

import { apiPrefix } from "@orpheum/sdk";
import { afterEach, describe, expect, it } from "vitest";

import { filmDetail, mountApp, viewer, walkServices, type Walking } from "./test-walk.tsx";
import { testServices } from "./test-services.tsx";
import type { Services } from "./services.tsx";

const signedIn = {
  "GET /me": viewer,
  "GET /client-settings": {},
  "GET /home": { continueWatching: [] },
};

/** A series named name, with one season of one episode. */
function seriesDetail(id: number, name: string): Record<string, unknown> {
  return {
    id,
    kind: "series",
    name,
    libraryId: 1,
    year: 2019,
    art: {},
    orderings: [{ name: "aired", kind: "aired", episodeCount: 1 }],
    seasons: [{ seasonNumber: 1, episodeCount: 1 }],
  };
}

/**
 * Services at path whose server answers as signed in, never answers GET /items/{id} (the page
 * stays loading), and answers every other request as not found; heard gets each request's path.
 */
function heldOnItem(id: number, path: string, heard: string[]): Services {
  const held = `${apiPrefix}/items/${String(id)}`;
  return testServices("token", path, async (request) => {
    await Promise.resolve();
    const url = new URL(request.url);
    heard.push(url.pathname);
    if (url.pathname === held) {
      return await new Promise<never>(() => undefined);
    }
    const answer = (signedIn as Record<string, unknown>)[
      `${request.method} ${url.pathname.slice(apiPrefix.length)}`
    ];
    return answer === undefined
      ? Response.json({ error: { code: "notFound", message: "none" } }, { status: 404 })
      : Response.json(answer, { status: 200 });
  });
}

/** The text of the page's heading, or "" when it has none. */
function heading(): string {
  return document.querySelector("h1")?.textContent ?? "";
}

describe("a page mounted after an earlier walk showed the same id", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
  });

  it("shows a film's page loading, not the details the earlier walk's page had", async () => {
    walking = await mountApp(
      walkServices({ ...signedIn, "GET /items/106": filmDetail(106, "First Cut") }, "/titles/106"),
    );
    await expect.poll(heading).toContain("First Cut");
    walking.unmount();
    const heard: string[] = [];
    walking = await mountApp(heldOnItem(106, "/titles/106", heard));
    await expect.poll(() => heard).toContain(`${apiPrefix}/items/106`);
    expect(heading()).not.toContain("First Cut");
    await expect.poll(() => document.querySelector(".title__loading")).not.toBeNull();
  });

  it("shows a series' page loading, not the details the earlier walk's page had", async () => {
    walking = await mountApp(
      walkServices({ ...signedIn, "GET /items/26": seriesDetail(26, "Glass Tide") }, "/series/26"),
    );
    await expect.poll(heading).toContain("Glass Tide");
    walking.unmount();
    const heard: string[] = [];
    walking = await mountApp(heldOnItem(26, "/series/26", heard));
    await expect.poll(() => heard).toContain(`${apiPrefix}/items/26`);
    expect(heading()).not.toContain("Glass Tide");
    await expect.poll(() => document.querySelector(".title__loading")).not.toBeNull();
  });
});

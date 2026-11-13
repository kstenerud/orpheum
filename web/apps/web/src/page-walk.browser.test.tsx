// SPDX-License-Identifier: MPL-2.0

/**
 * Every page the test services' answers draw, walked by arrows from its arrival in a real browser at
 * a Fire TV's page in Silk (960 × 480): each control on it must be reached, or a remote cannot use it
 * (ui-audit-2026-11-04.md §2, Android TV's checklist; R3 and R6). The walk follows every arrow from
 * every control reached (test-walk.tsx reachedFrom), the banner's included, or, where a dialog opens
 * over the page, the dialog's alone. A page whose reads the answers here do not give (a person's
 * record, the jobs, the update's state) draws its failure or its waiting, and is walked as it is.
 *
 * Here the library's pages and sign-in; page-walk-manage.browser.test.tsx and
 * page-walk-person.browser.test.tsx walk the rest, the answers and the walk all three take in
 * test-page-walk.tsx (split for the browser run's time, make-check-time-2026-11-12.md §2 and §3).
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { createSession } from "./session.ts";
import { anAdmin, libraryPages, unreachedOnThePage } from "./test-page-walk.tsx";
import { mountApp, walkServices, type Walking } from "./test-walk.tsx";

/** The walk's tools: vitest's own expect and keyboard. */
const tools = {
  expect,
  keyboard: async (keys: string) => {
    await userEvent.keyboard(keys);
  },
};

describe("every page by a remote's arrows, on a Fire TV's page in Silk", () => {
  let walking: Walking | undefined;
  afterEach(() => {
    walking?.unmount();
    walking = undefined;
    window.scrollTo({ top: 0, left: 0 });
  });

  it.each(libraryPages)("reaches every control of %s from its arrival", async (path) => {
    await page.viewport(960, 480);
    walking = await mountApp(walkServices(anAdmin, path));
    expect(await unreachedOnThePage(tools)).toEqual([]);
  });

  it("reaches every control of the sign-in page from its arrival", async () => {
    await page.viewport(960, 480);
    const services = walkServices({ "GET /client-settings": {} });
    walking = await mountApp({ ...services, session: createSession(undefined) });
    expect(await unreachedOnThePage(tools)).toEqual([]);
  });
});

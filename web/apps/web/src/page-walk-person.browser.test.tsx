// SPDX-License-Identifier: MPL-2.0

/**
 * Every page by a remote's arrows from its arrival, as page-walk.browser.test.tsx walks them, here
 * a person's own and the rest: each control on it must be reached, or a remote cannot use it.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { anAdmin, personPages, unreachedOnThePage } from "./test-page-walk.tsx";
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

  it.each(personPages)("reaches every control of %s from its arrival", async (path) => {
    await page.viewport(960, 480);
    walking = await mountApp(walkServices(anAdmin, path));
    expect(await unreachedOnThePage(tools)).toEqual([]);
  });
});

// SPDX-License-Identifier: MPL-2.0

import { expect, it } from "vitest";
import { commands } from "vitest/browser";

import { keepTheFrameFocused } from "./frame-focus.ts";

declare module "vitest/browser" {
  interface BrowserCommands {
    focusAfterAPageOpens: (
      keeper: string,
    ) => Promise<{ isFocused: boolean; isFocusShown: boolean; clicks: number }>;
  }
}

// Firefox took the focus from the frame of the page opened before a new one, and a test there read
// its focused tile as not shown, or saw its Enter click nothing (frame-focus.ts). Chromium never did.
it("keeps a test frame's focus, shown, and its Enter's click when another page opens", async () => {
  expect(await commands.focusAfterAPageOpens(keepTheFrameFocused.toString())).toEqual({
    isFocused: true,
    isFocusShown: true,
    clicks: 1,
  });
});

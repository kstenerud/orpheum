// SPDX-License-Identifier: MPL-2.0

import { expect, it } from "vitest";
import { commands } from "vitest/browser";

import { keepTheFrameFocused } from "./frame-focus.ts";

declare module "vitest/browser" {
  interface BrowserCommands {
    focusAfterAPageOpens: (
      keeper: string,
    ) => Promise<{ blurs: number; isFocused: boolean; isFocusShown: boolean; clicks: number }>;
  }
}

// Firefox took the focus from the frame of the page opened before a new one, and a test there read
// its focused tile as not shown, or saw its Enter click nothing, or heard a field save what it had
// typed as the field blurred (frame-focus.ts). Chromium never did. The field hears one blur: the
// focus moving to the button, not the page opening before it.
it("keeps a test frame's focus, shown, its field unblurred and its Enter's click when another page opens", async () => {
  expect(await commands.focusAfterAPageOpens(keepTheFrameFocused.toString())).toEqual({
    blurs: 1,
    isFocused: true,
    isFocusShown: true,
    clicks: 1,
  });
});

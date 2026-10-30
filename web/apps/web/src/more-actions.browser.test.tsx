// SPDX-License-Identifier: MPL-2.0

/**
 * A page's "…" of rarely used actions (more-actions.tsx) in a real browser, whose modal dialog takes
 * focus as it opens and leaves the page behind it inert: a place's dialog takes the list's place.
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import type { JSX } from "react";
import { describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { ModalDialog } from "./modal-dialog.tsx";
import { MoreActions } from "./more-actions.tsx";
import { mount } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A page's "…" with one rarely used action, whose dialog has a button of its own. */
function Page(): JSX.Element {
  return (
    <MoreActions
      actions={[
        {
          key: "rare",
          label: "Something rare",
          dialog: (close) => (
            <ModalDialog label="Rare" onClose={close}>
              <button type="button">inside</button>
            </ModalDialog>
          ),
        },
      ]}
    />
  );
}

describe("a page's menu of rarely used actions, in a real browser", () => {
  // The owner, 2026-10-29: "if that selection also opens another modal, the new modal should not
  // stack on top of the other modal; they should swap in/out, such that a single click outside of
  // any modal view dismisses and returns the user to normal functionality".
  it("opens a place's dialog in the list's place, and one press outside it is back on the page, focus on the …", async () => {
    await page.viewport(1280, 800);
    const mounted = await mount(withServices(<Page />));
    const more = page.getByRole("button", { name: "More actions" }).element();
    await userEvent.click(more);
    await expect.poll(() => document.querySelectorAll("dialog[open]").length).toBe(1);
    await userEvent.click(page.getByRole("button", { name: "Something rare" }).element());
    await expect
      .poll(() => [...document.querySelectorAll("dialog[open]")].map((each) => each.ariaLabel))
      .toEqual(["Rare"]);
    // A press on the backdrop, which reaches the dialog itself at a point outside its box: told, not
    // moved to by the pointer, which would stay there, hovering whatever the next file draws under
    // it.
    document
      .querySelector("dialog[open]")
      ?.dispatchEvent(new MouseEvent("click", { bubbles: true, clientX: 2, clientY: 2 }));
    await expect.poll(() => document.querySelectorAll("dialog").length).toBe(0);
    // Focus back where the person was (D361 §3), not lost to the page's start with the list the
    // dialog was opened from.
    await expect.poll(() => document.activeElement).toBe(more);
    mounted.unmount();
  });
});

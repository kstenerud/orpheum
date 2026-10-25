// SPDX-License-Identifier: MPL-2.0

/**
 * A modal dialog as a real browser lays it out (vitest's browser mode): modal-dialog.css alone keeps
 * the page behind it still while it is open (the owner, 2026-10-21), and a remote's arrows stay in
 * it while it is.
 */

import "./tokens.css";
import "./base.css";

import { useState, type JSX } from "react";
import { describe, expect, it } from "vitest";
import { userEvent } from "vitest/browser";

import { ModalDialog } from "./modal-dialog.tsx";
import { mount } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A page with a button beside a dialog of two choices, which a controller's Back closes. */
function PageWithADialog(): JSX.Element {
  const [isOpen, setIsOpen] = useState(true);
  return (
    <main>
      <button type="button">behind</button>
      {isOpen ? (
        <ModalDialog
          label="Which"
          onClose={() => {
            setIsOpen(false);
          }}
        >
          <button type="button">first</button>
          <button type="button">second</button>
        </ModalDialog>
      ) : null}
    </main>
  );
}

describe("a modal dialog, laid out by a real browser", () => {
  it("keeps the page behind it from scrolling while it is open, and lets it scroll again once closed", async () => {
    const page = document.documentElement;
    const mounted = await mount(
      <>
        <div style={{ height: "5000px" }} />
        <ModalDialog label="What it is for" onClose={() => undefined}>
          <p>Short.</p>
        </ModalDialog>
      </>,
    );
    expect(getComputedStyle(page).overflow).toBe("hidden");
    expect(
      getComputedStyle(mounted.container.querySelector("dialog") ?? page).overscrollBehavior,
    ).toBe("contain");
    mounted.unmount();
    expect(getComputedStyle(page).overflow).not.toBe("hidden");
  });

  it("keeps the arrows among its own controls, and closes on a remote's Back", async () => {
    const mounted = await mount(withServices(<PageWithADialog />));
    const names = async (keys: string): Promise<string | null | undefined> => {
      await userEvent.keyboard(keys);
      return document.activeElement?.textContent;
    };
    expect(document.activeElement?.closest("dialog")).not.toBeNull();
    expect(await names("{ArrowDown}{ArrowDown}{ArrowDown}")).not.toBe("behind");
    expect(await names("{ArrowUp}{ArrowUp}{ArrowLeft}{ArrowRight}")).not.toBe("behind");
    expect(document.activeElement?.closest("dialog")).not.toBeNull();
    await userEvent.keyboard("{Backspace}");
    expect(mounted.container.querySelector("dialog")).toBeNull();
    mounted.unmount();
  });
});

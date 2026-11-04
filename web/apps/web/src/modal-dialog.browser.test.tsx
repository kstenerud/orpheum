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
import { page, userEvent } from "vitest/browser";

import { ModalDialog } from "./modal-dialog.tsx";
import { mount } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A page with a button beside a dialog of two choices, which a controller's Back closes. */
function PageWithADialog(props: { readonly shouldStayOpenOnOutsidePress?: boolean }): JSX.Element {
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
          shouldStayOpenOnOutsidePress={props.shouldStayOpenOnOutsidePress === true}
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

  // The owner, 2026-10-30: "a single click outside of any modal view dismisses"; a question whose
  // closing would answer it stays.
  it("closes on a press outside it, unless told to stay open", async () => {
    // A press on the backdrop reaches the dialog itself at a point outside its box: told, not moved
    // to by the pointer, which would stay hovering whatever the next file draws there.
    const pressOutside = (): void => {
      document
        .querySelector("dialog[open]")
        ?.dispatchEvent(new MouseEvent("click", { bubbles: true, clientX: 2, clientY: 2 }));
    };
    const closing = await mount(withServices(<PageWithADialog />));
    await expect.poll(() => document.querySelectorAll("dialog[open]").length).toBe(1);
    pressOutside();
    await expect.poll(() => document.querySelectorAll("dialog[open]").length).toBe(0);
    closing.unmount();
    const staying = await mount(withServices(<PageWithADialog shouldStayOpenOnOutsidePress />));
    await expect.poll(() => document.querySelectorAll("dialog[open]").length).toBe(1);
    pressOutside();
    await new Promise((resolve) => setTimeout(resolve, 100));
    expect(document.querySelectorAll("dialog[open]")).toHaveLength(1);
    staying.unmount();
  });

  // The owner, 2026-10-30: every dialog has the same Close, a cross at its top-right.
  it("puts its Close at its top-right corner, its first line kept clear of it", async () => {
    await page.viewport(960, 600);
    const mounted = await mount(
      withServices(
        <ModalDialog label="Which" onClose={() => undefined}>
          <p>{"A first line long enough to reach the dialog's far side, ".repeat(4)}</p>
        </ModalDialog>,
      ),
    );
    const dialog = mounted.container.querySelector("dialog")?.getBoundingClientRect();
    const close = mounted.container.querySelector(".modal-dialog__close")?.getBoundingClientRect();
    const line = mounted.container.querySelector("dialog p")?.getBoundingClientRect();
    if (dialog === undefined || close === undefined || line === undefined) {
      throw new Error("the dialog, its Close or its line is not drawn");
    }
    expect(dialog.right - close.right).toBeLessThanOrEqual(12);
    expect(close.top - dialog.top).toBeLessThanOrEqual(12);
    expect(line.right).toBeLessThanOrEqual(close.left);
    mounted.unmount();
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

  // The owner on a Fire TV, 2026-10-28: a chooser's button left off the screen as its list closed
  // was brought back to its middle, the page jumping under the remote.
  it("gives focus back to its opener without moving the page where the opener shows, and brings one off the screen in only as far as it must", async () => {
    await page.viewport(960, 480);
    function Opened(): JSX.Element {
      const [isOpen, setIsOpen] = useState(false);
      return (
        <>
          <div style={{ height: "1500px" }} />
          <button
            type="button"
            onClick={() => {
              setIsOpen(true);
            }}
          >
            opener
          </button>
          <div style={{ height: "1500px" }} />
          {isOpen ? (
            <ModalDialog
              label="Which"
              onClose={() => {
                setIsOpen(false);
              }}
            >
              <button type="button">inside</button>
            </ModalDialog>
          ) : null}
        </>
      );
    }
    const mounted = await mount(withServices(<Opened />));
    const opener = page.getByRole("button", { name: "opener" }).element();
    const openAndClose = async (): Promise<void> => {
      await userEvent.click(opener);
      await expect.poll(() => document.querySelector("dialog")).not.toBeNull();
      await userEvent.keyboard("{Escape}");
      await expect.poll(() => document.activeElement).toBe(opener);
    };
    window.scrollTo({ top: 1500 - 240 });
    const shown = window.scrollY;
    await openAndClose();
    expect(window.scrollY).toBe(shown);
    // Opened, then the page moved off it: it comes in at the screen's bottom edge, not its middle.
    await userEvent.click(opener);
    await expect.poll(() => document.querySelector("dialog")).not.toBeNull();
    window.scrollTo({ top: 0 });
    await userEvent.keyboard("{Escape}");
    await expect.poll(() => document.activeElement).toBe(opener);
    expect(opener.getBoundingClientRect().bottom).toBeGreaterThan(window.innerHeight - 4);
    expect(opener.getBoundingClientRect().bottom).toBeLessThanOrEqual(window.innerHeight + 1);
    mounted.unmount();
  });
});

/** A page whose button opens a dialog of one choice, as a page's dialogs are opened. */
function PageWithAnOpener(): JSX.Element {
  const [isOpen, setIsOpen] = useState(false);
  return (
    <main>
      <button
        type="button"
        onClick={() => {
          setIsOpen(true);
        }}
      >
        opener
      </button>
      {isOpen ? (
        <ModalDialog
          label="Which"
          onClose={() => {
            setIsOpen(false);
          }}
        >
          <button type="button">inside</button>
        </ModalDialog>
      ) : null}
    </main>
  );
}

// R5 of ui-audit-2026-11-04.md: a dialog closed any way gives focus back to the button that opened it.
describe("a modal dialog closed", () => {
  it("gives focus back to its opener, closed by Back, by its Close and by a press outside", async () => {
    await page.viewport(960, 480);
    const mounted = await mount(withServices(<PageWithAnOpener />));
    const opener = page.getByRole("button", { name: "opener" }).element();
    const opened = async (): Promise<void> => {
      if (opener instanceof HTMLElement) {
        opener.focus();
      }
      await userEvent.keyboard("{Enter}");
      await expect.poll(() => document.activeElement?.closest("dialog[open]")).not.toBeNull();
    };
    const closed = async (): Promise<void> => {
      await expect.poll(() => document.querySelector("dialog")).toBeNull();
      expect(document.activeElement).toBe(opener);
    };
    await opened();
    await userEvent.keyboard("{Backspace}");
    await closed();
    await opened();
    document.querySelector<HTMLElement>(".modal-dialog__close button")?.focus();
    await userEvent.keyboard("{Enter}");
    await closed();
    await opened();
    document
      .querySelector("dialog[open]")
      ?.dispatchEvent(new MouseEvent("click", { bubbles: true, clientX: 2, clientY: 2 }));
    await closed();
    mounted.unmount();
  });
});

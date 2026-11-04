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

import { dangerousMark } from "./focus-navigation.ts";
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
      // Opened by click(), not by Enter: the browser's own Enter click is not what this test is of,
      // and Firefox now and then never dispatches it (idiosyncrasies.md, "Firefox now and then
      // drops the click of an Enter on a button").
      if (opener instanceof HTMLElement) {
        opener.click();
      }
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

  // The Cast and Watch Together dialogs, taller than the window, stopped Tab on their body: a box
  // that scrolls is a Tab stop of its own in Chromium and Firefox (idiosyncrasies.md).
  it("stops Tab only on its controls, never on its own body, however long it is", async () => {
    const mounted = await mount(
      withServices(
        <ModalDialog label="Long" onClose={() => undefined}>
          <button type="button">first</button>
          <div style={{ height: "3000px" }} />
          <button type="button">last</button>
        </ModalDialog>,
      ),
    );
    const dialog = document.querySelector("dialog[open]");
    expect(dialog?.scrollHeight).toBeGreaterThan(dialog?.clientHeight ?? 0);
    const stops: string[] = [];
    for (let press = 0; press < 4; press += 1) {
      await userEvent.keyboard("{Tab}");
      const at = document.activeElement;
      stops.push(at === null ? "none" : at.tagName.toLowerCase() + ":" + at.textContent);
    }
    expect(stops.filter((stop) => stop.startsWith("dialog"))).toEqual([]);
    mounted.unmount();
  });
});

/** A dialog of the choices named, those in dangerous marked as dangerous actions. */
function DialogOf(props: {
  readonly choices: readonly string[];
  readonly dangerous?: readonly string[];
  readonly onPress?: (choice: string) => void;
}): JSX.Element {
  return (
    <ModalDialog label="Which" onClose={() => undefined}>
      <p>What happens next.</p>
      {props.choices.map((choice) => (
        <button
          key={choice}
          type="button"
          {...(props.dangerous?.includes(choice) === true ? dangerousMark : {})}
          onClick={() => {
            props.onPress?.(choice);
          }}
        >
          {choice}
        </button>
      ))}
    </ModalDialog>
  );
}

// D421 (the owner, 2026-11-04): "If the only options available in a dialog are all dangerous, don't
// auto-select any of them. The user must move onto one of them. Otherwise the first item and default
// selected item is the safe one."
describe("where a dialog's focus arrives as it opens (D421)", () => {
  it("arrives on its first choice not marked a dangerous action", async () => {
    const mounted = await mount(
      withServices(<DialogOf choices={["Erase", "Keep it", "Other"]} dangerous={["Erase"]} />),
    );
    await expect.poll(() => document.activeElement?.textContent).toBe("Keep it");
    await new Promise((resolve) => requestAnimationFrame(resolve));
    expect(document.activeElement?.textContent).toBe("Keep it");
    mounted.unmount();
  });

  it("arrives on no choice where every one is dangerous, nor on its Close, until the person moves onto one", async () => {
    const pressed: string[] = [];
    const mounted = await mount(
      withServices(
        <DialogOf
          choices={["Erase", "Wipe"]}
          dangerous={["Erase", "Wipe"]}
          onPress={(choice) => pressed.push(choice)}
        />,
      ),
    );
    const dialog = document.querySelector("dialog[open]");
    await expect.poll(() => document.activeElement).toBe(dialog);
    await new Promise((resolve) => requestAnimationFrame(resolve));
    expect(document.activeElement).toBe(dialog);
    await userEvent.keyboard("{Enter}");
    expect(pressed).toEqual([]);
    await userEvent.keyboard("{ArrowDown}");
    expect(["Erase", "Wipe"]).toContain(document.activeElement?.textContent);
    mounted.unmount();
  });

  it("arrives on its first choice where none is marked, as before, and on no Close where it offers nothing else", async () => {
    const marked = await mount(withServices(<DialogOf choices={["One", "Two"]} />));
    await expect.poll(() => document.activeElement?.textContent).toBe("One");
    marked.unmount();
    const bare = await mount(withServices(<DialogOf choices={[]} />));
    const dialog = document.querySelector("dialog[open]");
    await expect.poll(() => document.activeElement).toBe(dialog);
    await new Promise((resolve) => requestAnimationFrame(resolve));
    expect(document.activeElement?.closest("[data-dialog-close]")).toBeNull();
    bare.unmount();
  });
});

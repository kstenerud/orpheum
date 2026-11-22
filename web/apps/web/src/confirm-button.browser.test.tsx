// SPDX-License-Identifier: MPL-2.0

/**
 * The one confirm (ui.md rule 8, D538 §4) as a real browser draws it: ConfirmButton opens it, focus
 * on Cancel with its ring showing for a keyboard and a remote; Cancel, a press outside, Escape and the
 * remote's Back send nothing and give focus back to the button; only the action acts; and one opened
 * from a dialog replaces that dialog, which comes back as it closes (D538 §5). By a mouse, a finger, a
 * keyboard and a remote, at a phone's, a laptop's and a television's width.
 */

import "./i18n.ts";
import "./tokens.css";
import "./base.css";

import { useState, type JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { ConfirmButton } from "./confirm-button.tsx";
import { ModalDialog } from "./modal-dialog.tsx";
import { mount as mountTree, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

const mounts: Mounted[] = [];

async function mount(tree: Parameters<typeof mountTree>[0]): Promise<Mounted> {
  const mounted = await mountTree(tree);
  mounts.push(mounted);
  return mounted;
}

afterEach(() => {
  for (const mounted of mounts.splice(0)) {
    if (mounted.container.isConnected) {
      mounted.unmount();
    }
  }
});

/** A phone's, a laptop's and a television's width. */
const widths = [360, 1280, 1920] as const;

/** The ways a person presses. */
const ways = ["mouse", "touch", "keyboard", "remote"] as const;
type Way = (typeof ways)[number];

/** Whether focus shows its ring on element, as a keyboard or a remote put it there. */
function ringShows(element: Element | null): boolean {
  if (!(element instanceof HTMLElement) || !element.matches(":focus-visible")) {
    return false;
  }
  const style = getComputedStyle(element);
  return style.outlineStyle !== "none" || style.boxShadow !== "none";
}

/** Whether focus is on a control, never on nothing: not the page, nor a body without a stop. */
function isFocusOnSomething(): boolean {
  const at = document.activeElement;
  return at !== null && at !== document.body && at !== document.documentElement;
}

/** The dialogs a person sees: open, and not hidden under another (D538 §5). */
function seenDialogs(): (string | null)[] {
  return [...document.querySelectorAll<HTMLDialogElement>("dialog[open]")]
    .filter((dialog) => dialog.checkVisibility({ visibilityProperty: true }))
    .map((dialog) => dialog.ariaLabel);
}

/** The button whose words are text, wherever it is drawn. */
function buttonSaying(text: string): HTMLElement {
  const found = [...document.querySelectorAll<HTMLElement>("button")].filter(
    (button) => button.textContent === text && button.checkVisibility({ visibilityProperty: true }),
  );
  const last = found.at(-1);
  if (last === undefined) {
    throw new Error(`no button says ${text}`);
  }
  return last;
}

/** Presses button as the way named does. */
async function press(button: HTMLElement, way: Way): Promise<void> {
  if (way === "mouse") {
    await userEvent.click(button);
    return;
  }
  if (way === "touch") {
    // A tap focuses the button it lands on, then clicks it.
    button.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true, pointerType: "touch" }));
    button.focus();
    button.click();
    return;
  }
  if (document.activeElement !== button) {
    button.focus();
  }
  // Firefox now and then drops the click of an Enter on a button (idiosyncrasies.md): the click
  // itself, after a key, as the browser's own would be.
  await userEvent.keyboard("{Shift}");
  button.click();
}

/** A page with one confirm before what cannot be undone, counting what it confirmed. */
function PageWithConfirm(props: { readonly onConfirm: () => void }): JSX.Element {
  return (
    <main>
      <ConfirmButton
        label="Delete the role"
        question="Delete this role?"
        consequence="Whoever holds it loses what it let them do."
        confirmLabel="Delete it"
        onConfirm={props.onConfirm}
      />
    </main>
  );
}

/** A page whose dialog holds a confirm, to see the confirm take the dialog's place. */
function DialogWithConfirm(props: { readonly onConfirm: () => void }): JSX.Element {
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
          label="A role"
          heading="A role"
          onClose={() => {
            setIsOpen(false);
          }}
        >
          <ConfirmButton
            label="Delete the role"
            question="Delete this role?"
            consequence="Whoever holds it loses what it let them do."
            confirmLabel="Delete it"
            onConfirm={props.onConfirm}
          />
        </ModalDialog>
      ) : null}
    </main>
  );
}

/**
 * Moves focus to the button saying text by the way named: Tab and Shift+Tab for a keyboard, the
 * arrows for a remote, its foot a row, or on a phone's sheet a column. Focus is never on nothing on
 * the way, and its ring shows where it lands.
 */
async function moveTo(text: string, way: "keyboard" | "remote"): Promise<void> {
  // Cancel comes first: a keyboard goes back to it, as a Tab past a modal dialog's last control
  // leaves the page for the browser's own controls.
  const keys =
    way === "keyboard"
      ? [text === "Cancel" ? "{Shift>}{Tab}{/Shift}" : "{Tab}"]
      : ["{ArrowRight}", "{ArrowDown}", "{ArrowLeft}", "{ArrowUp}"];
  for (const key of keys) {
    if (document.activeElement?.textContent === text) {
      break;
    }
    await userEvent.keyboard(key);
    expect(isFocusOnSomething()).toBe(true);
  }
  expect(document.activeElement?.textContent).toBe(text);
  expect(ringShows(document.activeElement)).toBe(true);
}

/** Backs out of the open confirm as the way named does: a press outside, a tap of Cancel, Escape, Back. */
async function backOut(way: Way): Promise<void> {
  if (way === "mouse") {
    document
      .querySelector('dialog[role="alertdialog"]')
      ?.dispatchEvent(
        new PointerEvent("click", { bubbles: true, clientX: 2, clientY: 2, pointerType: "mouse" }),
      );
  } else if (way === "touch") {
    await press(buttonSaying("Cancel"), "touch");
  } else if (way === "keyboard") {
    await userEvent.keyboard("{Escape}");
  } else {
    await userEvent.keyboard("{Backspace}");
  }
}

describe("the confirm, by every way of pressing, at each width", () => {
  for (const width of widths) {
    for (const way of ways) {
      it(`starts on Cancel, backs out sending nothing, and acts only from its action, by ${way} at ${String(width)}px`, async () => {
        await page.viewport(width, 720);
        let confirmed = 0;
        await mount(
          withServices(
            <PageWithConfirm
              onConfirm={() => {
                confirmed += 1;
              }}
            />,
          ),
        );
        const opener = buttonSaying("Delete the role");
        await press(opener, way);
        await expect.poll(seenDialogs).toEqual(["Delete this role?"]);
        expect(confirmed).toBe(0);
        await expect.poll(() => document.activeElement?.textContent).toBe("Cancel");
        if (way === "keyboard" || way === "remote") {
          // The ring shows on each of its two choices, and Cancel is reached again.
          await moveTo("Delete it", way);
          await moveTo("Cancel", way);
        }
        // A press too many, Enter on Cancel, keeps things as they are.
        await backOut(way);
        await expect.poll(seenDialogs).toEqual([]);
        expect(confirmed).toBe(0);
        await expect.poll(() => document.activeElement).toBe(opener);
        await press(opener, way);
        await expect.poll(() => document.activeElement?.textContent).toBe("Cancel");
        if (way === "keyboard" || way === "remote") {
          await moveTo("Delete it", way);
        }
        await press(buttonSaying("Delete it"), way);
        await expect.poll(() => confirmed).toBe(1);
        await expect.poll(seenDialogs).toEqual([]);
        expect(isFocusOnSomething()).toBe(true);
      });
    }
  }
});

describe("a confirm opened from a dialog replaces it (D538 §5)", () => {
  for (const way of ways) {
    it(`shows the confirm alone, and brings the dialog back on Cancel, its focus on the button, by ${way}`, async () => {
      await page.viewport(1280, 720);
      let confirmed = 0;
      await mount(
        withServices(
          <DialogWithConfirm
            onConfirm={() => {
              confirmed += 1;
            }}
          />,
        ),
      );
      await press(buttonSaying("opener"), way);
      await expect.poll(seenDialogs).toEqual(["A role"]);
      const remove = buttonSaying("Delete the role");
      await press(remove, way);
      await expect.poll(seenDialogs).toEqual(["Delete this role?"]);
      await expect.poll(() => document.activeElement?.textContent).toBe("Cancel");
      await press(buttonSaying("Cancel"), way);
      await expect.poll(seenDialogs).toEqual(["A role"]);
      await expect.poll(() => document.activeElement).toBe(remove);
      expect(confirmed).toBe(0);
    });
  }
});

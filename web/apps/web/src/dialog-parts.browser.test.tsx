// SPDX-License-Identifier: MPL-2.0

/**
 * The Dialog's parts as a real browser lays them out (D538 §2–§5): its heading and its foot kept in
 * view while its body scrolls, a sheet from the screen's bottom on a phone, focus arriving on its main
 * action or, for a confirm, on its least harmful choice, one dialog opened from another replacing it,
 * and a press outside closing it, by a mouse, a finger, a keyboard and a remote, at a phone's, a
 * laptop's and a television's width.
 */

import "./i18n.ts";
import "./tokens.css";
import "./base.css";

import { useState, type JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { ConfirmDialog } from "./confirm-button.tsx";
import { arrivalMark } from "./focus-navigation.ts";
import { ModalDialog } from "./modal-dialog.tsx";
import { mount as mountTree, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** What this file's tests mounted, taken down after each, a failed one's open dialog with it. */
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
function seenDialogs(): HTMLDialogElement[] {
  return [...document.querySelectorAll<HTMLDialogElement>("dialog[open]")].filter((dialog) =>
    dialog.checkVisibility({ visibilityProperty: true }),
  );
}

/** A press outside the dialog, on its backdrop, as a mouse's or a finger's lands. */
function pressOutside(pointerType: "mouse" | "touch"): void {
  document
    .querySelector("dialog[open]:not([data-replaced])")
    ?.dispatchEvent(
      new PointerEvent("click", { bubbles: true, clientX: 2, clientY: 2, pointerType }),
    );
}

/**
 * A page whose opener opens a dialog of parts, a long body between its heading and its foot of
 * Cancel and Save, the main action; and, from the dialog, Deeper opens a second over it.
 */
function PageWithParts(): JSX.Element {
  const [opened, setOpened] = useState(0);
  return (
    <main>
      <button
        type="button"
        onClick={() => {
          setOpened(1);
        }}
      >
        opener
      </button>
      {opened >= 1 ? (
        <ModalDialog
          label="First"
          heading="First"
          onClose={() => {
            setOpened(0);
          }}
          footer={
            <>
              <button
                type="button"
                onClick={() => {
                  setOpened(0);
                }}
              >
                Cancel
              </button>
              <button
                type="button"
                {...arrivalMark}
                onClick={() => {
                  setOpened(0);
                }}
              >
                Save
              </button>
            </>
          }
        >
          <button
            type="button"
            onClick={() => {
              setOpened(2);
            }}
          >
            Deeper
          </button>
          {Array.from({ length: 60 }, (_, line) => (
            <p key={line}>A line of what the dialog says, {line}.</p>
          ))}
          {opened === 2 ? (
            <ModalDialog
              label="Second"
              heading="Second"
              onClose={() => {
                setOpened(1);
              }}
              footer={
                <button
                  type="button"
                  {...arrivalMark}
                  onClick={() => {
                    setOpened(1);
                  }}
                >
                  Done
                </button>
              }
            >
              <p>The second dialog.</p>
            </ModalDialog>
          ) : null}
        </ModalDialog>
      ) : null}
    </main>
  );
}

/** Opens the first dialog by pressing opener as the way named does, its focus on opener first. */
async function openFirst(way: "mouse" | "touch" | "keyboard" | "remote"): Promise<HTMLElement> {
  const opener = page.getByRole("button", { name: "opener" }).element();
  if (!(opener instanceof HTMLElement)) {
    throw new Error("no opener");
  }
  if (way === "mouse") {
    await userEvent.click(opener);
  } else if (way === "touch") {
    // A tap focuses the button it lands on, then clicks it.
    opener.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true, pointerType: "touch" }));
    opener.focus();
    opener.click();
  } else {
    opener.focus();
    // Firefox now and then drops the click of an Enter on a button (idiosyncrasies.md): the click
    // itself, after a key, as the browser's own would be.
    await userEvent.keyboard("{Shift}");
    opener.click();
  }
  await expect.poll(() => seenDialogs().map((dialog) => dialog.ariaLabel)).toEqual(["First"]);
  return opener;
}

/** The open dialog's parts and their boxes, once it has risen, on a phone, from the screen's bottom. */
async function risenParts(): Promise<{
  dialog: HTMLDialogElement;
  head: DOMRect;
  foot: DOMRect;
  body: HTMLElement;
  box: DOMRect;
}> {
  const dialog = document.querySelector<HTMLDialogElement>("dialog[open]");
  if (dialog === null) {
    throw new Error("no dialog is open");
  }
  await Promise.all(dialog.getAnimations().map((animation) => animation.finished));
  const head = dialog.querySelector("header");
  const foot = dialog.querySelector("footer");
  const body = dialog.querySelector<HTMLElement>(".modal-dialog__body");
  if (head === null || foot === null || body === null) {
    throw new Error("the dialog's parts are not drawn");
  }
  return {
    dialog,
    head: head.getBoundingClientRect(),
    foot: foot.getBoundingClientRect(),
    body,
    box: dialog.getBoundingClientRect(),
  };
}

/**
 * A sheet from the screen's bottom, the screen's width, under 36rem (D538 §2): from its left edge to
 * its right, or to a desktop browser's scrollbar gutter, kept while the page under a dialog does not
 * scroll (a phone's scrollbar lies over the page); else centred, no wider than 26rem.
 */
function expectSizedFor(width: number, box: DOMRect): void {
  if (width < 36 * 16) {
    expect(Math.round(box.left)).toBe(0);
    expect(box.width).toBeGreaterThanOrEqual(width - 16);
    expect(Math.round(box.bottom)).toBe(window.innerHeight);
    return;
  }
  expect(box.width).toBeLessThanOrEqual(26 * 16);
  expect(box.left).toBeGreaterThan(0);
}

describe("a dialog of parts, at each width", () => {
  for (const width of widths) {
    it(`keeps its heading and its foot in view while its body scrolls, at ${String(width)}px`, async () => {
      await page.viewport(width, 720);
      await mount(withServices(<PageWithParts />));
      await openFirst("mouse");
      const { dialog, head, foot, body, box } = await risenParts();
      expect(body.scrollHeight).toBeGreaterThan(body.clientHeight);
      body.scrollTop = body.scrollHeight;
      expect(head.top).toBeGreaterThanOrEqual(box.top);
      expect(foot.bottom).toBeLessThanOrEqual(box.bottom + 1);
      expect(foot.bottom).toBeLessThanOrEqual(window.innerHeight + 1);
      expect(dialog.querySelector("header")?.getBoundingClientRect().top).toBe(head.top);
      expectSizedFor(width, box);
      // Focus arrives on its main action (D538 §4).
      expect(document.activeElement?.textContent).toBe("Save");
    });
  }
});

describe("a dialog opened from a dialog replaces it (D538 §5)", () => {
  for (const way of ["mouse", "touch", "keyboard", "remote"] as const) {
    it(`shows one dialog at a time, and brings the first back with focus on Deeper, by ${way}`, async () => {
      await page.viewport(1280, 720);
      await mount(withServices(<PageWithParts />));
      await openFirst(way);
      const deeper = page.getByRole("button", { name: "Deeper" }).element();
      if (!(deeper instanceof HTMLElement)) {
        throw new Error("no Deeper");
      }
      if (way === "remote") {
        // Save has focus; the arrows reach Deeper, at the dialog's top.
        for (let press = 0; press < 3 && document.activeElement !== deeper; press += 1) {
          await userEvent.keyboard("{ArrowUp}");
          expect(isFocusOnSomething()).toBe(true);
        }
      } else {
        deeper.focus();
      }
      if (way === "mouse") {
        await userEvent.click(deeper);
      } else {
        deeper.click();
      }
      await expect.poll(() => seenDialogs().map((dialog) => dialog.ariaLabel)).toEqual(["Second"]);
      expect(document.querySelectorAll("dialog[open]")).toHaveLength(2);
      expect(document.activeElement?.textContent).toBe("Done");
      if (way === "mouse" || way === "touch") {
        pressOutside(way);
      } else if (way === "keyboard") {
        await userEvent.keyboard("{Escape}");
      } else {
        await userEvent.keyboard("{Backspace}");
      }
      await expect.poll(() => seenDialogs().map((dialog) => dialog.ariaLabel)).toEqual(["First"]);
      await expect.poll(() => document.activeElement).toBe(deeper);
    });
  }
});

describe("a press outside a dialog closes it (D538 §3)", () => {
  for (const pointerType of ["mouse", "touch"] as const) {
    it(`closes on a ${pointerType}'s press outside it, focus back on its opener`, async () => {
      await page.viewport(360, 720);
      await mount(withServices(<PageWithParts />));
      const opener = await openFirst(pointerType);
      pressOutside(pointerType);
      await expect.poll(() => document.querySelector("dialog")).toBeNull();
      expect(document.activeElement).toBe(opener);
    });
  }
});

describe("a dialog by the keyboard and by a remote", () => {
  it("moves by Tab among its own controls with the ring showing, and Escape closes it", async () => {
    await page.viewport(1920, 1080);
    await mount(withServices(<PageWithParts />));
    const opener = await openFirst("keyboard");
    // From Save, its main action, back by Shift+Tab: Cancel, then Deeper; its body is no stop.
    for (const expected of ["Cancel", "Deeper"]) {
      await userEvent.keyboard("{Shift>}{Tab}{/Shift}");
      expect(document.activeElement?.textContent).toBe(expected);
      expect(ringShows(document.activeElement)).toBe(true);
    }
    await userEvent.keyboard("{Escape}");
    await expect.poll(() => document.querySelector("dialog")).toBeNull();
    expect(document.activeElement).toBe(opener);
  });

  it("moves by the arrows alone, OK presses, the ring showing, and focus is never on nothing", async () => {
    await page.viewport(1920, 1080);
    await mount(withServices(<PageWithParts />));
    const opener = await openFirst("remote");
    await userEvent.keyboard("{ArrowLeft}");
    expect(document.activeElement?.textContent).toBe("Cancel");
    expect(ringShows(document.activeElement)).toBe(true);
    await userEvent.keyboard("{ArrowRight}{ArrowDown}");
    expect(isFocusOnSomething()).toBe(true);
    (document.activeElement as HTMLElement | null)?.click();
    await expect.poll(() => document.querySelector("dialog")).toBeNull();
    expect(document.activeElement).toBe(opener);
  });
});

describe("a confirm (D538 §4)", () => {
  for (const width of widths) {
    it(`starts on its least harmful choice, read out as an alert with what it says, at ${String(width)}px`, async () => {
      await page.viewport(width, 720);
      await mount(
        withServices(
          <ConfirmDialog
            heading="Remove the folder?"
            consequence="It is never scanned again."
            confirmLabel="Remove"
            option="Also forget what was watched"
            onConfirm={() => undefined}
            onClose={() => undefined}
          />,
        ),
      );
      const dialog = document.querySelector("dialog[open]");
      expect(dialog?.getAttribute("role")).toBe("alertdialog");
      const describedBy = dialog?.getAttribute("aria-describedby") ?? "";
      expect(document.getElementById(describedBy)?.textContent).toContain("never scanned again");
      await expect.poll(() => document.activeElement?.textContent).toBe("Keep it");
      expect(dialog?.querySelector("footer")?.textContent).toBe("Keep itRemove");
      if (width >= 36 * 16) {
        expect(dialog?.getBoundingClientRect().width).toBeLessThanOrEqual(24 * 16);
      }
    });
  }
});

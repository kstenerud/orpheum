// SPDX-License-Identifier: MPL-2.0

/**
 * The People and System pages' dialogs as a real browser runs them, by the dialogs' rules
 * (dialogs-spec, D538 §2–§4): Invite someone starts on its one field, Back up now on its main action,
 * Create backup, not the box above it; Escape, a press outside and Cancel close each, focus back on
 * the button that opened it; a click, a tap, the keyboard and a remote's arrows and OK reach them
 * alike, the ring showing; and each stays inside the screen at each width.
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { useState, type JSX } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { startInput, type Input } from "./input-commands.ts";
import { InviteDialog } from "./people-dialogs.tsx";
import { SystemBackups } from "./system-backups.tsx";
import { mount, servedBy, settleAnswers, type Mounted } from "./test-dom.tsx";
import { focusedName, isRingShown, partWidths, tapOn } from "./test-parts.ts";
import { withServices } from "./test-services.tsx";

let input: Input | undefined;
let mounted: Mounted | undefined;

/** The People page's Invite someone, which opens its dialog. */
function Inviting(): JSX.Element {
  const [isOpen, setIsOpen] = useState(false);
  const close = (): void => {
    setIsOpen(false);
  };
  return (
    <main>
      <button
        type="button"
        onClick={() => {
          setIsOpen(true);
        }}
      >
        Invite someone
      </button>
      {isOpen ? (
        <InviteDialog
          choices={{ roleNames: ["user"], roles: undefined }}
          onClose={close}
          onInvited={close}
        />
      ) : null}
    </main>
  );
}

/** The button named text, wherever it shows. */
function button(text: string): HTMLElement {
  const found = [...document.querySelectorAll<HTMLElement>("button")].find(
    (each) => each.textContent.trim() === text,
  );
  if (found === undefined) {
    throw new Error(`no button ${text}`);
  }
  return found;
}

/** The open dialog, if one is. */
function dialog(): HTMLDialogElement | null {
  return document.querySelector<HTMLDialogElement>("dialog[open]");
}

/** A press on the backdrop, outside the dialog's box. */
function pressOutside(): void {
  dialog()?.dispatchEvent(new MouseEvent("click", { bubbles: true, clientX: 2, clientY: 2 }));
}

beforeEach(async () => {
  input = startInput({ goBack: () => false, isNavigating: () => true });
  await page.viewport(1280, 800);
});

afterEach(() => {
  input?.stop();
  input = undefined;
  mounted?.unmount();
  mounted = undefined;
});

describe("People's Invite someone", () => {
  beforeEach(async () => {
    mounted = await mount(withServices(<Inviting />, servedBy({})));
  });

  it("starts on its field at a click, and Escape gives focus back to Invite someone", async () => {
    await userEvent.click(button("Invite someone"));
    expect(document.activeElement?.closest("dialog") === dialog()).toBe(true);
    expect(document.activeElement?.tagName).toBe("INPUT");
    await userEvent.keyboard("{Escape}");
    expect(dialog()).toBeNull();
    expect(document.activeElement).toBe(button("Invite someone"));
  });

  it("opens at a tap and closes at a press outside", async () => {
    tapOn(button("Invite someone"));
    await expect.poll(dialog).not.toBeNull();
    pressOutside();
    await expect.poll(dialog).toBeNull();
  });

  it("is walked by a remote: OK opens it on its field, ringed, Down moves on from it, focus never on nothing", async () => {
    button("Invite someone").focus();
    await userEvent.keyboard("{Enter}");
    expect(document.activeElement?.tagName).toBe("INPUT");
    expect(isRingShown()).toBe(true);
    // Down moves on from the field, never onto nothing. (Firefox stops Down at the role chooser's
    // "Can watch", which Chromium passes on to Make their link: a question for the owner.)
    await userEvent.keyboard("{ArrowDown}");
    expect(document.activeElement).not.toBe(document.body);
    expect(document.activeElement?.tagName).not.toBe("INPUT");
    expect(isRingShown()).toBe(true);
  });

  it("stays inside the screen at each width", async () => {
    for (const width of partWidths) {
      await page.viewport(width, 800);
      button("Invite someone").focus();
      await userEvent.keyboard("{Enter}");
      const box = dialog()?.getBoundingClientRect();
      expect(box?.left, `${String(width)} px`).toBeGreaterThanOrEqual(0);
      expect(box?.right, `${String(width)} px`).toBeLessThanOrEqual(width);
      await userEvent.keyboard("{Escape}");
    }
  });
});

describe("System's Back up now", () => {
  beforeEach(async () => {
    mounted = await mount(
      withServices(
        <SystemBackups />,
        servedBy({ "GET /backups": [200, { directory: "/srv/orpheum/backups", backups: [] }] }),
      ),
    );
    await settleAnswers();
  });

  it("starts on Create backup at a click, Cancel at its foot before it, and Cancel gives focus back", async () => {
    await userEvent.click(button("Back up now"));
    expect(focusedName()).toBe("Create backup");
    expect(
      [...(dialog()?.querySelectorAll(".modal-dialog__foot button") ?? [])].map(
        (each) => each.textContent,
      ),
    ).toEqual(["Cancel", "Create backup"]);
    await userEvent.click(button("Cancel"));
    expect(dialog()).toBeNull();
    expect(document.activeElement).toBe(button("Back up now"));
  });

  it("opens at a tap and closes at a press outside", async () => {
    tapOn(button("Back up now"));
    await expect.poll(dialog).not.toBeNull();
    pressOutside();
    await expect.poll(dialog).toBeNull();
  });

  it("is walked by a remote: OK opens it on Create backup, Up reaches the box, ringed, Escape leaves it", async () => {
    button("Back up now").focus();
    await userEvent.keyboard("{Enter}");
    expect(focusedName()).toBe("Create backup");
    await userEvent.keyboard("{ArrowUp}");
    expect(document.activeElement?.getAttribute("type")).toBe("checkbox");
    // The ring shows once an arrow has moved focus; Firefox does not draw it on Create backup as the
    // dialog opens on it, Chromium does (a question for the owner).
    expect(isRingShown()).toBe(true);
    await userEvent.keyboard("{Escape}");
    expect(dialog()).toBeNull();
    expect(document.activeElement).toBe(button("Back up now"));
  });

  it("stays inside the screen at each width", async () => {
    for (const width of partWidths) {
      await page.viewport(width, 800);
      button("Back up now").focus();
      await userEvent.keyboard("{Enter}");
      const box = dialog()?.getBoundingClientRect();
      expect(box?.left, `${String(width)} px`).toBeGreaterThanOrEqual(0);
      expect(box?.right, `${String(width)} px`).toBeLessThanOrEqual(width);
      expect(button("Create backup").getBoundingClientRect().height).toBeGreaterThanOrEqual(44);
      await userEvent.keyboard("{Escape}");
    }
  });
});

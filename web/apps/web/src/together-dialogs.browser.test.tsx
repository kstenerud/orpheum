// SPDX-License-Identifier: MPL-2.0

/**
 * The tray's Watch Together dialogs (together-dialog.tsx, together-people.tsx) as a real browser runs
 * them, by the dialogs' rules (dialogs-spec, D538 §2–§4): Invite someone replaces the dialog it opens
 * from and starts on its one field, the ring showing; Cancel, Escape and a press outside close it,
 * focus back on Invite someone in the dialog under it; a click, a tap, the keyboard and a remote's
 * arrows and OK reach it alike; and it stays inside the screen at each width.
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { startInput, type Input } from "./input-commands.ts";
import { mount, servedBy, settleAnswers, type Mounted } from "./test-dom.tsx";
import { focusedName, isRingShown, partWidths, tapOn } from "./test-parts.ts";
import { withServices } from "./test-services.tsx";
import { TogetherDialog } from "./together-dialog.tsx";

const karl = { id: 1, name: "Karl", isHost: true };
const allRights = { canPlayPause: true, canSeek: true, canNext: true };

let input: Input | undefined;
let mounted: Mounted | undefined;

beforeEach(async () => {
  window.sessionStorage.clear();
  window.sessionStorage.setItem(
    "orpheum.together.host",
    JSON.stringify({ sessionId: 3, credential: "host-credential" }),
  );
  input = startInput({ goBack: () => false, isNavigating: () => true });
  await page.viewport(1280, 800);
  const services = servedBy(
    {
      "GET /together/3": [200, { sessionId: 3, nextRights: allRights, links: [], members: [karl] }],
    },
    [],
    "token",
  );
  mounted = await mount(withServices(<TogetherDialog onClose={() => undefined} />, services));
  await settleAnswers();
});

afterEach(() => {
  input?.stop();
  input = undefined;
  mounted?.unmount();
  mounted = undefined;
  window.sessionStorage.clear();
});

/** The button named text in the dialog shown. */
function button(text: string): HTMLElement {
  const found = [
    ...document.querySelectorAll<HTMLElement>("dialog[open]:not([data-replaced]) button"),
  ].find((each) => each.textContent.trim() === text);
  if (found === undefined) {
    throw new Error(`no button ${text}`);
  }
  return found;
}

/** The heading of the dialog shown, the one no other replaces. */
function shownHeading(): string | undefined {
  return (
    document.querySelector("dialog[open]:not([data-replaced]) .modal-dialog__head")?.textContent ??
    undefined
  );
}

/** A press on the backdrop, outside the shown dialog's box. */
function pressOutside(): void {
  document
    .querySelector("dialog[open]:not([data-replaced])")
    ?.dispatchEvent(new MouseEvent("click", { bubbles: true, clientX: 2, clientY: 2 }));
}

/** Whether focus is on the field of Invite someone. */
function isOnTheField(): boolean {
  return (document.activeElement?.closest(".watch-together__make-link input") ?? null) !== null;
}

describe("Watch Together's Invite someone, by every way of pressing it", () => {
  it("replaces Watch Together at a click, starting on its field, and Cancel gives focus back to Invite someone", async () => {
    await userEvent.click(button("Invite someone"));
    expect(shownHeading()).toBe("Invite someone");
    expect(document.querySelectorAll("dialog[open]:not([data-replaced])")).toHaveLength(1);
    expect(isOnTheField()).toBe(true);
    await userEvent.click(button("Cancel"));
    expect(shownHeading()).toBe("Watch Together");
    expect(document.activeElement).toBe(button("Invite someone"));
  });

  it("opens at a tap, and a press outside closes it, focus back on Invite someone", async () => {
    tapOn(button("Invite someone"));
    await expect.poll(shownHeading).toBe("Invite someone");
    pressOutside();
    await expect.poll(shownHeading).toBe("Watch Together");
    // A tap gives its button no focus, so focus goes back where it was, in the dialog under it.
    expect(document.activeElement?.closest("dialog[open]:not([data-replaced])")).not.toBeNull();
  });

  it("is walked by a keyboard: Enter opens it on its field, ringed, and Escape leaves it", async () => {
    button("Invite someone").focus();
    await userEvent.keyboard("{Enter}");
    expect(isOnTheField()).toBe(true);
    expect(isRingShown()).toBe(true);
    await userEvent.keyboard("{Escape}");
    expect(shownHeading()).toBe("Watch Together");
    expect(document.activeElement).toBe(button("Invite someone"));
  });

  it("is reached by a remote's arrows and OK, its foot's Cancel by Down and Make a link by Right, focus never on nothing", async () => {
    button("Invite someone").focus();
    await userEvent.keyboard("{Enter}");
    expect(isOnTheField()).toBe(true);
    for (let press = 0; press < 6 && focusedName() !== "Cancel"; press += 1) {
      await userEvent.keyboard("{ArrowDown}");
      expect(document.activeElement).not.toBe(document.body);
    }
    expect(focusedName()).toBe("Cancel");
    await userEvent.keyboard("{ArrowRight}");
    expect(focusedName()).toBe("Make a link");
    expect(isRingShown()).toBe(true);
  });

  it("stays inside the screen at each width, its choices a fingertip tall", async () => {
    for (const width of partWidths) {
      await page.viewport(width, 800);
      button("Invite someone").focus();
      await userEvent.keyboard("{Enter}");
      const box = document
        .querySelector("dialog[open]:not([data-replaced])")
        ?.getBoundingClientRect();
      expect(box?.left, `${String(width)} px`).toBeGreaterThanOrEqual(0);
      expect(box?.right, `${String(width)} px`).toBeLessThanOrEqual(width);
      expect(button("Make a link").getBoundingClientRect().height).toBeGreaterThanOrEqual(44);
      await userEvent.keyboard("{Escape}");
    }
  });
});

// SPDX-License-Identifier: MPL-2.0

/**
 * A help button walked by a mouse, a finger, a keyboard and a remote's keys in a real browser, at a
 * phone's, a computer's and a television's width (D538, interface rule 3; help-button.tsx): it
 * opens its help in a pop-up at it and closes it again, focus never on nothing and back on the
 * button once closed; nothing opens on a hover.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { HelpButton } from "./help-button.tsx";
import { startInput, type Input } from "./input-commands.ts";
import { mount, type Mounted } from "./test-dom.tsx";
import { isRingShown, partWidths, tapOn } from "./test-parts.ts";

const help = "Devices outside the house reach this server on this port.";

let input: Input | undefined;
let mounted: Mounted | undefined;

beforeEach(async () => {
  input = startInput({ goBack: () => false, isNavigating: () => true });
  await page.viewport(960, 600);
  mounted = await mount(
    <main>
      <div>
        <button type="button">Above</button>
      </div>
      <div>
        <HelpButton label="About the port" help={help} />
      </div>
    </main>,
  );
});

afterEach(() => {
  input?.stop();
  input = undefined;
  mounted?.unmount();
  mounted = undefined;
});

/** The help button. */
function opener(): HTMLElement {
  const found = document.querySelector<HTMLElement>('[aria-label="About the port"]');
  if (found === null) {
    throw new Error("no help button");
  }
  return found;
}

/** The help shown, if it is open. */
const shownHelp = (): Element | null | undefined =>
  document.querySelector("dialog[open]")?.querySelector(".help-button__note");

/** Waits for the help to open, focus inside it and on something. */
async function isOpened(): Promise<void> {
  await expect.poll(() => shownHelp()?.textContent).toBe(help);
  await expect.poll(() => document.activeElement?.closest("dialog[open]")).not.toBeNull();
  expect(document.activeElement).not.toBe(document.body);
  expect(opener().getAttribute("aria-expanded")).toBe("true");
}

/** Waits for the help to close, focus back on its button. */
async function isClosed(): Promise<void> {
  await expect.poll(() => document.querySelector("dialog[open]")).toBeNull();
  expect(document.activeElement).toBe(opener());
  expect(opener().getAttribute("aria-expanded")).toBe("false");
}

describe("a help button", () => {
  it("says what it is about and opens nothing on a hover", async () => {
    expect(opener().getAttribute("aria-haspopup")).toBe("dialog");
    expect(opener().hasAttribute("title")).toBe(false);
    await userEvent.hover(opener());
    expect(shownHelp()).toBeFalsy();
  });

  it("opens its help by a click and closes it by Escape, and by a press outside", async () => {
    await userEvent.click(opener());
    await isOpened();
    await userEvent.keyboard("{Escape}");
    await isClosed();
    await userEvent.click(opener());
    await isOpened();
    document
      .querySelector("dialog[open]")
      ?.dispatchEvent(new MouseEvent("click", { bubbles: true, clientX: 950, clientY: 590 }));
    await isClosed();
  });

  it("opens its help by a tap", async () => {
    // A tap focuses what it lands on, as a click does; tapOn's own click() does not.
    opener().focus();
    tapOn(opener());
    await isOpened();
    await userEvent.keyboard("{Escape}");
    await isClosed();
  });

  it("is reached by Tab with its ring and opened by Space, focus on the pop-up that holds the help", async () => {
    document.querySelector<HTMLButtonElement>("button")?.focus();
    await userEvent.keyboard("{Tab}");
    expect(document.activeElement).toBe(opener());
    expect(isRingShown()).toBe(true);
    await userEvent.keyboard(" ");
    await isOpened();
    expect(document.activeElement?.contains(shownHelp() ?? null)).toBe(true);
    await userEvent.keyboard("{Escape}");
    await isClosed();
  });

  it("is reached by a remote's Down and opened by OK, Back closing it", async () => {
    document.querySelector<HTMLButtonElement>("button")?.focus();
    await userEvent.keyboard("{ArrowDown}");
    expect(document.activeElement).toBe(opener());
    expect(isRingShown()).toBe(true);
    await userEvent.keyboard("{Enter}");
    await isOpened();
    await userEvent.keyboard("{Escape}");
    await isClosed();
  });

  it("lays out a fingertip square button, its help on the screen and no wider than reading, at each width", async () => {
    for (const width of partWidths) {
      await page.viewport(width, 800);
      const button = opener().getBoundingClientRect();
      expect(button.width, `${String(width)} px`).toBeGreaterThanOrEqual(44);
      expect(button.height).toBeGreaterThanOrEqual(44);
      await userEvent.click(opener());
      await isOpened();
      const note = shownHelp()?.getBoundingClientRect();
      expect(note?.left ?? -1).toBeGreaterThanOrEqual(0);
      expect(note?.right ?? width + 1).toBeLessThanOrEqual(width);
      await userEvent.keyboard("{Escape}");
      await isClosed();
    }
  });
});

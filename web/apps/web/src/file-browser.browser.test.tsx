// SPDX-License-Identifier: MPL-2.0

/**
 * The file browser's Choose as a real browser runs it (vitest's browser mode, D539 §1, §2, §4):
 * focus goes into it as it opens, stays in it, and returns to its opener when Escape closes it at
 * its places (D361 §3); the keys and a remote's arrows move among its rows, the focus ring showing,
 * Enter and OK open one, Escape goes up one folder onto the folder it came from, never onto nothing;
 * a tap opens a row; and its rows are a fingertip tall and inside the screen at each width.
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { ChooseOpener } from "./file-browser-choose.tsx";
import { startInput, type Input } from "./input-commands.ts";
import { mount, servedBy, settleAnswers, type Mounted } from "./test-dom.tsx";
import { focusedName, isRingShown, partWidths, tapOn } from "./test-parts.ts";
import { withServices } from "./test-services.tsx";

let input: Input | undefined;
let mounted: Mounted | undefined;

beforeEach(async () => {
  input = startInput({ goBack: () => false, isNavigating: () => true });
  await page.viewport(1280, 800);
  mounted = await mount(
    withServices(
      <main>
        <button type="button">behind</button>
        <ChooseOpener
          opener="Pick…"
          heading="Pick a folder"
          places={[
            { nodeName: "nas", path: "/tank/media" },
            { nodeName: "attic", path: "/films" },
          ]}
          onChoose={() => undefined}
        />
      </main>,
      servedBy({
        "GET /nodes/nas/folders": [
          [200, { path: "/tank/media", state: "readable", folders: ["music", "video"] }],
          [200, { path: "/tank/media/video", state: "readable", folders: [] }],
          [200, { path: "/tank/media", state: "readable", folders: ["music", "video"] }],
        ],
      }),
    ),
  );
});

afterEach(() => {
  input?.stop();
  input = undefined;
  mounted?.unmount();
  mounted = undefined;
});

/** The button that opens Choose. */
function opener(): HTMLElement {
  const found = [...document.querySelectorAll<HTMLElement>("button")].find(
    (button) => button.textContent === "Pick…",
  );
  if (found === undefined) {
    throw new Error("no opener");
  }
  return found;
}

/** The row named name. */
function row(name: string): HTMLElement {
  const found = document.querySelector<HTMLElement>(`.file-browser__row[aria-label="${name}"]`);
  if (found === null) {
    throw new Error(`no row ${name}`);
  }
  return found;
}

/** Whether focus is on something in the dialog, never on nothing. */
function isFocusInTheDialog(): boolean {
  return document.activeElement?.closest("dialog") !== null && document.activeElement !== null;
}

describe("the file browser's Choose, in a real browser", () => {
  it("holds focus while open, and gives it back to its opener when Escape closes it at its places", async () => {
    await userEvent.click(opener());
    expect(isFocusInTheDialog()).toBe(true);
    await userEvent.keyboard("{Tab}{Tab}{Tab}{Tab}");
    expect(document.activeElement?.textContent).not.toBe("behind");
    await userEvent.keyboard("{Escape}");
    expect(document.querySelector("dialog")).toBeNull();
    expect(document.activeElement).toBe(opener());
  });

  it("moves among its rows by the arrows, opens one by Enter, and goes up by Escape onto it", async () => {
    await userEvent.click(opener());
    // Choose opens on its first row (D539 §4), not on the path bar's Go to… above it.
    await expect.poll(focusedName).toBe("films");
    await userEvent.keyboard("{ArrowDown}");
    expect(focusedName()).toBe("media");
    await userEvent.keyboard("{Enter}");
    await settleAnswers();
    expect(focusedName()).toBe("music");
    await userEvent.keyboard("{ArrowDown}");
    expect(focusedName()).toBe("video");
    expect(isRingShown()).toBe(true);
    await userEvent.keyboard("{Enter}");
    await settleAnswers();
    // A folder with nothing in it to press puts focus on the dialog's main button, never on nothing.
    expect(isFocusInTheDialog()).toBe(true);
    expect(document.activeElement?.textContent).toContain("Use video");
    expect(document.querySelector("dialog")?.textContent).toContain("No folders in video");
    // Escape goes up one folder, the dialog staying open, focus on the folder it came from.
    await userEvent.keyboard("{Escape}");
    await settleAnswers();
    expect(document.querySelector("dialog")).not.toBeNull();
    expect(focusedName()).toBe("video");
    expect(isRingShown()).toBe(true);
  });

  it("opens a row by a tap", async () => {
    await userEvent.click(opener());
    tapOn(row("media"));
    await settleAnswers();
    expect(row("music")).toBeDefined();
    expect(isFocusInTheDialog()).toBe(true);
  });

  it("draws its rows a fingertip tall, inside the screen, at each width", async () => {
    await userEvent.click(opener());
    for (const width of partWidths) {
      await page.viewport(width, 800);
      const box = row("media").getBoundingClientRect();
      expect(box.height, `${String(width)} px`).toBeGreaterThanOrEqual(56);
      expect(box.right, `${String(width)} px`).toBeLessThanOrEqual(width);
      const dialog = document.querySelector("dialog")?.getBoundingClientRect();
      expect(dialog?.right ?? width + 1, `${String(width)} px`).toBeLessThanOrEqual(width);
    }
  });
});

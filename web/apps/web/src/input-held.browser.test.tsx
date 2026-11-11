// SPDX-License-Identifier: MPL-2.0

/**
 * A held arrow in a real browser whose frames come slower than the key repeats (the owner,
 * 2026-11-11): focus moves once a drawn frame, stops at the release, and a first press always moves;
 * an arrow something answers, as the player's held seek, hears every repeat.
 */

import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { page } from "vitest/browser";

import { onCommand, startInput, type CommandDetail, type Input } from "./input-commands.ts";

/** A column of ten buttons in the page's content. */
function columnOfButtons(): HTMLElement {
  const main = document.createElement("main");
  for (let index = 0; index < 10; index += 1) {
    const button = document.createElement("button");
    button.type = "button";
    button.textContent = `button ${String(index)}`;
    button.style.cssText = "display: block; height: 30px";
    main.append(button);
  }
  document.body.append(main);
  return main;
}

/** A keydown of Down on what has focus, a repeat or the press. */
function down(isRepeat: boolean): void {
  const event = new KeyboardEvent("keydown", {
    key: "ArrowDown",
    keyCode: 40,
    repeat: isRepeat,
    bubbles: true,
    cancelable: true,
  });
  (document.activeElement ?? document.body).dispatchEvent(event);
}

/** Down's keyup. */
function released(): void {
  document.dispatchEvent(
    new KeyboardEvent("keyup", { key: "ArrowDown", keyCode: 40, bubbles: true }),
  );
}

/** The animation frames asked for, which the test draws by hand. */
let frames: FrameRequestCallback[] = [];

/** Draws the frames asked for so far, and lets the work after them run. */
async function frameDrawn(): Promise<void> {
  const drawing = frames;
  frames = [];
  for (const frame of drawing) {
    frame(0);
  }
  await new Promise((resolve) => setTimeout(resolve, 0));
}

let input: Input | undefined;

beforeEach(async () => {
  await page.viewport(960, 480);
  frames = [];
  vi.spyOn(window, "requestAnimationFrame").mockImplementation((frame) => {
    frames.push(frame);
    return frames.length;
  });
  input = startInput({ goBack: () => false, isNavigating: () => true });
});

afterEach(() => {
  input?.stop();
  input = undefined;
  vi.restoreAllMocks();
  document.body.replaceChildren();
});

describe("a held arrow while frames come slowly", () => {
  it("moves once a drawn frame, stops at its release, and a new press moves at once", async () => {
    const main = columnOfButtons();
    main.querySelector("button")?.focus();
    down(false);
    for (let repeat = 0; repeat < 5; repeat += 1) {
      down(true);
    }
    expect(document.activeElement?.textContent).toBe("button 1");
    await frameDrawn();
    for (let repeat = 0; repeat < 5; repeat += 1) {
      down(true);
    }
    expect(document.activeElement?.textContent).toBe("button 2");
    released();
    down(false);
    expect(document.activeElement?.textContent).toBe("button 3");
  });

  it("offers every repeat to what answers the arrow, as the player's held seek", () => {
    const main = columnOfButtons();
    const first = main.querySelector("button");
    first?.focus();
    const heard: CommandDetail[] = [];
    const stop = onCommand(main, (detail, event) => {
      heard.push(detail);
      event.preventDefault();
    });
    try {
      down(false);
      for (let repeat = 0; repeat < 5; repeat += 1) {
        down(true);
      }
      expect(heard).toHaveLength(6);
      expect(document.activeElement).toBe(first);
    } finally {
      stop();
    }
  });
});

// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it } from "vitest";

import { driveFocus, geometricFocus, moveFocus } from "./lab-focus.ts";

/** A region of three buttons in the document, and a field outside it. */
function regionOfButtons(): { readonly region: HTMLElement; readonly field: HTMLInputElement } {
  const region = document.createElement("div");
  for (const name of ["first", "second", "third"]) {
    const button = document.createElement("button");
    button.textContent = name;
    region.append(button);
  }
  const field = document.createElement("input");
  document.body.append(region, field);
  return { region, field };
}

/** Presses key on whatever has focus, as a remote's press reaches the page. */
function press(key: string): KeyboardEvent {
  const event = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true });
  (document.activeElement ?? document.body).dispatchEvent(event);
  return event;
}

describe("the input probe's focus driving", () => {
  afterEach(() => {
    document.body.replaceChildren();
  });

  it("focuses the region's first element on an arrow while nothing in it has focus", () => {
    const { region } = regionOfButtons();
    const stop = driveFocus(region, geometricFocus);
    try {
      const event = press("ArrowDown");
      expect(document.activeElement?.textContent).toBe("first");
      expect(event.defaultPrevented).toBe(true);
    } finally {
      stop();
    }
  });

  it("leaves an arrow in a field outside the region to the field", () => {
    const { region, field } = regionOfButtons();
    field.focus();
    const stop = driveFocus(region, geometricFocus);
    try {
      const event = press("ArrowLeft");
      expect(document.activeElement).toBe(field);
      expect(event.defaultPrevented).toBe(false);
    } finally {
      stop();
    }
  });

  it("leaves an arrow to a text field inside the region while it composes, even at the value's end", () => {
    const { region } = regionOfButtons();
    const field = document.createElement("input");
    region.append(field);
    field.focus();
    const stop = driveFocus(region, geometricFocus);
    try {
      const composing = new KeyboardEvent("keydown", {
        key: "ArrowLeft",
        isComposing: true,
        bubbles: true,
        cancelable: true,
      });
      field.dispatchEvent(composing);
      expect(composing.defaultPrevented).toBe(false);
      expect(press("ArrowLeft").defaultPrevented).toBe(true);
    } finally {
      stop();
    }
  });

  it("hears no arrow once stopped", () => {
    const { region } = regionOfButtons();
    driveFocus(region, geometricFocus)();
    press("ArrowDown");
    expect(document.activeElement).toBe(document.body);
  });

  it("moves focus for a controller so that the browser shows it, as it shows a key's", () => {
    // A gamepad's press is no key, so a browser draws no :focus-visible after it unless asked
    // (the owner, Firefox on a Mac, 2026-10-25).
    const { region } = regionOfButtons();
    const first = region.querySelector("button");
    if (first === null) {
      throw new Error("the region has no button");
    }
    const asked: unknown[] = [];
    first.focus = (options?: FocusOptions): void => {
      asked.push(options);
    };
    moveFocus({ region, engine: geometricFocus, start: region }, "down");
    expect(asked).toEqual([{ focusVisible: true }]);
  });
});

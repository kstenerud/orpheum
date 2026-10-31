// SPDX-License-Identifier: MPL-2.0

// @vitest-environment happy-dom
import { afterEach, describe, expect, it } from "vitest";

import { guardFormsOk } from "./form-ok.ts";

/** A form with a name field, a checkbox and its submit, its submits counted; and the guard's notes. */
function formGuarded(): {
  readonly box: HTMLInputElement;
  readonly field: HTMLInputElement;
  readonly submits: () => number;
  readonly notes: string[];
  readonly stop: () => void;
} {
  document.body.innerHTML =
    '<form><input type="text" name="name"><label><input type="checkbox"> Remember</label><button type="submit">Sign in</button></form>';
  const form = document.querySelector("form");
  const box = document.querySelector<HTMLInputElement>('input[type="checkbox"]');
  const field = document.querySelector<HTMLInputElement>('input[type="text"]');
  if (form === null || box === null || field === null) {
    throw new Error("the form was not drawn");
  }
  let submits = 0;
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    submits += 1;
  });
  const notes: string[] = [];
  const stop = guardFormsOk({ target: window, focused: () => document.activeElement }, (line) =>
    notes.push(line),
  );
  return { box, field, submits: () => submits, notes, stop };
}

/** Sends a key event of type for Enter at what has focus; whether its default was prevented. */
function enter(type: "keydown" | "keypress" | "keyup", keyCode = 13): boolean {
  const event = new KeyboardEvent(type, { key: "Enter", keyCode, bubbles: true, cancelable: true });
  (document.activeElement ?? document.body).dispatchEvent(event);
  return event.defaultPrevented;
}

/** Submits the form as a browser's implicit submission does; whether the form's own handler heard it. */
function submitFromFocus(): boolean {
  let isHeard = false;
  const form = document.querySelector("form");
  const hear = (): void => {
    isHeard = true;
  };
  form?.addEventListener("submit", hear);
  form?.dispatchEvent(new Event("submit", { bubbles: true, cancelable: true }));
  form?.removeEventListener("submit", hear);
  return isHeard;
}

/** Waits until a key up's own default is done. */
async function afterKeyUp(): Promise<void> {
  await new Promise((resolve) => setTimeout(resolve, 0));
}

let stopGuard: (() => void) | undefined;

afterEach(() => {
  stopGuard?.();
  stopGuard = undefined;
  document.body.innerHTML = "";
});

// The owner's Fire TV, 2026-10-30: "As soon as I select the checkbox, it acts as though I'd pressed
// the log in button and attempts a login."
describe("OK in a form (form-ok.ts)", () => {
  it("ticks a checkbox on OK and stops every submit until the key is up, its key press and key up kept from the browser", async () => {
    const shown = formGuarded();
    stopGuard = shown.stop;
    shown.box.focus();
    expect(enter("keydown")).toBe(true);
    expect(shown.box.checked).toBe(true);
    // Silk's held OK: more key downs, unmarked as repeats, with no key up between.
    enter("keydown");
    enter("keydown");
    expect(shown.box.checked).toBe(true);
    expect(enter("keypress")).toBe(true);
    expect(submitFromFocus()).toBe(false);
    expect(enter("keyup")).toBe(true);
    // What the key up does by default, before the guard lets go.
    expect(submitFromFocus()).toBe(false);
    await afterKeyUp();
    expect(shown.submits()).toBe(0);
    expect(shown.notes).toEqual([
      "a form's OK on a checkbox input: keydown Enter/13, ticked, keydown Enter/13, keydown Enter/13, keypress Enter/13, stopped, submit, stopped, keyup Enter/13, stopped, submit, stopped",
    ]);
    // A press of its own afterwards unticks it, and the form's Sign in still submits.
    enter("keydown");
    enter("keyup");
    await afterKeyUp();
    expect(shown.box.checked).toBe(false);
    expect(submitFromFocus()).toBe(true);
    expect(shown.submits()).toBe(1);
  });

  it("takes a TV's D-pad centre for OK as well", async () => {
    const shown = formGuarded();
    stopGuard = shown.stop;
    shown.box.focus();
    enter("keydown", 23);
    expect(shown.box.checked).toBe(true);
    expect(submitFromFocus()).toBe(false);
    enter("keyup", 23);
    await afterKeyUp();
    expect(shown.submits()).toBe(0);
  });

  it("leaves OK in a text field to the browser, which submits, and notes it", async () => {
    const shown = formGuarded();
    stopGuard = shown.stop;
    shown.field.focus();
    expect(enter("keydown")).toBe(false);
    expect(enter("keypress")).toBe(false);
    expect(submitFromFocus()).toBe(true);
    enter("keyup");
    await afterKeyUp();
    expect(shown.submits()).toBe(1);
    expect(shown.notes).toEqual([
      "a form's OK on a text input: keydown Enter/13, keypress Enter/13, submit, keyup Enter/13",
    ]);
  });

  it("ticks once where two guards run, as the loader's under the client's", async () => {
    const shown = formGuarded();
    const second = guardFormsOk(
      { target: window, focused: () => document.activeElement },
      () => undefined,
    );
    stopGuard = () => {
      shown.stop();
      second();
    };
    shown.box.focus();
    enter("keydown");
    expect(shown.box.checked).toBe(true);
    expect(submitFromFocus()).toBe(false);
    enter("keyup");
    await afterKeyUp();
    expect(shown.submits()).toBe(0);
  });
});

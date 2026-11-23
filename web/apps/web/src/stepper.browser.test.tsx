// SPDX-License-Identifier: MPL-2.0

/**
 * A stepper walked by a remote's keys, a keyboard, a mouse and a finger in a real browser, at a Fire
 * TV's page in Silk (the owner, 2026-11-22; D538 §9): Up and Down leave its row from the field as
 * from − and +, never stepping it; Left and Right move the field's caret, and past its ends go on to
 * − and +, which OK presses; at a bound the sign that can go no further keeps the focus.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";

import { useState, type JSX } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { startInput, type Input } from "./input-commands.ts";
import { Stepper } from "./stepper.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { focusedName, isRingShown, partWidths, tapOn } from "./test-parts.ts";

/** A button above the stepper, laid out as a menu's row has it, and a button below. */
function Column(): JSX.Element {
  const [value, setValue] = useState(100);
  return (
    <main>
      <button type="button">Above</button>
      <Stepper
        label="Delay"
        fieldLabel="Delay, in ms"
        valueText={`Delay: ${String(value)} ms`}
        value={value}
        min={-200}
        max={200}
        step={50}
        unit="ms"
        stepDownLabel="50 ms less"
        stepUpLabel="50 ms more"
        onChange={setValue}
      />
      <button type="button">Below</button>
    </main>
  );
}

/**
 * A setting's stepper as Settings draws one: a long label, no unit, a caption wider than the control,
 * and a narrow range, its field the narrowest.
 */
function LongWorded(): JSX.Element {
  const [value, setValue] = useState(6);
  return (
    <main>
      <Stepper
        label="Most hours between a library's checks, when it is checked on a schedule"
        fieldLabel="Most hours, from 1 to 168"
        valueText={`Most hours between a library's checks: ${String(value)}`}
        value={value}
        min={1}
        max={168}
        step={1}
        unit=""
        stepDownLabel="One hour less"
        stepUpLabel="One hour more"
        onChange={setValue}
      />
    </main>
  );
}

/** The box of the element selector names, which must be on the page. */
function boxOf(selector: string): DOMRect {
  const found = document.querySelector(selector);
  if (found === null) {
    throw new Error(`no ${selector}`);
  }
  return found.getBoundingClientRect();
}

/** How wide text is drawn in font, a field's text being no element of its own to measure. */
function widthOfText(text: string, font: string): number {
  const context = document.createElement("canvas").getContext("2d");
  if (context === null) {
    throw new Error("no canvas to measure with");
  }
  context.font = font;
  return context.measureText(text).width;
}

/** How wide the field's widest value, its bound's, is drawn, a figure to each of its characters. */
function widestFigures(): number {
  const looks = getComputedStyle(field());
  return Math.max(widthOfText("−200", looks.font), widthOfText("168", looks.font));
}

let input: Input | undefined;
let mounted: Mounted | undefined;

beforeEach(async () => {
  await page.viewport(960, 480);
  input = startInput({ goBack: () => false, isNavigating: () => true });
  mounted = await mount(<Column />);
});

afterEach(() => {
  input?.stop();
  input = undefined;
  mounted?.unmount();
  mounted = undefined;
});

/** The stepper's field. */
function field(): HTMLInputElement {
  const found = document.querySelector("input");
  if (found === null) {
    throw new Error("no field");
  }
  return found;
}

/** The stepper's button named name. */
function sign(name: string): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(`button[aria-label="${name}"]`);
  if (found === null) {
    throw new Error(`no ${name}`);
  }
  return found;
}

/** Focuses the field, its caret at the value's end, as typing leaves it. */
function focusFieldAtEnd(): void {
  const { value } = field();
  field().focus();
  field().setSelectionRange(value.length, value.length);
}

/** Focuses the field, its caret at the value's start. */
function focusFieldAtStart(): void {
  field().focus();
  field().setSelectionRange(0, 0);
}

/** The stepper's caption. */
function caption(): HTMLOutputElement {
  const found = document.querySelector("output");
  if (found === null) {
    throw new Error("no caption");
  }
  return found;
}

describe("a stepper, by a remote's keys", () => {
  it("leaves its row by Up and Down from the field, never stepping it", async () => {
    focusFieldAtEnd();
    await userEvent.keyboard("{ArrowDown}");
    expect(focusedName()).toBe("Below");
    focusFieldAtEnd();
    await userEvent.keyboard("{ArrowUp}");
    expect(focusedName()).toBe("Above");
    expect(field().value).toBe("100");
    expect(caption().textContent).toBe("Delay: 100 ms");
  });

  it("moves the field's caret by Left and Right, and past its ends goes to − and + and back", async () => {
    focusFieldAtEnd();
    await userEvent.keyboard("{ArrowLeft}");
    expect(document.activeElement).toBe(field());
    expect(field().selectionStart).toBe(2);
    focusFieldAtEnd();
    await userEvent.keyboard("{ArrowRight}");
    expect(focusedName()).toBe("50 ms more");
    expect(isRingShown()).toBe(true);
    await userEvent.keyboard("{ArrowLeft}");
    expect(document.activeElement).toBe(field());
    focusFieldAtStart();
    await userEvent.keyboard("{ArrowLeft}");
    expect(focusedName()).toBe("50 ms less");
    expect(isRingShown()).toBe(true);
    await userEvent.keyboard("{ArrowRight}");
    expect(document.activeElement).toBe(field());
  });

  it("steps by OK on − and +, one step a press, and leaves their row by Up and Down", async () => {
    focusFieldAtEnd();
    await userEvent.keyboard("{ArrowRight}");
    await userEvent.keyboard("{Enter}");
    expect(field().value).toBe("150");
    focusFieldAtStart();
    await userEvent.keyboard("{ArrowLeft}");
    await userEvent.keyboard("{Enter}");
    await userEvent.keyboard("{Enter}");
    expect(field().value).toBe("50");
    expect(caption().textContent).toBe("Delay: 50 ms");
    await userEvent.keyboard("{ArrowDown}");
    expect(focusedName()).toBe("Below");
    await userEvent.keyboard("{ArrowUp}");
    await userEvent.keyboard("{ArrowUp}");
    expect(focusedName()).toBe("Above");
  });

  it("keeps the focus on + at its most, OK doing nothing more, so a television never loses it", async () => {
    focusFieldAtEnd();
    await userEvent.keyboard("{ArrowRight}");
    await userEvent.keyboard("{Enter}");
    await userEvent.keyboard("{Enter}");
    await userEvent.keyboard("{Enter}");
    expect(field().value).toBe("200");
    expect(document.activeElement).toBe(sign("50 ms more"));
    expect(sign("50 ms more").getAttribute("aria-disabled")).toBe("true");
    expect(isRingShown()).toBe(true);
    await userEvent.keyboard("{ArrowLeft}");
    expect(document.activeElement).toBe(field());
  });
});

describe("a stepper, by a keyboard, a mouse and a finger", () => {
  it("is reached by Tab, −, the field, then +, and Space presses a sign", async () => {
    document.querySelector<HTMLButtonElement>("button")?.focus();
    await userEvent.keyboard("{Tab}");
    expect(focusedName()).toBe("50 ms less");
    expect(isRingShown()).toBe(true);
    await userEvent.keyboard(" ");
    expect(field().value).toBe("50");
    await userEvent.keyboard("{Tab}");
    expect(document.activeElement).toBe(field());
    await userEvent.keyboard("{Tab}");
    expect(focusedName()).toBe("50 ms more");
    await userEvent.keyboard("{Tab}");
    expect(focusedName()).toBe("Below");
  });

  it("takes a number typed, applied on Enter, and a click and a tap on its signs", async () => {
    // Its text is selected outright: a triple click a loaded machine draws out is three clicks.
    await userEvent.click(field());
    field().select();
    await userEvent.keyboard("−150{Enter}");
    expect(field().value).toBe("−150");
    expect(caption().textContent).toBe("Delay: -150 ms");
    await userEvent.click(sign("50 ms more"));
    expect(field().value).toBe("−100");
    tapOn(sign("50 ms less"));
    tapOn(sign("50 ms less"));
    tapOn(sign("50 ms less"));
    expect(field().value).toBe("−200");
    expect(sign("50 ms less").getAttribute("aria-disabled")).toBe("true");
  });

  it("lays out −, the field and + as one row, its caption under its end, at each width", async () => {
    for (const width of partWidths) {
      await page.viewport(width, 800);
      const less = sign("50 ms less").getBoundingClientRect();
      const typed = field().getBoundingClientRect();
      const more = sign("50 ms more").getBoundingClientRect();
      const control = document.querySelector("fieldset")?.getBoundingClientRect();
      const words = caption().getBoundingClientRect();
      expect(less.right, `${String(width)} px`).toBeLessThanOrEqual(typed.left);
      expect(typed.right).toBeLessThanOrEqual(more.left);
      expect(Math.abs(less.top - more.top)).toBeLessThan(1);
      expect(less.height).toBeGreaterThanOrEqual(44);
      expect(control?.right ?? Infinity).toBeLessThanOrEqual(width);
      expect(words.top).toBeGreaterThanOrEqual(control?.bottom ?? Infinity);
      expect(Math.abs(words.right - (control?.right ?? 0))).toBeLessThan(1);
    }
  });
});

describe("a stepper's look, as the design system's mock-up draws it", () => {
  // The owner, 2026-11-23: "The - and + buttons are different sizes and have weird gaps between
  // them that were not present in the mock-ups"; and the empty space after + (revamp question 21).
  it("draws − and + the same square, abutting the middle, the frame no wider than its parts", async () => {
    mounted?.unmount();
    for (const shown of [<Column key="ms" />, <LongWorded key="hours" />]) {
      mounted = await mount(shown);
      for (const width of partWidths) {
        await page.viewport(width, 800);
        const at = `${String(width)} px`;
        const [less, more] = [...document.querySelectorAll(".stepper__control > button")].map(
          (button) => button.getBoundingClientRect(),
        );
        if (less === undefined || more === undefined) {
          throw new Error("no − and +");
        }
        const control = boxOf(".stepper__control");
        const middle = boxOf(".stepper__middle");
        const typed = field().getBoundingClientRect();
        expect(Math.abs(less.width - more.width), at).toBeLessThan(0.5);
        expect(Math.abs(less.height - more.height), at).toBeLessThan(0.5);
        expect(less.width, at).toBeGreaterThanOrEqual(44);
        expect(Math.abs(less.width - less.height), at).toBeLessThan(0.5);
        // Inside the frame's 1 px edge, − then the middle then +, with nothing between them.
        expect(Math.abs(less.left - (control.left + 1)), at).toBeLessThan(0.5);
        expect(Math.abs(less.right - middle.left), at).toBeLessThan(0.5);
        expect(Math.abs(middle.right - more.left), at).toBeLessThan(0.5);
        expect(Math.abs(more.right - (control.right - 1)), at).toBeLessThan(0.5);
        expect(Math.abs(less.top - (control.top + 1)), at).toBeLessThan(0.5);
        expect(Math.abs(less.bottom - (control.bottom - 1)), at).toBeLessThan(0.5);
        // The field no wider than a sign or its widest value: no blank between − and the figures.
        expect(typed.width, at).toBeLessThanOrEqual(Math.max(44, widestFigures()) + 0.5);
        // Its own edge and padding none (base.css's field's would win a weaker rule), its figures
        // whole within it.
        const looks = getComputedStyle(field());
        expect([looks.borderInlineStartWidth, looks.paddingInlineStart], at).toEqual([
          "0px",
          "0px",
        ]);
        expect(widthOfText(field().value, looks.font), at).toBeLessThanOrEqual(field().clientWidth);
        expect(control.right, at).toBeLessThanOrEqual(width);
      }
      mounted.unmount();
      mounted = undefined;
    }
  });
});

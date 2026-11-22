// SPDX-License-Identifier: MPL-2.0

/**
 * A number input walked by a remote's keys in a real browser, at a Fire TV's page in Silk (the owner,
 * 2026-11-22): Up and Down leave its row from the field as from its arrows, never stepping it; Left
 * and Right move the field's caret, and past its end go on to the arrows, which OK presses.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";

import { useState, type JSX } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { startInput, type Input } from "./input-commands.ts";
import { NumberInput } from "./number-input.tsx";
import { mount, type Mounted } from "./test-dom.tsx";

/** A button above the number input, laid out as a menu's row has it, and a button below. */
function Column(): JSX.Element {
  const [value, setValue] = useState(100);
  return (
    <main>
      <button type="button">Above</button>
      <NumberInput
        label="Delay"
        valueText={`Delay: ${String(value)} ms`}
        value={value}
        min={-2000}
        max={2000}
        step={50}
        unit="ms"
        stepDownLabel="50 ms less"
        stepUpLabel="50 ms more"
        layout={{ kind: "labelled", rowLabel: "Delay" }}
        onChange={setValue}
      />
      <button type="button">Below</button>
    </main>
  );
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

/** The number input's field. */
function field(): HTMLInputElement {
  const found = document.querySelector("input");
  if (found === null) {
    throw new Error("no field");
  }
  return found;
}

/** Focuses the field, its caret at the value's end, as typing leaves it. */
function focusFieldAtEnd(): void {
  const { value } = field();
  field().focus();
  field().setSelectionRange(value.length, value.length);
}

/** The accessible name of what has focus, its label or its text. */
function focusedName(): string | null | undefined {
  const focused = document.activeElement;
  return focused?.getAttribute("aria-label") ?? focused?.textContent;
}

describe("a number input, by a remote's keys", () => {
  it("leaves its row by Up and Down from the field, never stepping it", async () => {
    focusFieldAtEnd();
    await userEvent.keyboard("{ArrowDown}");
    expect(focusedName()).toBe("Below");
    focusFieldAtEnd();
    await userEvent.keyboard("{ArrowUp}");
    expect(focusedName()).toBe("Above");
    expect(field().value).toBe("100");
    expect(document.querySelector("output")?.textContent).toBe("Delay: 100 ms");
  });

  it("moves the field's caret by Left, and past its end goes Right to the arrows and back", async () => {
    focusFieldAtEnd();
    await userEvent.keyboard("{ArrowLeft}");
    expect(document.activeElement).toBe(field());
    expect(field().selectionStart).toBe(2);
    focusFieldAtEnd();
    await userEvent.keyboard("{ArrowRight}");
    expect(focusedName()).toBe("50 ms less");
    await userEvent.keyboard("{ArrowRight}");
    expect(focusedName()).toBe("50 ms more");
    await userEvent.keyboard("{ArrowLeft}");
    await userEvent.keyboard("{ArrowLeft}");
    expect(document.activeElement).toBe(field());
  });

  it("steps by OK on an arrow, one step a press, and leaves the arrows' row by Up and Down", async () => {
    focusFieldAtEnd();
    await userEvent.keyboard("{ArrowRight}");
    await userEvent.keyboard("{ArrowRight}");
    await userEvent.keyboard("{Enter}");
    expect(field().value).toBe("150");
    await userEvent.keyboard("{ArrowLeft}");
    await userEvent.keyboard("{Enter}");
    await userEvent.keyboard("{Enter}");
    expect(field().value).toBe("50");
    expect(document.querySelector("output")?.textContent).toBe("Delay: 50 ms");
    await userEvent.keyboard("{ArrowDown}");
    expect(focusedName()).toBe("Below");
    await userEvent.keyboard("{ArrowUp}");
    await userEvent.keyboard("{ArrowUp}");
    expect(focusedName()).toBe("Above");
  });
});

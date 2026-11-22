// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { NumberInput, numberTyped } from "./number-input.tsx";
import { buttonNamed, click, enter, fire, mount, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** A number input from -100 to 100 in steps of 25, starting at start, each value it heard kept. */
function Tested(props: { readonly start: number; readonly heard: number[] }): JSX.Element {
  const [value, setValue] = useState(props.start);
  return (
    <NumberInput
      label="Delay"
      valueText={`Delay: ${String(value)} ms`}
      value={value}
      min={-100}
      max={100}
      step={25}
      unit="ms"
      stepDownLabel="25 ms less"
      stepUpLabel="25 ms more"
      onChange={(next) => {
        props.heard.push(next);
        setValue(next);
      }}
    />
  );
}

/** The mounted input's field. */
function fieldOf(container: HTMLElement): HTMLInputElement {
  const field = container.querySelector("input");
  if (field === null) {
    throw new Error("no field");
  }
  return field;
}

/** The mounted input's words, its output. */
function wordsOf(container: HTMLElement): string {
  return container.querySelector("output")?.textContent ?? "";
}

/** Types text into the field and presses Enter. */
async function typeAndEnter(field: HTMLInputElement, text: string): Promise<void> {
  await enter(field, text);
  await fire(field, new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
}

describe("a number input (the owner, 2026-11-22)", () => {
  it("steps its value by its down and up arrows, held at its bounds", async () => {
    const heard: number[] = [];
    mounted = await mount(<Tested start={50} heard={heard} />);
    const { container } = mounted;
    await click(buttonNamed(container, "25 ms more"));
    expect(fieldOf(container).value).toBe("75");
    expect(wordsOf(container)).toBe("Delay: 75 ms");
    await click(buttonNamed(container, "25 ms more"));
    await click(buttonNamed(container, "25 ms more"));
    expect(fieldOf(container).value).toBe("100");
    await click(buttonNamed(container, "25 ms less"));
    expect(heard).toEqual([75, 100, 75]);
  });

  it("applies a number typed on Enter and on leaving the field, held at its bounds", async () => {
    const heard: number[] = [];
    mounted = await mount(<Tested start={0} heard={heard} />);
    const field = fieldOf(mounted.container);
    await enter(field, "-60");
    // Nothing applies while it is typed.
    expect(heard).toEqual([]);
    await fire(field, new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
    expect(heard).toEqual([-60]);
    expect(wordsOf(mounted.container)).toBe("Delay: -60 ms");
    await enter(field, "40");
    await fire(field, new FocusEvent("focusout", { bubbles: true }));
    expect(heard).toEqual([-60, 40]);
    await typeAndEnter(field, "5000");
    expect(field.value).toBe("100");
    await typeAndEnter(field, "−250");
    expect(heard).toEqual([-60, 40, 100, -100]);
  });

  it("refuses what is not a whole number, showing its value again", async () => {
    const heard: number[] = [];
    mounted = await mount(<Tested start={25} heard={heard} />);
    const field = fieldOf(mounted.container);
    for (const typed of ["abc", "", "12.5", "1e2", "--3"]) {
      await typeAndEnter(field, typed);
      expect(field.value).toBe("25");
    }
    expect(heard).toEqual([]);
    expect(numberTyped(" 30 ", -100, 100)).toBe(30);
    expect(numberTyped("+30", -100, 100)).toBe(30);
    expect(numberTyped("3 0", -100, 100)).toBeUndefined();
  });

  it("is a field named for what it holds, described by its unit, as wide as its widest value, with arrows named for what they do", async () => {
    mounted = await mount(<Tested start={0} heard={[]} />);
    const field = fieldOf(mounted.container);
    expect(field.type).toBe("text");
    expect(field.getAttribute("aria-label")).toBe("Delay");
    const unit = document.getElementById(field.getAttribute("aria-describedby") ?? "");
    expect(unit?.textContent).toBe("ms");
    expect(field.size).toBe(4);
    // Below zero, the whole keyboard, which has a minus.
    expect(field.inputMode).toBe("text");
    const arrows = [...mounted.container.querySelectorAll("button")];
    expect(arrows.map((each) => each.getAttribute("aria-label"))).toEqual([
      "25 ms less",
      "25 ms more",
    ]);
    expect(arrows.every((each) => !each.disabled && each.textContent === "")).toBe(true);
  });
});

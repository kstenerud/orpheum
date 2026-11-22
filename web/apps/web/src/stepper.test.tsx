// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { Stepper, numberShown, numberTyped } from "./stepper.tsx";
import { buttonNamed, click, enter, fire, mount, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** A stepper from -100 to 100 in steps of 25, starting at start, each value it heard kept. */
function Tested(props: { readonly start: number; readonly heard: number[] }): JSX.Element {
  const [value, setValue] = useState(props.start);
  return (
    <Stepper
      label="Delay"
      fieldLabel="Delay, in ms"
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

/** The mounted stepper's caption, its output. */
function wordsOf(container: HTMLElement): string {
  return container.querySelector("output")?.textContent ?? "";
}

/** Types text into the field and presses Enter. */
async function typeAndEnter(field: HTMLInputElement, text: string): Promise<void> {
  await enter(field, text);
  await fire(field, new KeyboardEvent("keydown", { key: "Enter", bubbles: true }));
}

describe("a stepper, the number input's next form (the owner, 2026-11-22; D538 §9)", () => {
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
    // Below zero it shows the minus sign, which it takes back typed as it is.
    expect(field.value).toBe("−100");
    await typeAndEnter(field, field.value);
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

  // The subtitles' timing is kept in ms and shown in seconds to a tenth, "+0.5" above zero (the
  // menu spec's Timing row, D538 §9).
  it("shows a value in a unit of its own, a leading + above zero, and takes a fraction typed in it", () => {
    const seconds = { scale: 1000, fractionDigits: 1 };
    expect(numberShown(500, seconds, true)).toBe("+0.5");
    expect(numberShown(-1200, seconds, true)).toBe("−1.2");
    expect(numberShown(0, seconds, true)).toBe("0.0");
    expect(numberShown(40, undefined, true)).toBe("+40");
    expect(numberTyped("1.25", -600000, 600000, seconds)).toBe(1250);
    expect(numberTyped("−0,3", -600000, 600000, seconds)).toBe(-300);
    expect(numberTyped(".5", -600000, 600000, seconds)).toBe(500);
    expect(numberTyped("1.5.2", -600000, 600000, seconds)).toBeUndefined();
    // A field showing whole numbers still refuses a fraction.
    expect(numberTyped("1.5", -100, 100)).toBeUndefined();
  });

  it("is a field named for what it holds, described by its unit, as wide as its widest value, with arrows named for what they do", async () => {
    mounted = await mount(<Tested start={0} heard={[]} />);
    const field = fieldOf(mounted.container);
    expect(field.type).toBe("text");
    expect(field.getAttribute("aria-label")).toBe("Delay, in ms");
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

  it("is a group named by its label, described by its caption, shown only while the value is not 0", async () => {
    mounted = await mount(<Tested start={0} heard={[]} />);
    const { container } = mounted;
    const group = container.querySelector("fieldset");
    const named = (attribute: string): HTMLElement | null =>
      document.getElementById(group?.getAttribute(attribute) ?? "");
    expect(named("aria-labelledby")?.textContent).toBe("Delay");
    const caption = named("aria-describedby");
    if (!caption) {
      throw new Error("no caption");
    }
    expect(caption.tagName).toBe("OUTPUT");
    expect(caption.textContent).toBe("Delay: 0 ms");
    expect(caption.classList.contains("stepper__caption--unshown")).toBe(true);
    await click(buttonNamed(container, "25 ms less"));
    expect(caption.textContent).toBe("Delay: -25 ms");
    expect(caption.classList.contains("stepper__caption--unshown")).toBe(false);
  });

  it("marks − at its least and + at its most unavailable, a press doing nothing, yet focusable (D538 §9)", async () => {
    const heard: number[] = [];
    mounted = await mount(<Tested start={-100} heard={heard} />);
    const { container } = mounted;
    const less = buttonNamed(container, "25 ms less");
    const more = buttonNamed(container, "25 ms more");
    if (!less || !more) {
      throw new Error("no − or +");
    }
    expect(less.getAttribute("aria-disabled")).toBe("true");
    expect(more.hasAttribute("aria-disabled")).toBe(false);
    await click(less);
    expect(heard).toEqual([]);
    // Never disabled, which would drop a television's focus with nowhere to go.
    expect(less.hasAttribute("disabled")).toBe(false);
    less.focus();
    expect(document.activeElement).toBe(less);
    await click(more);
    expect(heard).toEqual([-75]);
    expect(less.hasAttribute("aria-disabled")).toBe(false);
    await typeAndEnter(fieldOf(container), "100");
    expect(more.getAttribute("aria-disabled")).toBe("true");
  });
});

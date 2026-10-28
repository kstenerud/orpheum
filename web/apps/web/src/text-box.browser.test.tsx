// SPDX-License-Identifier: MPL-2.0

/**
 * A text box's example text, as a real browser draws it: dimmer than any value typed in and than
 * secondary text, and slanted, so it never reads as a value to delete (the owner, 2026-10-28: "The
 * example text in text boxes is too bright. At first I mistook it for a pre-filled field and tried to
 * delete its contents").
 */

import "./tokens.css";
import "./base.css";

import { describe, expect, it } from "vitest";

import { mount } from "./test-dom.tsx";

/** The relative luminance of a computed `rgb(…)` colour (WCAG 2), 0 for black to 1 for white. */
function luminanceOf(colour: string): number {
  const channels = (colour.match(/[\d.]+/gu) ?? []).slice(0, 3).map(Number);
  const [red = 0, green = 0, blue = 0] = channels.map((channel) => {
    const c = channel / 255;
    return c <= 0.039_28 ? c / 12.92 : ((c + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * red + 0.7152 * green + 0.0722 * blue;
}

describe("a text box's example text", () => {
  it("is dimmer than a typed value and than secondary text, and slanted", async () => {
    const mounted = await mount(
      <div>
        <input placeholder="https://192-168-1-10.….orpheum-app.com:8443" />
        <span style={{ color: "var(--color-muted)" }}>secondary</span>
      </div>,
    );
    const input = mounted.container.querySelector("input");
    const muted = mounted.container.querySelector("span");
    if (input === null || muted === null) {
      throw new Error("the text box is not drawn");
    }
    const example = getComputedStyle(input, "::placeholder");
    const typed = luminanceOf(getComputedStyle(input).color);
    const secondary = luminanceOf(getComputedStyle(muted).color);
    expect(luminanceOf(example.color)).toBeLessThan(secondary * 0.7);
    expect(luminanceOf(example.color)).toBeLessThan(typed * 0.5);
    expect(example.fontStyle).toBe("italic");
    mounted.unmount();
  });
});

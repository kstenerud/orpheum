// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { declarationsOf, sourceHere } from "./test-sources.ts";

/** The colour a token of tokens.css's :root is, a #rrggbb. */
function token(name: string): string {
  const found = declarationsOf(sourceHere("tokens.css")).find(
    (declaration) => declaration.selector === ":root" && declaration.text.startsWith(`--${name}:`),
  );
  const value = found?.text.slice(name.length + 3).trim() ?? "";
  if (!/^#[\da-f]{6}$/u.test(value)) {
    throw new Error(`--${name} is no #rrggbb: ${value}`);
  }
  return value;
}

/** A #rrggbb's relative luminance (WCAG 2.2's definition). */
function luminance(colour: string): number {
  const [r = 0, g = 0, b = 0] = [1, 3, 5].map((at) => {
    const channel = Number.parseInt(colour.slice(at, at + 2), 16) / 255;
    return channel <= 0.040_45 ? channel / 12.92 : ((channel + 0.055) / 1.055) ** 2.4;
  });
  return 0.2126 * r + 0.7152 * g + 0.0722 * b;
}

/** The contrast between two #rrggbb colours, 1 to 21. */
function contrast(one: string, other: string): number {
  const [light, dark] = [luminance(one), luminance(other)].toSorted((a, b) => b - a);
  return ((light ?? 0) + 0.05) / ((dark ?? 0) + 0.05);
}

describe("the colours of a control's parts (D538)", () => {
  it("draws a search box's example text readable on the surface a text box fills itself with (WCAG 1.4.3)", () => {
    // A text box is drawn on its own surface wherever it lies, a raised panel's included (base.css),
    // so that surface is the one its example text is read on.
    const fills = declarationsOf(sourceHere("base.css")).filter(
      (declaration) =>
        declaration.selector.split(",")[0]?.trim().startsWith("input:not(") === true &&
        declaration.text.startsWith("background:"),
    );
    expect(fills.map((declaration) => declaration.text)).toEqual([
      "background: var(--color-surface)",
    ]);
    expect(contrast(token("color-placeholder"), token("color-surface"))).toBeGreaterThanOrEqual(
      4.5,
    );
  });

  it("draws a control's own edge seen on a surface and a raised one (WCAG 1.4.11)", () => {
    expect(contrast(token("color-control-edge"), token("color-surface"))).toBeGreaterThanOrEqual(3);
    expect(
      contrast(token("color-control-edge"), token("color-surface-raised")),
    ).toBeGreaterThanOrEqual(3);
  });

  it("measures contrast as WCAG does, so the checks above can fail", () => {
    expect(contrast("#000000", "#ffffff")).toBeCloseTo(21, 5);
    expect(contrast("#6b6b75", "#1c1c20")).toBeLessThan(4.5);
  });
});

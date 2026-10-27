// SPDX-License-Identifier: MPL-2.0

import { readdirSync, readFileSync } from "node:fs";
import { renderToStaticMarkup } from "react-dom/server";
import { describe, expect, it } from "vitest";

import { Icon } from "./icon.tsx";

/** This directory, web/apps/web/src/, whose sources are read. */
const here = new URL("./", import.meta.url);

/** Each source in this directory ending in suffix, tests left out, its text by its name. */
function sourcesEndingIn(suffix: string): readonly (readonly [string, string])[] {
  return readdirSync(here)
    .filter((name) => name.endsWith(suffix) && !/\.test\.tsx?$/u.test(name))
    .map((name) => [name, readFileSync(new URL(name, here), "utf8")] as const);
}

/**
 * The colours an icon's shapes may be given: the text colour of where it is drawn, nothing, its
 * ground, and the warning triangle's edge, a token, which keeps its yellow apart from a pale ground.
 */
const iconColours = new Set([
  "currentcolor",
  "none",
  "var(--icon-ground)",
  "var(--color-icon-outline)",
]);

/** Each fill or stroke a stylesheet declares that is not an icon's colour, as `file: value`. */
function strayPaints(sheets: readonly (readonly [string, string])[]): string[] {
  return sheets.flatMap(([name, text]) =>
    [...text.matchAll(/(?:^|[;{\s])(fill|stroke)\s*:\s*([^;}]+)/gu)]
      .map((match) => (match[2] ?? "").trim())
      .filter((value) => !iconColours.has(value.toLowerCase()))
      .map((value) => `${name}: ${value}`),
  );
}

/** Each source that draws its own SVG element or paints a shape by attribute, as `file: what`. */
function strayDrawings(sources: readonly (readonly [string, string])[]): string[] {
  return sources.flatMap(([name, text]) => [
    ...[...text.matchAll(/<svg\b/gu)].map(() => `${name}: <svg`),
    ...[...text.matchAll(/\b(fill|stroke)="([^"]*)"/gu)]
      .filter((match) => !iconColours.has((match[2] ?? "").toLowerCase()))
      .map((match) => `${name}: ${match[0]}`),
  ]);
}

describe("every icon (the owner, 2026-10-27)", () => {
  it("is drawn by Icon, hidden from a screen reader, in its own class and the common one", () => {
    const markup = renderToStaticMarkup(
      <Icon className="test-icon">
        <path d="M0 0h1v1z" />
      </Icon>,
    );
    expect(markup).toBe(
      '<svg class="icon test-icon" viewBox="0 0 24 24" aria-hidden="true" focusable="false"><path d="M0 0h1v1z"></path></svg>',
    );
    expect(
      renderToStaticMarkup(
        <Icon className="x" viewBox="0 0 64 64">
          {null}
        </Icon>,
      ),
    ).toContain('viewBox="0 0 64 64"');
  });

  it("is drawn by Icon alone: no other component writes an SVG element or paints its shapes", () => {
    // The QR code draws its own, black on white, as a scanner needs whatever the theme.
    const sources = sourcesEndingIn(".tsx").filter(
      ([name]) => name !== "icon.tsx" && name !== "qr-code.tsx",
    );
    expect(sources.length).toBeGreaterThan(50);
    expect(strayDrawings(sources)).toEqual([]);
  });

  it("is coloured only by the text colour and its ground, which its region sets", () => {
    const sheets = sourcesEndingIn(".css");
    expect(sheets.map(([name]) => name)).toContain("warning-sign.css");
    expect(strayPaints(sheets)).toEqual([]);
  });

  it("finds a stray SVG, painted shape or paint, so the checks above can fail", () => {
    expect(
      strayDrawings([
        ["a.tsx", '<svg viewBox="0 0 24 24"><path fill="#fff" stroke="currentColor" /></svg>'],
      ]),
    ).toEqual(["a.tsx: <svg", 'a.tsx: fill="#fff"']);
    expect(
      strayPaints([
        [
          "a.css",
          ".a { fill: var(--color-caution); stroke: currentcolor; }\n.b {\n  stroke: rgb(0 0 0 / 35%);\n  fill: none;\n  stroke-width: 2;\n}",
        ],
      ]),
    ).toEqual(["a.css: var(--color-caution)", "a.css: rgb(0 0 0 / 35%)"]);
  });
});

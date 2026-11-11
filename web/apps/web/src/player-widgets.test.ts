// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import { sourcesHere } from "./test-sources.ts";

/** Each component source in this directory, tests left out, its text by its name. */
function components(): readonly (readonly [string, string])[] {
  return sourcesHere(".tsx");
}

/** The sources that may draw a slider: a film's timeline, and a volume's. */
const drawsRanges = new Set(["timeline.tsx", "volume-range.tsx"]);

/**
 * The sources whose own icon-only buttons wait for the later batches of the widget set (D369 §5;
 * ui-widgets-audit-2026-10-27.md §4.4): the settings rows' controls, each with its tooltip and its
 * question to confirm, and voice's Talk, a toggle with its level ring. None is the player's, and the
 * list only shrinks.
 */
const iconButtonsWaiting = new Set(["settings.tsx", "server-settings.tsx", "together-voice.tsx"]);

/** Each slider a source draws itself, as `file: type="range"`. */
function strayRanges(sources: readonly (readonly [string, string])[]): string[] {
  return sources
    .filter(([name]) => !drawsRanges.has(name))
    .flatMap(([name, text]) =>
      [...text.matchAll(/type=["{]?["']?range\b/gu)].map(() => `${name}: type="range"`),
    );
}

/** Each `<button …>` opening tag in text, read past the braces of its attributes' expressions. */
function buttonTags(text: string): { readonly tag: string; readonly content: string }[] {
  const tags: { tag: string; content: string }[] = [];
  for (const start of text.matchAll(/<button\b/gu)) {
    let at = start.index + start[0].length;
    let depth = 0;
    while (at < text.length && !(text[at] === ">" && depth === 0)) {
      if (text[at] === "{") {
        depth += 1;
      } else if (text[at] === "}") {
        depth -= 1;
      }
      at += 1;
    }
    const tag = text.slice(start.index, at + 1);
    const end = text.indexOf("</button>", at);
    tags.push({ tag, content: end < 0 ? "" : text.slice(at + 1, end) });
  }
  return tags;
}

/**
 * Whether a button's content is an icon alone: a text character with no letter (▶, ×, ⤢), as text
 * or a string, or nothing but an icon component, with no words beside it.
 */
function isIconAlone(content: string): boolean {
  const words = content
    .replaceAll(/<[A-Z]\w*Icon\b[^>]*\/>/gu, "")
    .replaceAll(/<\/?span[^>]*>/gu, "")
    .replaceAll(/[{}"'\s]/gu, "");
  const hasIcon = /<[A-Z]\w*Icon\b|<Icon\b/u.test(content);
  return words === "" ? hasIcon : !/[\p{L}\p{N}(]/u.test(words);
}

/** Each button a source draws as an icon alone, outside IconButton, as `file: <button …`. */
function strayIconButtons(sources: readonly (readonly [string, string])[]): string[] {
  return sources
    .filter(([name]) => name !== "icon-button.tsx" && !iconButtonsWaiting.has(name))
    .flatMap(([name, text]) =>
      buttonTags(text)
        .filter(({ content }) => isIconAlone(content))
        .map(({ tag }) => `${name}: ${tag.replaceAll(/\s+/gu, " ").slice(0, 80)}`),
    );
}

describe("the player's widgets, one of each (D369 §4)", () => {
  it("draws a slider in Timeline, a film's, and VolumeRange, a volume's, alone", () => {
    const sources = components();
    expect(sources.length).toBeGreaterThan(50);
    expect(strayRanges(sources)).toEqual([]);
  });

  it("draws a button that is an icon alone in IconButton alone, every player button among them", () => {
    expect(strayIconButtons(components())).toEqual([]);
  });

  it("finds a slider, a glyph button or an icon button drawn by hand, so the checks above can fail", () => {
    expect(
      strayRanges([
        ["a.tsx", '<input type="range" value={1} />'],
        ["timeline.tsx", '<input type="range" />'],
      ]),
    ).toEqual(['a.tsx: type="range"']);
    expect(
      strayIconButtons([
        [
          "a.tsx",
          [
            '<button type="button" onClick={() => {}} aria-label={t("x")}>',
            '  <span aria-hidden="true">{"×"}</span>',
            "</button>",
            '<button type="button" onClick={() => go()}><PlayIcon /></button>',
            '<button type="button">{t("upNext.play")}</button>',
            '<button type="button"><PlayIcon /> {t("play.it")}</button>',
          ].join("\n"),
        ],
      ]),
    ).toEqual([
      'a.tsx: <button type="button" onClick={() => {}} aria-label={t("x")}>',
      'a.tsx: <button type="button" onClick={() => go()}>',
    ]);
  });
});

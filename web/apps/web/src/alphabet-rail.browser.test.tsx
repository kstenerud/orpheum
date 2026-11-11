// SPDX-License-Identifier: MPL-2.0

/**
 * A letter jump's poster in a real browser (alphabet-rail.tsx): shown at the top, clear of the
 * banner, and kept there while the cards around it, laid out only near the view, take their heights.
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { AlphabetRail, showAtTop } from "./alphabet-rail.tsx";
import { coversTopAttribute } from "./focus-engine.ts";
import { clearOfTheBannerPx } from "./focus-navigation.ts";
import { mount, type Mounted } from "./test-dom.tsx";

/**
 * A banner over the page's top and a list of cards each guessed at 100 pixels tall until it is drawn
 * near the view, three times that once it is: what a poster grid's content-visibility does.
 */
const listPage = (
  <>
    <header
      {...{ [coversTopAttribute]: "" }}
      style={{ position: "fixed", insetBlockStart: 0, insetInline: 0, height: "60px" }}
    />
    <ul style={{ margin: 0, padding: 0, listStyle: "none" }}>
      {Array.from({ length: 200 }, (_, index) => (
        <li key={index} style={{ contentVisibility: "auto", containIntrinsicSize: "auto 100px" }}>
          <a href={`#card-${String(index)}`} style={{ display: "block", height: "300px" }}>
            card {index}
          </a>
        </li>
      ))}
    </ul>
  </>
);

/**
 * Waits for the browser to draw a frame, then for the task after it: where the page is then is
 * where it was drawn, with what ran before the drawing (its ResizeObservers) applied.
 */
async function drawn(): Promise<void> {
  await new Promise((resolve) => requestAnimationFrame(resolve));
  await new Promise((resolve) => setTimeout(resolve, 0));
}

describe("a letter jump's poster, as the cards around it take their heights (the owner, Firefox, 2026-11-11)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    window.scrollTo(0, 0);
  });

  it("is drawn at the top, clear of the banner, frame after frame", async () => {
    await page.viewport(960, 480);
    mounted = await mount(listPage);
    const poster = document.querySelector<HTMLElement>('a[href="#card-120"]');
    if (poster === null) {
      throw new Error("no card 120");
    }
    showAtTop(poster);
    const tops: number[] = [];
    for (let index = 0; index < 6; index += 1) {
      await drawn();
      tops.push(Math.round(poster.getBoundingClientRect().top - clearOfTheBannerPx()));
    }
    expect(tops).toEqual([0, 0, 0, 0, 0, 0]);
  });
});

describe("the alphabet rail by Tab (the owner, 2026-11-11: one Tab stop, as the poster grid is)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("is entered at the letter last focused, and left by the next Tab", async () => {
    mounted = await mount(
      <>
        <button type="button">before</button>
        <AlphabetRail onLetter={() => undefined} letters={["A", "B", "C", "D"]} />
        <button type="button">after</button>
      </>,
    );
    const letter = (name: string): HTMLElement => {
      const found = [...document.querySelectorAll<HTMLElement>(".alphabet-rail__letter")].find(
        (button) => button.textContent === name,
      );
      if (found === undefined) {
        throw new Error(`no letter ${name}`);
      }
      return found;
    };
    letter("C").focus();
    await userEvent.tab();
    expect(document.activeElement?.textContent).toBe("after");
    await userEvent.tab({ shift: true });
    expect(document.activeElement).toBe(letter("C"));
    await userEvent.tab({ shift: true });
    expect(document.activeElement?.textContent).toBe("before");
    const stops = [...document.querySelectorAll<HTMLElement>(".alphabet-rail__letter")].filter(
      (button) => button.tabIndex >= 0,
    );
    expect(stops).toEqual([letter("C")]);
  });
});

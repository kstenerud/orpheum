// SPDX-License-Identifier: MPL-2.0

/**
 * The "Still watching?" card as a real browser lays it out (D339): its question runs the card's
 * full width, with its two answers in a row under it, not squeezed into Up next's thumbnail
 * column, which the card does not have a thumbnail to fill.
 */

import "./tokens.css";
import "./base.css";
import "./up-next.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { StillWatching } from "./still-watching.tsx";
import { mount, type Mounted } from "./test-dom.tsx";

describe("the Still watching card, laid out by a real browser", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("runs its question the card's full width, with its two answers in a row", async () => {
    mounted = await mount(<StillWatching onContinue={() => undefined} onStop={() => undefined} />);
    mounted.container.style.width = "26rem";
    const card = mounted.container.querySelector(".still-watching__card");
    const question = mounted.container.querySelector(".still-watching__card p");
    const buttons = [...mounted.container.querySelectorAll(".still-watching__card button")];
    if (card === null || question === null || buttons.length !== 2) {
      throw new Error("the card is missing its question or its two answers");
    }
    expect(getComputedStyle(card).display).toBe("flex");
    // The question runs the card's content width, not squeezed into an 8rem thumbnail column.
    const cardBox = card.getBoundingClientRect();
    const questionBox = question.getBoundingClientRect();
    expect(questionBox.width).toBeGreaterThan(cardBox.width * 0.9);
    // The two answers sit beside each other in a row under the question, not one above the other.
    const [first, second] = buttons.map((button) => button.getBoundingClientRect());
    expect(first).toBeDefined();
    expect(second).toBeDefined();
    if (first !== undefined && second !== undefined) {
      expect(Math.abs(first.top - second.top)).toBeLessThan(2);
      expect(first.top).toBeGreaterThanOrEqual(questionBox.bottom - 1);
    }
  });
});

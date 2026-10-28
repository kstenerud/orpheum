// SPDX-License-Identifier: MPL-2.0

/**
 * Where a drop-down's list opens, as a real browser lays it out (drop-down-place.ts): at its button,
 * so what it belongs to is plain (the owner, 2026-10-28: "try to make them overlap the chooser
 * somehow"), inside the screen whatever the button's place, on a big screen its current choice over
 * the button, on a phone's a pop-up over it; the page under it never moving.
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { screenEdgeRoomPx } from "./drop-down-place.ts";
import { ActionChooser } from "./route-chooser.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** The screens the lists are opened on: a computer's, a Fire TV's page in Silk, and a phone's. */
const screens = [
  { name: "a computer's window", width: 1280, height: 800, isPhone: false },
  { name: "a Fire TV's page in Silk", width: 960, height: 480, isPhone: false },
  { name: "a phone's", width: 390, height: 740, isPhone: true },
] as const;

/** Where on the screen the chooser stands: its corners and its middle. */
const spots = [
  { name: "top left", left: "8px", top: "8px" },
  { name: "middle", left: "40%", top: "45%" },
  { name: "bottom right", left: "calc(100% - 12rem)", top: "calc(100% - 3rem)" },
] as const;

/** Twelve choices, the seventh the current one. */
const sorts = Array.from({ length: 12 }, (_, at) => ({
  key: String(at),
  label: `Choice ${String(at + 1)}`,
  isCurrent: at === 6,
  onChoose: () => undefined,
}));

/** How far layout's rounding may put a box. */
const roundingPx = 1;

/** A box, or a failure where the element is not drawn. */
function boxOf(element: Element | null | undefined): DOMRect {
  if (element === null || element === undefined) {
    throw new Error("an element the test measures is not drawn");
  }
  return element.getBoundingClientRect();
}

/** Whether two boxes overlap or touch. */
function isMeeting(one: DOMRect, other: DOMRect): boolean {
  return (
    one.left <= other.right + roundingPx &&
    other.left <= one.right + roundingPx &&
    one.top <= other.bottom + roundingPx &&
    other.top <= one.bottom + roundingPx
  );
}

describe.each(screens)("a drop-down's list on $name", ({ width, height, isPhone }) => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it.each(spots)("opens at its button at the $name, inside the screen", async (spot) => {
    await page.viewport(width, height);
    mounted = await mount(
      withServices(
        <>
          <div style={{ height: "300vh" }} />
          <div style={{ position: "fixed", left: spot.left, top: spot.top }}>
            <ActionChooser
              fieldLabel="Order"
              isFieldLabelShown={false}
              current="Choice 7"
              options={sorts}
            />
          </div>
        </>,
      ),
    );
    window.scrollTo({ top: height });
    const scrolled = window.scrollY;
    const opener = mounted.container.querySelector<HTMLElement>(".route-chooser__button");
    await userEvent.click(opener ?? document.body);
    const list = document.querySelector("dialog");
    expect(list).not.toBeNull();
    const [listBox, openerBox] = [boxOf(list), boxOf(opener)];
    expect(isMeeting(listBox, openerBox)).toBe(true);
    expect(listBox.left).toBeGreaterThanOrEqual(screenEdgeRoomPx - roundingPx);
    expect(listBox.top).toBeGreaterThanOrEqual(screenEdgeRoomPx - roundingPx);
    expect(listBox.right).toBeLessThanOrEqual(width - screenEdgeRoomPx + roundingPx);
    expect(listBox.bottom).toBeLessThanOrEqual(height - screenEdgeRoomPx + roundingPx);
    expect(document.activeElement?.textContent).toBe("Choice 7");
    if (!isPhone) {
      expect(listBox.width).toBeGreaterThanOrEqual(openerBox.width - roundingPx);
    }
    if (!isPhone && spot.name === "middle") {
      // As a select's list: the current choice laid over the button.
      const current = boxOf(document.activeElement);
      const middleOf = (box: DOMRect): number => box.top + box.height / 2;
      expect(Math.abs(middleOf(current) - middleOf(openerBox))).toBeLessThan(2);
    }
    if (isPhone) {
      // A pop-up over the button, its top the button's where it fits.
      expect(listBox.top).toBeLessThanOrEqual(openerBox.top + roundingPx);
    }
    expect(window.scrollY).toBe(scrolled);
    await userEvent.keyboard("{Escape}");
    expect(document.querySelector("dialog")).toBeNull();
    expect(window.scrollY).toBe(scrolled);
  });
});

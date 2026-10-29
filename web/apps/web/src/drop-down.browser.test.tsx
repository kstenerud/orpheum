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

  // The owner's Fire TV, 2026-10-29: "The X button on the list partially obscures the first item ...
  // You can see it partially covering the outline around the first item in the list."
  it("keeps its first place, focus ring and all, clear of its Close", async () => {
    await page.viewport(width, height);
    mounted = await mount(
      withServices(
        <ActionChooser
          fieldLabel="Order"
          isFieldLabelShown={false}
          current="Choice 1"
          options={sorts.map((sort, at) => ({ ...sort, isCurrent: at === 0 }))}
        />,
      ),
    );
    mounted.container.querySelector<HTMLElement>(".route-chooser__button")?.focus();
    await userEvent.keyboard("{Enter}");
    const first = document.querySelector<HTMLElement>("dialog ul button");
    await expect.poll(() => document.activeElement).toBe(first);
    const close = boxOf(document.querySelector("dialog .drop-down__close"));
    expect(isOverlapping(close, ringedBoxOf(first))).toBe(false);
  });
});

/** Whether two boxes overlap, more than layout's rounding. */
function isOverlapping(one: DOMRect, other: DOMRect): boolean {
  return (
    one.left < other.right - roundingPx &&
    other.left < one.right - roundingPx &&
    one.top < other.bottom - roundingPx &&
    other.top < one.bottom - roundingPx
  );
}

/** An element's box with the focus ring drawn around it (tokens.css's --focus-ring-room). */
function ringedBoxOf(element: Element | null): DOMRect {
  const box = boxOf(element);
  const room = Number.parseFloat(
    getComputedStyle(document.documentElement).getPropertyValue("--focus-ring-room"),
  );
  const ring = Number.isNaN(room) ? 6 : room;
  return new DOMRect(box.left - ring, box.top - ring, box.width + 2 * ring, box.height + 2 * ring);
}

/** Five choices, as a five-season series' picker, the one at current the current one. */
function fiveChoices(current: number): readonly {
  key: string;
  label: string;
  isCurrent: boolean;
  onChoose: () => undefined;
}[] {
  return Array.from({ length: 5 }, (_, at) => ({
    key: String(at),
    label: `Choice ${String(at + 1)}`,
    isCurrent: at === current,
    onChoose: () => undefined,
  }));
}

/** The middle of a box, from the screen's top. */
function middleOf(box: DOMRect): number {
  return box.top + box.height / 2;
}

/**
 * Where the screen's edge would cut a short list at its button (the owner, 2026-10-29): cut there,
 * scrolling within itself as a select's, its choice over the button, while it keeps half its height
 * at least; past that moved to stay on screen, whole.
 */
describe("a short list near a short window's bottom (1920×600)", () => {
  const [width, height] = [1920, 600];
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  /** Mounts a chooser of five choices, at current, standing at the screen's middle. */
  async function mountChooser(current: number): Promise<HTMLElement> {
    await page.viewport(width, height);
    mounted = await mount(
      withServices(
        <div className="test-spot" style={{ position: "fixed", left: "40%", top: "45%" }}>
          <ActionChooser
            fieldLabel="Season"
            isFieldLabelShown={false}
            current={`Choice ${String(current + 1)}`}
            options={fiveChoices(current)}
          />
        </div>,
      ),
    );
    const opener = mounted.container.querySelector<HTMLElement>(".route-chooser__button");
    if (opener === null) {
      throw new Error("the chooser's button is not drawn");
    }
    return opener;
  }

  /**
   * The list's own height and how far below its top its choice's middle is, opened where it fits,
   * its choice over the button; closed again.
   */
  async function measured(opener: HTMLElement): Promise<{ list: number; choice: number }> {
    await userEvent.click(opener);
    const listBox = boxOf(document.querySelector("dialog"));
    const choice = middleOf(boxOf(document.activeElement)) - listBox.top;
    expect(Math.abs(choice + listBox.top - middleOf(boxOf(opener)))).toBeLessThan(2);
    await userEvent.keyboard("{Escape}");
    return { list: listBox.height, choice };
  }

  /** Moves the chooser so its list, its choice over the button, would cross the bottom by over. */
  function standCrossingBottomBy(
    opener: HTMLElement,
    over: number,
    at: { list: number; choice: number },
  ): void {
    const openerBox = boxOf(opener);
    const middle = height - screenEdgeRoomPx + over - at.list + at.choice;
    const spot = opener.closest<HTMLElement>(".test-spot");
    spot?.style.setProperty("top", `${String(middle - openerBox.height / 2)}px`);
    expect(boxOf(opener).bottom).toBeLessThanOrEqual(height);
  }

  it("is cut at the edge and scrolls, its choice over the button, keeping over half", async () => {
    const opener = await mountChooser(2);
    const at = await measured(opener);
    standCrossingBottomBy(opener, at.list / 3, at);
    await userEvent.click(opener);
    const list = document.querySelector("dialog");
    const places = list?.querySelector<HTMLElement>(".drop-down__places");
    const listBox = boxOf(list);
    expect(document.activeElement?.textContent).toBe("Choice 3");
    expect(
      Math.abs(middleOf(boxOf(document.activeElement)) - middleOf(boxOf(opener))),
    ).toBeLessThan(2);
    expect(listBox.bottom).toBeLessThanOrEqual(height - screenEdgeRoomPx + roundingPx);
    expect(listBox.height).toBeLessThan(at.list - roundingPx);
    expect(listBox.height).toBeGreaterThanOrEqual(at.list / 2 - roundingPx);
    expect(places?.scrollHeight ?? 0).toBeGreaterThan((places?.clientHeight ?? 0) + roundingPx);
    await userEvent.keyboard("{Escape}");
    expect(document.querySelector("dialog")).toBeNull();
  });

  it("moves up whole where the cut would keep less than half", async () => {
    const opener = await mountChooser(0);
    const at = await measured(opener);
    standCrossingBottomBy(opener, (at.list * 2) / 3, at);
    await userEvent.click(opener);
    const list = document.querySelector("dialog");
    const places = list?.querySelector<HTMLElement>(".drop-down__places");
    const listBox = boxOf(list);
    expect(document.activeElement?.textContent).toBe("Choice 1");
    expect(Math.abs(listBox.height - at.list)).toBeLessThanOrEqual(roundingPx);
    expect(places?.scrollHeight ?? 0).toBeLessThanOrEqual((places?.clientHeight ?? 0) + roundingPx);
    expect(Math.abs(listBox.bottom - (height - screenEdgeRoomPx))).toBeLessThanOrEqual(roundingPx);
    expect(isMeeting(listBox, boxOf(opener))).toBe(true);
    await userEvent.keyboard("{Escape}");
    expect(document.querySelector("dialog")).toBeNull();
  });
});

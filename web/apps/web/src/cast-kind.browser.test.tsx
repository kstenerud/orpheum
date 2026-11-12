// SPDX-License-Identifier: MPL-2.0

/**
 * Cast's menu's marks and its separator as a real browser lays them out (cast.css, D465 §3): each
 * mark the size of a list's tool icon with a space before its name, and between the person's devices
 * and Chromecast a quiet rule, spaced as the list's entries are.
 */

import "./tokens.css";
import "./base.css";
import "./cast.css";
import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { CastKindMark } from "./cast-kind.tsx";
import { FindDevicesIcon } from "./icons.tsx";
import { mount, type Mounted } from "./test-dom.tsx";

/** A mark's side: 1.125rem at the browser's 16 px root. */
const markSidePx = 18;

/** The space between a mark and its name: --space-2, 0.5rem. */
const markSpacePx = 8;

/** The room above and below the separator: --space-3, 0.75rem. */
const separatorRoomPx = 12;

/** How far layout's rounding may put a box. */
const roundingPx = 1;

/** Where element is drawn; it fails the test when there is none. */
function boxOf(element: Element | null | undefined): DOMRect {
  if (element === null || element === undefined) {
    throw new Error("an element the test measures is not drawn");
  }
  return element.getBoundingClientRect();
}

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

describe("Cast's menu's marks", () => {
  it.each([
    { name: "the laurel", mark: <CastKindMark kind="orpheum" /> },
    { name: "the Cast icon", mark: <CastKindMark kind="chromecast" /> },
    { name: "the magnifying glass", mark: <FindDevicesIcon /> },
  ])("draws $name a tool icon's size, a space before the name beside it", async ({ mark }) => {
    mounted = await mount(
      <button type="button">
        {mark}
        <span>Living room</span>
      </button>,
    );
    const drawn = boxOf(mounted.container.querySelector(".cast-kind"));
    const name = boxOf(mounted.container.querySelector("span"));
    expect(Math.abs(drawn.width - markSidePx)).toBeLessThanOrEqual(roundingPx);
    expect(Math.abs(drawn.height - markSidePx)).toBeLessThanOrEqual(roundingPx);
    expect(Math.abs(name.left - drawn.right - markSpacePx)).toBeLessThanOrEqual(roundingPx);
  });

  it("separates Chromecast from the person's devices by a single quiet rule, spaced as the entries are", async () => {
    mounted = await mount(<hr className="cast-to__separator" />);
    const rule = mounted.container.querySelector("hr");
    if (rule === null) {
      throw new Error("no separator drawn");
    }
    const style = getComputedStyle(rule);
    expect(style.borderTopWidth).toBe("1px");
    expect(style.borderTopStyle).toBe("solid");
    expect(style.borderBottomWidth).toBe("0px");
    expect(style.borderLeftWidth).toBe("0px");
    expect(Number.parseFloat(style.marginTop)).toBe(separatorRoomPx);
    expect(Number.parseFloat(style.marginBottom)).toBe(separatorRoomPx);
  });
});

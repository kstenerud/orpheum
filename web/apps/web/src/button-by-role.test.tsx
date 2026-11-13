// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, describe, expect, it, vi } from "vitest";

import { ButtonByRole } from "./button-by-role.tsx";
import { fire, mount, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** Presses key on element, and whether the browser's own default was taken from it. */
async function pressed(element: Element | null, key: string): Promise<boolean> {
  const down = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true });
  await fire(element, down);
  return down.defaultPrevented;
}

// A <button> focused by a key costs Silk on a Fire TV 300 ms of layout a move, the first 5 s
// (idiosyncrasies.md, "Silk lays the page out each time a key focuses a button").
describe("a button by role", () => {
  it("is a div of role button in Tab's order, named as it is given", async () => {
    mounted = await mount(<ButtonByRole aria-label="Libraries">L</ButtonByRole>);
    const button = mounted.container.querySelector("[aria-label='Libraries']");
    expect(button?.tagName).toBe("DIV");
    expect(button?.getAttribute("role")).toBe("button");
    expect(button?.getAttribute("tabindex")).toBe("0");
  });

  it("keeps a roving Tab stop's -1", async () => {
    mounted = await mount(<ButtonByRole tabIndex={-1}>A</ButtonByRole>);
    expect(mounted.container.querySelector("[role='button']")?.getAttribute("tabindex")).toBe("-1");
  });

  // OK and Enter are the input layer's, which clicks what has focus (focus-navigation.ts).
  it("is pressed by Space, as a <button> is, the page not scrolled; Enter left to the input layer", async () => {
    const onClick = vi.fn();
    mounted = await mount(<ButtonByRole onClick={onClick}>A</ButtonByRole>);
    const button = mounted.container.querySelector("[role='button']");
    expect(await pressed(button, " ")).toBe(true);
    expect(onClick).toHaveBeenCalledOnce();
    expect(await pressed(button, "Enter")).toBe(false);
    expect(await pressed(button, "a")).toBe(false);
    expect(onClick).toHaveBeenCalledOnce();
  });
});

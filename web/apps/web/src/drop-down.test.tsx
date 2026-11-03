// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { sendFailuresTo } from "@orpheum/sdk";
import { act, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { DropDown } from "./drop-down.tsx";
import { ListEntry } from "./list-box.tsx";
import { fire, mount } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A drop-down of two places, around which tree is drawn. */
function twoPlaces(around: (tree: ReactNode) => ReactNode): ReactNode {
  return around(
    <DropDown label="Which" opener="Open" openerClassName="opener" openerNaming={{}}>
      <li>
        <button type="button">one</button>
      </li>
      <li aria-current="page">
        <button type="button">two</button>
      </li>
    </DropDown>,
  );
}

/** Presses Left where focus is, and lets the drop-down's check of it run. */
async function pressLeft(): Promise<void> {
  await fire(
    document.activeElement,
    new KeyboardEvent("keydown", { key: "ArrowLeft", code: "ArrowLeft", bubbles: true }),
  );
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
}

describe("a drop-down", () => {
  afterEach(() => {
    sendFailuresTo(() => undefined);
    vi.restoreAllMocks();
  });

  it("opens on its current place", async () => {
    const mounted = await mount(twoPlaces((tree) => withServices(tree)));
    await fire(
      mounted.container.querySelector(".opener"),
      new MouseEvent("click", { bubbles: true }),
    );
    expect(document.activeElement?.textContent).toBe("two");
    mounted.unmount();
  });

  // The owner's Fire TV, 2026-10-28: Left and Right left the list open, focus on a place.
  it("tells the core's log of a Left that left it open: the key, who took it, and where focus was", async () => {
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const sent: string[] = [];
    sendFailuresTo((text) => sent.push(text));
    // No input layer: nothing turns the key into a command, so nothing closes the list.
    const mounted = await mount(twoPlaces((tree) => tree));
    await fire(
      mounted.container.querySelector(".opener"),
      new MouseEvent("click", { bubbles: true }),
    );
    await pressLeft();
    expect(mounted.container.querySelector("dialog")).not.toBeNull();
    expect(sent).toEqual([
      "Error: a drop-down stayed open at left: key ArrowLeft, keyCode 0, code ArrowLeft, taken before the app false, taken after false, focus on button in the list",
    ]);
    mounted.unmount();
  });

  it("tells nothing of a Left that closed it", async () => {
    const sent: string[] = [];
    sendFailuresTo((text) => sent.push(text));
    const mounted = await mount(twoPlaces((tree) => withServices(tree)));
    await fire(
      mounted.container.querySelector(".opener"),
      new MouseEvent("click", { bubbles: true }),
    );
    await pressLeft();
    expect(mounted.container.querySelector("dialog")).toBeNull();
    expect(sent).toEqual([]);
    mounted.unmount();
  });

  it("tells each opening and closing, and closes as full screen changes (D407 §8)", async () => {
    const told: boolean[] = [];
    const mounted = await mount(
      withServices(
        <DropDown
          label="Which"
          opener="Open"
          openerClassName="opener"
          openerNaming={{}}
          onOpenChange={(isOpen) => told.push(isOpen)}
        >
          <li>
            <button type="button">one</button>
          </li>
        </DropDown>,
      ),
    );
    const opener = mounted.container.querySelector(".opener");
    await fire(opener, new MouseEvent("click", { bubbles: true }));
    expect(mounted.container.querySelector("dialog")).not.toBeNull();
    act(() => {
      document.dispatchEvent(new Event("fullscreenchange"));
    });
    expect(mounted.container.querySelector("dialog")).toBeNull();
    expect(told).toEqual([true, false]);
    mounted.unmount();
  });

  it("stays open at a press of a place marked to keep it open, a stepper's", async () => {
    const mounted = await mount(
      withServices(
        <DropDown label="Which" opener="Open" openerClassName="opener" openerNaming={{}}>
          <ListEntry isKeptOpen>
            <button type="button">later</button>
          </ListEntry>
          <li>
            <button type="button">done</button>
          </li>
        </DropDown>,
      ),
    );
    await fire(
      mounted.container.querySelector(".opener"),
      new MouseEvent("click", { bubbles: true }),
    );
    const buttonNamed = (name: string): Element | undefined =>
      [...mounted.container.querySelectorAll("dialog button")].find(
        (button) => button.textContent === name,
      );
    await fire(buttonNamed("later"), new MouseEvent("click", { bubbles: true }));
    expect(mounted.container.querySelector("dialog")).not.toBeNull();
    await fire(buttonNamed("done"), new MouseEvent("click", { bubbles: true }));
    expect(mounted.container.querySelector("dialog")).toBeNull();
    mounted.unmount();
  });
});

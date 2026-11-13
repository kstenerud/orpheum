// SPDX-License-Identifier: MPL-2.0

/**
 * A pop-up's size and place as a real browser lays them out (popover.tsx, popover.css): as wide as
 * its widest entry, never wider than half the screen nor capped under 16rem, a longer entry wrapping
 * (the owner, 2026-11-03: the banner's menu "is too narrow, and causes word wrap"); clear of a
 * little room at every edge of the screen ("a (very small) margin at the edge of the screen that
 * popups aren't allowed to cover"); and 🔈's grown out of its button, its speaker where the button
 * was and the slider above it.
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { createRef, type ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { AppShell } from "./app-shell.tsx";
import { DropDown } from "./drop-down.tsx";
import { ListOption } from "./list-box.tsx";
import type { PlayerSound } from "./player-controls.tsx";
import { PlayerMenus } from "./player-menus.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/**
 * The room a pop-up keeps from the screen's edge (tokens.css's --screen-edge-room, 0.5rem), where a
 * headless browser has no safe area beside it.
 */
const screenEdgeRoomPx = 8;

/** A pop-up's narrowest cap, 16rem, where half the screen is less. */
const leastCapPx = 256;

/** How far layout's rounding may put a box. */
const roundingPx = 1;

/** An entry far longer than half a computer's screen. */
const longEntry =
  "An entry so very long that no pop-up should widen to hold it on one line, however wide the screen it opens on may happen to be";

/** A film's sound at half, which the test never sets. */
const sound: PlayerSound = {
  volume: 0.5,
  isMuted: false,
  canSetVolume: true,
  setVolume: () => undefined,
  setMuted: () => undefined,
};

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** An element's box, or a failure where it is not drawn. */
function boxOf(element: Element | null | undefined): DOMRect {
  if (element === null || element === undefined) {
    throw new Error("an element the test measures is not drawn");
  }
  return element.getBoundingClientRect();
}

/** The open pop-up. */
function popUp(): HTMLDialogElement {
  const dialog = document.querySelector<HTMLDialogElement>("dialog[open]");
  if (dialog === null) {
    throw new Error("no pop-up is open");
  }
  return dialog;
}

/** An entry's own size laid out unwrapped, its width max-content for the measure alone. */
function unwrappedSizeOf(entry: HTMLElement): { width: number; height: number } {
  const { style } = entry;
  style.setProperty("width", "max-content");
  const { width, height } = entry.getBoundingClientRect();
  style.removeProperty("width");
  return { width, height };
}

/** A drop-down menu at a spot on the screen, its entries those given. */
function menuAt(spot: Readonly<Record<string, string>>, entries: readonly string[]): ReactNode {
  return withServices(
    <div style={{ position: "fixed", ...spot }}>
      <DropDown
        label="Menu"
        opener={<span>Menu</span>}
        openerClassName="test-opener"
        openerNaming={{ "aria-label": "Menu" }}
        isFromItsTop
      >
        {entries.map((entry) => (
          <ListOption key={entry} onChoose={() => undefined}>
            {entry}
          </ListOption>
        ))}
      </DropDown>
    </div>,
  );
}

/** Expects box inside the screen less the room at each of its edges. */
function expectClearOfTheEdges(box: DOMRect, width: number, height: number): void {
  expect(box.left).toBeGreaterThanOrEqual(screenEdgeRoomPx - roundingPx);
  expect(box.top).toBeGreaterThanOrEqual(screenEdgeRoomPx - roundingPx);
  expect(box.right).toBeLessThanOrEqual(width - screenEdgeRoomPx + roundingPx);
  expect(box.bottom).toBeLessThanOrEqual(height - screenEdgeRoomPx + roundingPx);
}

describe("a pop-up's width, laid out by a real browser", () => {
  // The owner, 2026-11-03: "The top-right menu is too narrow, and causes word wrap on the
  // selectable items." Sized to fit the room left of its place at the screen's right edge, it
  // shrank below its entries.
  it("opens the banner's menu at the screen's top right as wide as its widest choice, none wrapping", async () => {
    await page.viewport(1280, 800);
    mounted = await mount(
      withServices(
        <AppShell
          isAdmin={true}
          canHostWatchTogether={true}
          currentScreen="films"
          onSignOut={() => undefined}
          onSwitchHousehold={() => undefined}
          signedInAs={{ name: "owner", deviceName: "the living room's television" }}
          realm="staging"
        >
          <p>screen</p>
        </AppShell>,
      ),
    );
    await userEvent.click(page.getByRole("button", { name: "Menu" }));
    const entries = [...popUp().querySelectorAll<HTMLElement>(".list-box__option")];
    expect(entries.length).toBeGreaterThan(2);
    const widest = Math.max(...entries.map((entry) => unwrappedSizeOf(entry).width));
    for (const entry of entries) {
      const laid = boxOf(entry);
      expect(laid.width, entry.textContent).toBeCloseTo(widest, 0);
      expect(laid.height, entry.textContent).toBeCloseTo(unwrappedSizeOf(entry).height, 0);
    }
    expectClearOfTheEdges(boxOf(popUp()), window.innerWidth, window.innerHeight);
  });

  // The owner, 2026-11-03: "the part that says 'admin on Firefox on Mac' is fine to wrap, since it's
  // just informative content. It's the stuff underneath that I don't want wrapping."
  it("wraps who is signed in where to the width its choices set, never widening the menu for it", async () => {
    await page.viewport(1280, 800);
    mounted = await mount(
      withServices(
        <AppShell
          isAdmin={false}
          canHostWatchTogether={false}
          currentScreen="films"
          onSignOut={() => undefined}
          signedInAs={{
            name: "owner",
            deviceName: "Firefox on the Mac in the study at the top of the stairs",
          }}
        >
          <p>screen</p>
        </AppShell>,
      ),
    );
    await userEvent.click(page.getByRole("button", { name: "Menu" }));
    const choices = [...popUp().querySelectorAll<HTMLElement>(".list-box__option")];
    const widest = Math.max(...choices.map((choice) => unwrappedSizeOf(choice).width));
    for (const choice of choices) {
      expect(boxOf(choice).width, choice.textContent).toBeCloseTo(widest, 0);
    }
    const who = popUp().querySelector<HTMLElement>(".app-shell__who");
    const oneLine = boxOf(choices[0]).height;
    expect(boxOf(who).width).toBeCloseTo(widest, 0);
    expect(boxOf(who).height).toBeGreaterThan(oneLine * 1.5);
  });

  // The owner, 2026-11-03: it "definitely shouldn't occupy more than half the screen, but it also
  // shouldn't widen unnecessarily for an extremely long item (that could word-wrap to look nicer)".
  it("caps a menu with a very long entry at half the screen, the entry wrapping", async () => {
    await page.viewport(1280, 800);
    mounted = await mount(menuAt({ left: "40%", top: "30%" }, ["Home", longEntry]));
    await userEvent.click(page.getByRole("button", { name: "Menu" }));
    expect(Math.abs(boxOf(popUp()).width - 640)).toBeLessThanOrEqual(roundingPx);
    const [short, long] = [...popUp().querySelectorAll(".list-box__option")];
    expect(boxOf(long).height).toBeGreaterThan(boxOf(short).height * 1.5);
  });

  // Half a phone's screen is too narrow for a word: the cap is never under 16rem, and the room at
  // the screen's edges still wins.
  it("caps it at 16rem on a phone's screen, inside the room at its edges", async () => {
    await page.viewport(400, 740);
    mounted = await mount(menuAt({ right: "0px", top: "0px" }, ["Home", longEntry]));
    await userEvent.click(page.getByRole("button", { name: "Menu" }));
    const box = boxOf(popUp());
    expect(Math.abs(box.width - leastCapPx)).toBeLessThanOrEqual(roundingPx);
    expectClearOfTheEdges(box, 400, 740);
  });
});

/** The screen's corners, a button pressed against each. */
const corners = [
  { name: "top left", spot: { left: "0px", top: "0px" } },
  { name: "top right", spot: { right: "0px", top: "0px" } },
  { name: "bottom left", spot: { left: "0px", bottom: "0px" } },
  { name: "bottom right", spot: { right: "0px", bottom: "0px" } },
] as const;

/** A computer's screen and a phone's. */
const screens = [
  { name: "a computer's", width: 1280, height: 800 },
  { name: "a phone's", width: 400, height: 740 },
] as const;

describe.each(screens)("a pop-up on $name screen", ({ width, height }) => {
  it.each(corners)(
    "keeps clear of the room at every edge, its button at the $name",
    async ({ spot }) => {
      await page.viewport(width, height);
      mounted = await mount(menuAt(spot, ["Home", "Settings", longEntry, "Sign out"]));
      await userEvent.click(page.getByRole("button", { name: "Menu" }));
      expectClearOfTheEdges(boxOf(popUp()), width, height);
      await userEvent.keyboard("{Escape}");
      mounted.unmount();
      mounted = await mount(
        withServices(
          <div style={{ position: "fixed", ...spot }}>
            <PlayerMenus
              videoRef={createRef()}
              shown={["sound"]}
              sound={sound}
              onOpenChange={() => undefined}
            />
          </div>,
        ),
      );
      await userEvent.click(page.getByRole("button", { name: "Volume" }));
      expectClearOfTheEdges(boxOf(popUp()), width, height);
    },
  );
});

/** 🔈 at a spot on the screen, opened. */
async function volumeOpenedAt(spot: Readonly<Record<string, string>>): Promise<{
  readonly opener: DOMRect;
  readonly speaker: DOMRect;
  readonly slider: DOMRect;
}> {
  mounted = await mount(
    withServices(
      <div style={{ position: "fixed", ...spot }}>
        <PlayerMenus
          videoRef={createRef()}
          shown={["sound"]}
          sound={sound}
          onOpenChange={() => undefined}
        />
      </div>,
    ),
  );
  await userEvent.click(page.getByRole("button", { name: "Volume" }));
  return {
    // Measured open: the page's scrollbar's room, kept while a pop-up is open (modal-layer.css),
    // may move it.
    opener: boxOf(document.querySelector('[aria-haspopup][aria-label="Volume"]')),
    speaker: boxOf(popUp().querySelector('button[aria-label="Mute"]')),
    slider: boxOf(popUp().querySelector(".volume-range")),
  };
}

/** A box's centre. */
function centreOf(box: DOMRect): { x: number; y: number } {
  return { x: box.left + box.width / 2, y: box.top + box.height / 2 };
}

describe("🔈's pop-up, laid out by a real browser", () => {
  // The owner, 2026-11-03: "the speaker icon inside the slider ends up physically at a similar
  // location to the button it spawned from ... clicking it expanded the control such that the
  // slider is above the button".
  it("grows out of its button: its speaker over the button, the button's size, the slider above", async () => {
    await page.viewport(1280, 800);
    const { opener, speaker, slider } = await volumeOpenedAt({ right: "64px", bottom: "32px" });
    expect(Math.abs(centreOf(speaker).x - centreOf(opener).x)).toBeLessThanOrEqual(roundingPx);
    expect(Math.abs(centreOf(speaker).y - centreOf(opener).y)).toBeLessThanOrEqual(roundingPx);
    expect(speaker.width).toBeCloseTo(opener.width, 0);
    expect(speaker.height).toBeCloseTo(opener.height, 0);
    expect(slider.bottom).toBeLessThanOrEqual(speaker.top + roundingPx);
    // Focus opens on the speaker, where the button was.
    expect(document.activeElement?.getAttribute("aria-label")).toBe("Mute");
    expectClearOfTheEdges(boxOf(popUp()), 1280, 800);
  });

  it("opens below its button's top as any pop-up, where there is no room above it", async () => {
    await page.viewport(1280, 800);
    const { opener } = await volumeOpenedAt({ right: "64px", top: "16px" });
    const box = boxOf(popUp());
    expect(Math.abs(box.top - opener.top)).toBeLessThanOrEqual(roundingPx);
    expect(box.bottom).toBeGreaterThan(opener.bottom);
    expectClearOfTheEdges(box, 1280, 800);
  });
});

// R5 of ui-audit-2026-11-04.md: a pop-up closed any way gives focus back to the button that opened it.
describe("a pop-up closed", () => {
  it("gives focus back to its button, closed by Back, by a press outside and by choosing", async () => {
    await page.viewport(1280, 800);
    mounted = await mount(menuAt({ left: "40px", top: "40px" }, ["One", "Two"]));
    const opener = document.querySelector<HTMLElement>(".test-opener");
    const opened = async (): Promise<void> => {
      opener?.focus();
      // Opened by click(), not by Enter: the browser's own Enter click is not what this test is of,
      // and Firefox now and then never dispatches it (idiosyncrasies.md, "Firefox now and then
      // drops the click of an Enter on a button").
      opener?.click();
      await expect.poll(() => document.activeElement?.closest("dialog[open]")).not.toBeNull();
    };
    await opened();
    await userEvent.keyboard("{Escape}");
    await expect.poll(() => document.querySelector("dialog[open]")).toBeNull();
    expect(document.activeElement).toBe(opener);
    await opened();
    document
      .querySelector("dialog[open]")
      ?.dispatchEvent(new MouseEvent("click", { bubbles: true, clientX: 1270, clientY: 790 }));
    await expect.poll(() => document.querySelector("dialog[open]")).toBeNull();
    expect(document.activeElement).toBe(opener);
    await opened();
    // Down moves to the second item; it is chosen by click(), not by Enter, for the reason above.
    const first = document.activeElement;
    await userEvent.keyboard("{ArrowDown}");
    await expect.poll(() => document.activeElement).not.toBe(first);
    (document.activeElement as HTMLElement | null)?.click();
    await expect.poll(() => document.querySelector("dialog[open]")).toBeNull();
    expect(document.activeElement).toBe(opener);
  });
});

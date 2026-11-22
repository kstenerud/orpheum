// SPDX-License-Identifier: MPL-2.0

/**
 * The form's choices walked by a mouse, a finger, a keyboard and a remote's keys in a real browser,
 * at a phone's, a computer's and a television's width (D538, interface rules 7 and 14): a switch
 * (switch.tsx), toggled by every one of them, its focus ring on its whole row.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";

import { useState, type JSX } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { startInput, type Input } from "./input-commands.ts";
import { Switch } from "./switch.tsx";
import { isSwitchOn, mount, switchNamed, type Mounted } from "./test-dom.tsx";
import { focusedName, isRingShown, partWidths, tapOn } from "./test-parts.ts";

/** A button above a switch and a button below, as a column of settings has them. */
function SwitchColumn(): JSX.Element {
  const [isOn, setIsOn] = useState(false);
  return (
    <main>
      <button type="button">Above</button>
      <Switch label="Level loudness" isOn={isOn} onSwitch={setIsOn} />
      <button type="button">Below</button>
    </main>
  );
}

let input: Input | undefined;
let mounted: Mounted | undefined;

beforeEach(() => {
  input = startInput({ goBack: () => false, isNavigating: () => true });
});

afterEach(() => {
  input?.stop();
  input = undefined;
  mounted?.unmount();
  mounted = undefined;
});

/** The mounted switch. */
function theSwitch(): HTMLElement {
  const found = switchNamed(document.body, "Level loudness");
  if (found === undefined) {
    throw new Error("no switch");
  }
  return found;
}

/** The box of the part of within matching selector. */
function boxOf(within: Element, selector: string): DOMRect {
  const found = within.querySelector(selector);
  if (found === null) {
    throw new Error(`no ${selector}`);
  }
  return found.getBoundingClientRect();
}

describe("a switch, by every way of pressing it", () => {
  beforeEach(async () => {
    await page.viewport(960, 480);
    mounted = await mount(<SwitchColumn />);
  });

  it("toggles by a click and by a tap, anywhere on its row", async () => {
    await userEvent.click(theSwitch(), { position: { x: 4, y: 4 } });
    expect(isSwitchOn(theSwitch())).toBe(true);
    tapOn(theSwitch().querySelector(".switch__track"));
    expect(isSwitchOn(theSwitch())).toBe(false);
  });

  it("is reached by Tab, its ring on it, and toggled by Space and by Enter", async () => {
    document.querySelector<HTMLButtonElement>("button")?.focus();
    await userEvent.keyboard("{Tab}");
    expect(focusedName()).toBe("Level loudness");
    expect(isRingShown()).toBe(true);
    await userEvent.keyboard(" ");
    expect(isSwitchOn(theSwitch())).toBe(true);
    await userEvent.keyboard("{Enter}");
    expect(isSwitchOn(theSwitch())).toBe(false);
    await userEvent.keyboard("{Tab}");
    expect(focusedName()).toBe("Below");
  });

  it("is one stop for a remote: Down reaches it, OK toggles it, Down and Up leave it", async () => {
    document.querySelector<HTMLButtonElement>("button")?.focus();
    await userEvent.keyboard("{ArrowDown}");
    expect(document.activeElement).toBe(theSwitch());
    expect(isRingShown()).toBe(true);
    await userEvent.keyboard("{Enter}");
    expect(isSwitchOn(theSwitch())).toBe(true);
    expect(document.activeElement).toBe(theSwitch());
    await userEvent.keyboard("{ArrowDown}");
    expect(focusedName()).toBe("Below");
    await userEvent.keyboard("{ArrowUp}");
    await userEvent.keyboard("{ArrowUp}");
    expect(focusedName()).toBe("Above");
  });

  it("lays out its label at its start and its track at its end, a fingertip tall, at each width", async () => {
    for (const width of partWidths) {
      await page.viewport(width, 800);
      const row = theSwitch().getBoundingClientRect();
      const label = boxOf(theSwitch(), ".switch__label");
      const track = boxOf(theSwitch(), ".switch__track");
      expect(row.height, `${String(width)} px`).toBeGreaterThanOrEqual(44);
      expect(row.right).toBeLessThanOrEqual(width);
      expect(label.right).toBeLessThanOrEqual(track.left);
      expect(Math.abs(track.right - row.right)).toBeLessThanOrEqual(13);
      expect(track.width).toBeCloseTo(40, 0);
      expect(track.height).toBeCloseTo(24, 0);
    }
  });

  it("moves its knob to the track's end when on, its state not carried by colour alone", async () => {
    const knob = (): DOMRect | undefined =>
      theSwitch().querySelector(".switch__knob")?.getBoundingClientRect();
    const offAt = knob()?.left ?? 0;
    await userEvent.click(theSwitch());
    await expect.poll(() => (knob()?.left ?? 0) - offAt).toBeCloseTo(16, 0);
  });
});

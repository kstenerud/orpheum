// SPDX-License-Identifier: MPL-2.0

/**
 * A Watch Together guest's Cast (together-guest-cast.tsx) as a real browser runs it: a quick choice,
 * so a pop-up at its opener, not a dialog (dialogs-spec, D538 §3). A click, a tap, the keyboard and a
 * remote's arrows open it at Cast, focus on its first choice, the ring showing; choosing Chromecast
 * closes it, focus back on Cast, and a failed cast is said under Cast; Escape leaves it as it was.
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import type { TogetherSnapshot } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { startInput, type Input } from "./input-commands.ts";
import { GuestsCastEntry, type GuestsCast } from "./together-guest-cast.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { focusedName, isRingShown, partWidths, tapOn } from "./test-parts.ts";
import { withServices } from "./test-services.tsx";

const connected = { status: "connected" } as TogetherSnapshot;

/** How many casts were started, and whether the next one fails. */
const casts = { started: 0, shouldFail: false };

/** A guest's cast that starts by counting, failing where told, as useGuestsCast's would. */
function GuestsCastHere(): JSX.Element {
  const [isChoosing, setIsChoosing] = useState(false);
  const [problem, setProblem] = useState<string | undefined>(undefined);
  const cast: GuestsCast = {
    isOffered: true,
    castingTo: undefined,
    isChoosing,
    setIsChoosing,
    isBusy: false,
    problem,
    start: () => {
      casts.started += 1;
      setProblem(casts.shouldFail ? "The Chromecast could not take the cast." : undefined);
    },
    stop: () => undefined,
    airPlayGrants: undefined,
  };
  return (
    <main style={{ display: "flex", flexDirection: "column", alignItems: "start" }}>
      <button type="button">Above</button>
      <GuestsCastEntry cast={cast} snapshot={connected} />
    </main>
  );
}

let input: Input | undefined;
let mounted: Mounted | undefined;

beforeEach(async () => {
  input = startInput({ goBack: () => false, isNavigating: () => true });
  casts.started = 0;
  casts.shouldFail = false;
  await page.viewport(1280, 800);
  mounted = await mount(withServices(<GuestsCastHere />));
});

afterEach(() => {
  input?.stop();
  input = undefined;
  mounted?.unmount();
  mounted = undefined;
});

/** The button named text, wherever it shows. */
function button(text: string): HTMLElement {
  const found = [...document.querySelectorAll<HTMLElement>("button")].find(
    (each) => each.textContent?.trim() === text,
  );
  if (found === undefined) {
    throw new Error(`no button ${text}`);
  }
  return found;
}

/** The open pop-up, if one is. */
function popUp(): HTMLDialogElement | null {
  return document.querySelector<HTMLDialogElement>("dialog[open]");
}

describe("a guest's Cast, by every way of pressing it", () => {
  it("opens a pop-up at Cast at a click, not a dialog, and casts at a click on Chromecast, closing it", async () => {
    await userEvent.click(button("Cast"));
    const opened = popUp();
    expect(opened?.classList.contains("popover")).toBe(true);
    expect(opened?.classList.contains("modal-dialog")).toBe(false);
    expect(opened?.getAttribute("aria-label")).toBe("Cast to");
    expect(button("Cast").getAttribute("aria-expanded")).toBe("true");
    await userEvent.click(button("Chromecast"));
    expect(casts.started).toBe(1);
    expect(popUp()).toBeNull();
    expect(document.activeElement).toBe(button("Cast"));
  });

  it("opens and casts at a tap", async () => {
    tapOn(button("Cast"));
    await expect.poll(popUp).not.toBeNull();
    tapOn(button("Chromecast"));
    await expect.poll(() => casts.started).toBe(1);
    expect(popUp()).toBeNull();
  });

  it("is walked by a keyboard: Enter opens it on Chromecast, Escape leaves it, focus back on Cast", async () => {
    button("Cast").focus();
    await userEvent.keyboard("{Enter}");
    expect(popUp()).not.toBeNull();
    expect(focusedName()).toBe("Chromecast");
    await userEvent.keyboard("{Escape}");
    expect(popUp()).toBeNull();
    expect(casts.started).toBe(0);
    expect(document.activeElement).toBe(button("Cast"));
  });

  it("is one stop for a remote: Down reaches Cast, OK opens it, OK casts, and a failure is said under Cast", async () => {
    casts.shouldFail = true;
    button("Above").focus();
    await userEvent.keyboard("{ArrowDown}");
    expect(document.activeElement).toBe(button("Cast"));
    await userEvent.keyboard("{Enter}");
    expect(focusedName()).toBe("Chromecast");
    expect(isRingShown()).toBe(true);
    await userEvent.keyboard("{Enter}");
    expect(casts.started).toBe(1);
    expect(popUp()).toBeNull();
    expect(document.activeElement).toBe(button("Cast"));
    expect(document.querySelector("main")?.textContent).toContain(
      "The Chromecast could not take the cast.",
    );
  });

  it("opens beside Cast and inside the screen at each width", async () => {
    for (const width of partWidths) {
      await page.viewport(width, 800);
      await userEvent.click(button("Cast"));
      const box = button("Chromecast").getBoundingClientRect();
      expect(box.left, `${String(width)} px`).toBeGreaterThanOrEqual(0);
      expect(box.right, `${String(width)} px`).toBeLessThanOrEqual(width);
      expect(box.height, `${String(width)} px`).toBeGreaterThanOrEqual(44);
      await userEvent.keyboard("{Escape}");
    }
  });
});

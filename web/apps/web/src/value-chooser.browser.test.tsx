// SPDX-License-Identifier: MPL-2.0

/**
 * The pop-up choosers a form chooses a value in, ValueChooser and RoleChooser (D538 §3), walked by
 * a mouse, a finger, a keyboard and a remote's keys in a real browser, at a phone's, a computer's
 * and a television's width: its button opens its list at it, the current choice marked; a choice
 * chosen is told and shown on the button, the list closed and focus back on the button; Escape, a
 * remote's Back, leaves it as it was.
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { useState, type JSX } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { startInput, type Input } from "./input-commands.ts";
import { RoleChooser } from "./people-dialogs.tsx";
import { ValueChooser } from "./route-chooser.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { focusedName, partWidths, tapOn } from "./test-parts.ts";

const qualities = [
  { value: 480, label: "480p" },
  { value: 720, label: "720p" },
  { value: 1080, label: "1080p" },
] as const;

/** What each chooser was last told, by its field. */
const chosen: { quality?: number; role?: string } = {};

/** A button above the two choosers, as a column of settings has them. */
function Choosers(): JSX.Element {
  const [quality, setQuality] = useState<number>(720);
  const [role, setRole] = useState("friends");
  return (
    <main style={{ display: "flex", flexDirection: "column", alignItems: "start" }}>
      <button type="button">Above</button>
      <ValueChooser
        fieldLabel="Quality"
        choices={qualities}
        current={quality}
        onChoose={(value) => {
          chosen.quality = value;
          setQuality(value);
        }}
      />
      <RoleChooser
        roleNames={["friends", "family"]}
        role={role}
        onChoose={(value) => {
          chosen.role = value;
          setRole(value);
        }}
      />
    </main>
  );
}

let input: Input | undefined;
let mounted: Mounted | undefined;

beforeEach(async () => {
  input = startInput({ goBack: () => false, isNavigating: () => true });
  delete chosen.quality;
  delete chosen.role;
  await page.viewport(1280, 800);
  mounted = await mount(<Choosers />);
});

afterEach(() => {
  input?.stop();
  input = undefined;
  mounted?.unmount();
  mounted = undefined;
});

/** The opener of the chooser of field. */
function openerOf(field: string): HTMLElement {
  const chooser = [...document.querySelectorAll(".route-chooser")].find(
    (each) => each.querySelector(".route-chooser__field")?.textContent === field,
  );
  const opener = chooser?.querySelector<HTMLElement>("[aria-haspopup]");
  if (opener === null || opener === undefined) {
    throw new Error(`no chooser of ${field}`);
  }
  return opener;
}

/** The open list's choices. */
function choices(): HTMLElement[] {
  return [...document.querySelectorAll<HTMLElement>("dialog[open] .list-box__option")];
}

/** The open list's choice that reads text. */
function choiceOf(text: string): HTMLElement {
  const found = choices().find((each) => each.textContent === text);
  if (found === undefined) {
    throw new Error(`no choice ${text}`);
  }
  return found;
}

/** Whether a list is open. */
function isOpen(): boolean {
  return document.querySelector("dialog[open]") !== null;
}

describe("a value chooser, by every way of pressing it", () => {
  it("opens at a click, the current choice marked, and a click on another chooses it", async () => {
    await userEvent.click(openerOf("Quality"));
    expect(isOpen()).toBe(true);
    expect(choices().map((each) => each.textContent)).toEqual(["480p", "720p", "1080p"]);
    expect(choiceOf("720p").getAttribute("aria-current")).toBe("true");
    await userEvent.click(choiceOf("1080p"));
    expect(chosen.quality).toBe(1080);
    expect(isOpen()).toBe(false);
    expect(openerOf("Quality").textContent).toBe("1080p");
  });

  it("opens at a tap and chooses at a tap", async () => {
    tapOn(openerOf("Quality"));
    await expect.poll(isOpen).toBe(true);
    tapOn(choiceOf("480p"));
    await expect.poll(() => chosen.quality).toBe(480);
    expect(openerOf("Quality").textContent).toBe("480p");
  });

  it("is walked by a keyboard: Enter opens it on the current choice, Down and Enter choose, Escape leaves it", async () => {
    openerOf("Quality").focus();
    await userEvent.keyboard("{Enter}");
    expect(isOpen()).toBe(true);
    expect(focusedName()).toBe("720p");
    await userEvent.keyboard("{Escape}");
    expect(isOpen()).toBe(false);
    expect(chosen.quality).toBeUndefined();
    expect(document.activeElement).toBe(openerOf("Quality"));
    await userEvent.keyboard("{Enter}");
    await userEvent.keyboard("{ArrowDown}");
    expect(focusedName()).toBe("1080p");
    await userEvent.keyboard("{Enter}");
    expect(chosen.quality).toBe(1080);
    expect(isOpen()).toBe(false);
    expect(document.activeElement).toBe(openerOf("Quality"));
  });

  it("is one stop for a remote: Down reaches it, OK opens it, Up and OK choose", async () => {
    document.querySelector<HTMLButtonElement>("button")?.focus();
    await userEvent.keyboard("{ArrowDown}");
    expect(document.activeElement).toBe(openerOf("Quality"));
    await userEvent.keyboard("{Enter}");
    await userEvent.keyboard("{ArrowUp}");
    expect(focusedName()).toBe("480p");
    await userEvent.keyboard("{Enter}");
    expect(chosen.quality).toBe(480);
    expect(document.activeElement).toBe(openerOf("Quality"));
    await userEvent.keyboard("{ArrowDown}");
    expect(document.activeElement).toBe(openerOf("Role"));
  });

  it("opens its list inside the screen at each width, each choice a fingertip tall and no taller (ui.md §2)", async () => {
    for (const width of partWidths) {
      await page.viewport(width, 800);
      await userEvent.click(openerOf("Quality"));
      for (const choice of choices()) {
        const box = choice.getBoundingClientRect();
        expect(box.left, `${String(width)} px`).toBeGreaterThanOrEqual(0);
        expect(box.right).toBeLessThanOrEqual(width);
        expect(box.height, `${String(width)} px`).toBeGreaterThanOrEqual(44);
        expect(box.height, `${String(width)} px`).toBeLessThan(52);
      }
      await userEvent.keyboard("{Escape}");
    }
  });
});

describe("a role chooser, by every way of pressing it", () => {
  it("draws each role a fingertip tall at a phone's width (ui.md §2)", async () => {
    await page.viewport(360, 800);
    await userEvent.click(openerOf("Role"));
    expect(choices()).toHaveLength(2);
    for (const choice of choices()) {
      expect(choice.getBoundingClientRect().height).toBeGreaterThanOrEqual(44);
    }
  });

  it("offers the household's roles, the person's marked, and gives another by a click or a tap", async () => {
    await userEvent.click(openerOf("Role"));
    expect(choices().map((each) => each.textContent)).toEqual(["friends", "family"]);
    expect(choiceOf("friends").getAttribute("aria-current")).toBe("true");
    await userEvent.click(choiceOf("family"));
    expect(chosen.role).toBe("family");
    expect(openerOf("Role").textContent).toBe("family");
    tapOn(openerOf("Role"));
    await expect.poll(isOpen).toBe(true);
    tapOn(choiceOf("friends"));
    await expect.poll(() => chosen.role).toBe("friends");
  });

  it("is walked by a keyboard and a remote alike: OK opens it, Down and OK give the role, Back leaves it", async () => {
    openerOf("Role").focus();
    await userEvent.keyboard("{Enter}");
    expect(focusedName()).toBe("friends");
    await userEvent.keyboard("{ArrowDown}");
    await userEvent.keyboard("{Enter}");
    expect(chosen.role).toBe("family");
    expect(document.activeElement).toBe(openerOf("Role"));
    await userEvent.keyboard("{Enter}");
    await userEvent.keyboard("{Escape}");
    expect(isOpen()).toBe(false);
    expect(chosen.role).toBe("family");
  });
});

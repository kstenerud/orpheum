// SPDX-License-Identifier: MPL-2.0

/**
 * The file browser's path bar as a real browser lays it out (D539 §1, file-browser-spec): on a wide
 * browser, crumbs that do not fit on one line fold their middle ones into one "…", the place and the
 * last two still shown, which opens a menu of them by a click, a tap, the keyboard and a remote's
 * arrows and OK; a path that fits does not fold; on a narrow one, the folder's name opens a menu of
 * every folder above, the nearest first.
 */

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { afterEach, beforeEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { FileBrowser, type Crumb } from "./file-browser.tsx";
import { startInput, type Input } from "./input-commands.ts";
import { mount, type Mounted } from "./test-dom.tsx";
import { focusedName, isRingShown, tapOn } from "./test-parts.ts";
import { withServices } from "./test-services.tsx";

let input: Input | undefined;
let mounted: Mounted | undefined;
/** The crumbs opened, by name. */
let opened: string[] = [];

/** Crumbs of these names, each telling opened it was pressed. */
function crumbsOf(names: readonly string[]): readonly Crumb[] {
  return names.map((name) => ({
    key: name,
    name,
    onOpen: () => {
      opened.push(name);
    },
  }));
}

const longPath = [
  "Media server",
  "a folder of a rather long name",
  "another folder of a long name",
  "a third folder of a long name",
  "a fourth folder of a long name",
  "the folder above",
  "the folder here",
];

/** The file browser at path, its list empty. */
async function browserAt(path: readonly string[]): Promise<void> {
  mounted = await mount(
    withServices(
      <main>
        <FileBrowser
          use="browse"
          crumbs={crumbsOf(path)}
          listLabel={path.at(-1) ?? ""}
          shown={{
            kind: "listed",
            groups: [{ key: "here", rows: [] }],
            empty: { kind: "empty", text: "Nothing here" },
          }}
        />
      </main>,
    ),
  );
}

/** The names the wide bar shows, in order. */
function wideCrumbs(): string[] {
  return [...document.querySelectorAll(".file-browser__crumbs li")].map((each) => {
    const pressed = each.querySelector("button, [role='button']");
    return (pressed?.getAttribute("aria-label") ?? pressed?.textContent ?? each.textContent).trim();
  });
}

/** The button named name: a button, or a drop-down's opener, a button by its role. */
function button(name: string): HTMLElement {
  const found = [...document.querySelectorAll<HTMLElement>("button, [role='button']")].find(
    (each) => (each.getAttribute("aria-label") ?? each.textContent.trim()) === name,
  );
  if (found === undefined) {
    throw new Error(`no button ${name}`);
  }
  return found;
}

/** The entries of the menu open, by name. */
function menuEntries(): string[] {
  return [...document.querySelectorAll("dialog[open] .list-box__option")].map((each) =>
    each.textContent.trim(),
  );
}

beforeEach(async () => {
  input = startInput({ goBack: () => false, isNavigating: () => true });
  opened = [];
  await page.viewport(1280, 800);
});

afterEach(() => {
  input?.stop();
  input = undefined;
  mounted?.unmount();
  mounted = undefined;
});

describe("the file browser's path bar", () => {
  it("folds the middle crumbs of a path too long for a line into one …, the place and the last two shown", async () => {
    await browserAt(longPath);
    await expect
      .poll(wideCrumbs)
      .toEqual(["Media server", "Folders between", "the folder above", "the folder here"]);
    await userEvent.click(button("Folders between"));
    await expect.poll(menuEntries).toEqual(longPath.slice(1, -2));
    await userEvent.click(button("another folder of a long name"));
    expect(opened).toEqual(["another folder of a long name"]);
  });

  it("opens the … by a tap", async () => {
    await browserAt(longPath);
    await expect.poll(wideCrumbs).toContain("Folders between");
    tapOn(button("Folders between"));
    await expect.poll(menuEntries).toEqual(longPath.slice(1, -2));
  });

  it("opens the … by the keyboard and a remote: OK on it, Down, OK opens that folder, focus never on nothing", async () => {
    await browserAt(longPath);
    await expect.poll(wideCrumbs).toContain("Folders between");
    button("Media server").focus();
    await userEvent.keyboard("{ArrowRight}");
    expect(focusedName()).toBe("Folders between");
    expect(isRingShown()).toBe(true);
    await userEvent.keyboard("{Enter}");
    await expect.poll(focusedName).toBe("a folder of a rather long name");
    await userEvent.keyboard("{ArrowDown}");
    expect(focusedName()).toBe("another folder of a long name");
    expect(isRingShown()).toBe(true);
    await userEvent.keyboard("{Enter}");
    expect(opened).toEqual(["another folder of a long name"]);
    expect(document.activeElement).not.toBe(document.body);
  });

  it("does not fold a path that fits", async () => {
    await browserAt(["Media server", "Films", "Old", "Silent"]);
    expect(wideCrumbs()).toEqual(["Media server", "Films", "Old", "Silent"]);
  });

  it("on a narrow browser, opens from the folder's name a menu of every folder above, the nearest first", async () => {
    await page.viewport(360, 800);
    await browserAt(["Media server", "Films", "Old", "Silent"]);
    await userEvent.click(button("Folders above Silent"));
    expect(menuEntries()).toEqual(["Old", "Films", "Media server"]);
    const box = document.querySelector("dialog[open]")?.getBoundingClientRect();
    expect(box?.right ?? 361).toBeLessThanOrEqual(360);
    // The menu's Films, not the wide bar's, which a narrow browser does not draw.
    const films = [
      ...document.querySelectorAll<HTMLElement>("dialog[open] .list-box__option"),
    ].find((each) => each.textContent.trim() === "Films");
    await userEvent.click(films ?? document.body);
    expect(opened).toEqual(["Films"]);
  });
});

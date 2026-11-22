// SPDX-License-Identifier: MPL-2.0

/**
 * The picker screen, the banners and the status notes (D538), as a real browser lays them out and as
 * a mouse, a finger, a keyboard and a remote use them, at a phone's, a laptop's and a television's
 * width: focus always on something, its ring showing where keys put it there.
 */

import "./i18n.ts";
import "./tokens.css";
import "./base.css";

import { useState, type JSX } from "react";
import { renderToStaticMarkup } from "react-dom/server";
import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { Banners, type BannerNotice } from "./banner.tsx";
import { DropDown } from "./drop-down.tsx";
import { ListOption, ListSubmenu, PickerScreen } from "./list-box.tsx";
import { StatusNote } from "./status-note.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

const widths = [360, 1280, 1920] as const;
const ways = ["mouse", "touch", "keyboard", "remote"] as const;
type Way = (typeof ways)[number];

let mounted: Mounted | undefined;
afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** Whether focus shows its ring on element, as a keyboard or a remote put it there. */
function ringShows(element: Element | null): boolean {
  if (!(element instanceof HTMLElement) || !element.matches(":focus-visible")) {
    return false;
  }
  const style = getComputedStyle(element);
  return style.outlineStyle !== "none" || style.boxShadow !== "none";
}

/** Whether focus is on a control, never on nothing. */
function isFocusOnSomething(): boolean {
  const at = document.activeElement;
  return at !== null && at !== document.body && at !== document.documentElement;
}

/** The element matching selector, or a failure where it is not drawn. */
function drawn(selector: string): HTMLElement {
  const found = document.querySelector<HTMLElement>(selector);
  if (found === null) {
    throw new Error(`${selector} is not drawn`);
  }
  return found;
}

/** Presses button the way given: a click, a tap, Enter, or a remote's OK (Enter alone). */
async function press(button: HTMLElement, way: Way): Promise<void> {
  if (way === "mouse") {
    await userEvent.click(button);
    return;
  }
  if (way === "touch") {
    button.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true, pointerType: "touch" }));
    button.focus();
    button.click();
    return;
  }
  // A key first, so the focus that follows shows as a key's; then the click an Enter makes, which
  // Firefox now and then drops when sent as a key (idiosyncrasies.md).
  await userEvent.keyboard("{Shift}");
  button.focus();
  button.click();
}

const choices = Array.from({ length: 30 }, (_, at) => `Choice ${String(at + 1)}`);

/** A menu whose second entry opens a picker screen of thirty choices, the third current. */
function MenuWithPicker(): JSX.Element {
  return (
    <DropDown label="Menu" opener="Menu" openerClassName="opener" openerNaming={{}} isFromItsTop>
      <ListOption onChoose={() => undefined}>Start over</ListOption>
      <ListSubmenu name="Order" value="Choice 3">
        {choices.map((choice, at) => (
          <ListOption key={choice} isCurrent={at === 2} onChoose={() => undefined}>
            {choice}
          </ListOption>
        ))}
      </ListSubmenu>
    </DropDown>
  );
}

describe("a picker screen (D538)", () => {
  for (const width of widths) {
    for (const way of ways) {
      it(`takes its menu's place with Back kept at its top, and Back returns, by ${way} at ${String(width)}px`, async () => {
        await page.viewport(width, 600);
        mounted = await mount(withServices(<MenuWithPicker />));
        await press(drawn(".opener"), way);
        const entry = drawn(".list-box__submenu-entry");
        if (way === "remote") {
          await userEvent.keyboard("{Shift}");
          entry.focus();
          await userEvent.keyboard("{ArrowRight}");
        } else {
          await press(entry, way);
        }
        expect(document.activeElement?.textContent).toBe("Choice 3");
        if (way === "keyboard" || way === "remote") {
          // An arrow moves among its choices, the ring showing where it lands (Firefox shows none on
          // a focus a script gave).
          await userEvent.keyboard("{ArrowDown}");
          expect(document.activeElement?.textContent).toBe("Choice 4");
          expect(ringShows(document.activeElement)).toBe(true);
        }
        const list = drawn(".list-box");
        list.scrollTop = list.scrollHeight;
        await expect
          .poll(() => drawn(".list-box__submenu-head").getBoundingClientRect().top)
          .toBeCloseTo(list.getBoundingClientRect().top, 0);
        const back = drawn(".list-box__submenu-back");
        if (way === "remote") {
          await userEvent.keyboard("{ArrowLeft}");
        } else {
          await press(back, way);
        }
        expect(document.activeElement?.textContent).toBe("Order: Choice 3›");
        expect(isFocusOnSomething()).toBe(true);
      });
    }
  }

  it("refuses a submenu inside a submenu: a menu has one level", () => {
    expect(() =>
      renderToStaticMarkup(
        <PickerScreen name="Order" onBack={() => undefined}>
          <ListSubmenu name="Deeper" value="One">
            <ListOption onChoose={() => undefined}>One</ListOption>
          </ListSubmenu>
        </PickerScreen>,
      ),
    ).toThrow(/one level/u);
  });
});

/** Three notices: information first as given, a warning, and one that waits. */
function PageWithBanners(): JSX.Element {
  const [gone, setGone] = useState<readonly string[]>([]);
  const putAway = (key: string): BannerNotice["putAway"] => ({
    kind: "dismiss",
    onDismiss: () => {
      setGone([...gone, key]);
    },
  });
  const notices: BannerNotice[] = [
    {
      key: "plain",
      tone: "information",
      text: "Plain words",
      link: { label: "Read more", href: "#after" },
      putAway: putAway("plain"),
    },
    { key: "lost", tone: "warning", text: "Lost the connection", putAway: { kind: "none" } },
    { key: "updated", tone: "information", text: "Updated", putAway: putAway("updated") },
  ];
  return (
    <main>
      <Banners notices={notices.filter((notice) => !gone.includes(notice.key))} />
      <button type="button" id="after">
        The page
      </button>
    </main>
  );
}

describe("a page's banners (D538)", () => {
  for (const width of widths) {
    for (const way of ways) {
      it(`shows two, warnings first, and the third once one is dismissed by ${way} at ${String(width)}px`, async () => {
        await page.viewport(width, 720);
        mounted = await mount(withServices(<PageWithBanners />));
        const words = (): string[] =>
          [...document.querySelectorAll(".banner")].map((banner) => banner.textContent);
        expect(words()).toEqual(["Lost the connection", "Plain wordsRead more"]);
        expect(drawn(".banner--warning").getAttribute("role")).toBe("note");
        const dismiss = page.getByRole("button", { name: "Dismiss" }).element();
        if (!(dismiss instanceof HTMLElement)) {
          throw new Error("no Dismiss");
        }
        expect(dismiss.getBoundingClientRect().height).toBeGreaterThanOrEqual(44);
        await press(dismiss, way);
        await expect.poll(words).toEqual(["Lost the connection", "Updated"]);
        expect(isFocusOnSomething()).toBe(true);
      });
    }
    it(`puts the link under the words in a narrow banner only, at ${String(width)}px`, async () => {
      await page.viewport(width, 720);
      mounted = await mount(withServices(<PageWithBanners />));
      const link = drawn(".banner--information .banner__link").getBoundingClientRect();
      const text = drawn(".banner--information .banner__words > span").getBoundingClientRect();
      const isUnder = link.top >= text.bottom - 1;
      expect(isUnder).toBe(drawn(".banner--information").clientWidth < 36 * 16);
    });
  }
});

/** A problem whose Try again is counted. */
function ProblemPage(): JSX.Element {
  const [tries, setTries] = useState(0);
  return (
    <main>
      <StatusNote
        told={{
          kind: "problem",
          text: `The libraries could not be read (${String(tries)})`,
          onTryAgain: () => {
            setTries(tries + 1);
          },
        }}
      />
      <StatusNote told={{ kind: "loading", text: "Loading libraries…" }} />
      <StatusNote told={{ kind: "success", text: "Your password is changed." }} />
    </main>
  );
}

describe("a status note (D538)", () => {
  for (const width of widths) {
    for (const way of ways) {
      it(`keeps focus on Try again as it is pressed by ${way} at ${String(width)}px`, async () => {
        await page.viewport(width, 720);
        mounted = await mount(withServices(<ProblemPage />));
        expect(drawn(".status-line--problem").getAttribute("role")).toBe("alert");
        expect(drawn(".status-line--loading").tagName).toBe("OUTPUT");
        expect(drawn(".status-line--success").tagName).toBe("OUTPUT");
        const again = page.getByRole("button", { name: "Try again" }).element();
        if (!(again instanceof HTMLElement)) {
          throw new Error("no Try again");
        }
        if (way === "keyboard") {
          // Reached by Tab, its ring showing.
          await userEvent.keyboard("{Tab}");
          expect(document.activeElement).toBe(again);
          expect(ringShows(again)).toBe(true);
        }
        await press(again, way);
        await expect.poll(() => drawn(".status-line__words").textContent).toContain("(1)");
        expect(document.activeElement).toBe(again);
        const words = drawn(".status-line__words").getBoundingClientRect();
        const isUnder = again.getBoundingClientRect().top >= words.bottom - 1;
        expect(isUnder).toBe(width < 36 * 16);
      });
    }
  }
});

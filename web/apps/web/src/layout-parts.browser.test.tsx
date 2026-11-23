// SPDX-License-Identifier: MPL-2.0

/**
 * The layout parts as a real browser lays them out and as each input reaches them (standards/ui.md,
 * the parts and the checklist; D538 §1): Page, Section, ActionRow, Pill and OfferCard at a phone's
 * 360px, a computer's 1280px and a TV's 1920px; pressed by a mouse, a fingertip, a keyboard's Tab,
 * Enter and Space, and a remote's arrows and OK; the focus ring drawn on whatever has focus, and
 * focus never left on nothing.
 */

// Every page's ground, as the client's entry lays it (client-main.tsx): its tokens and base rules.
import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { act, useState, type JSX, type ReactNode } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { page, userEvent } from "vitest/browser";

import { ActionRow } from "./action-row.tsx";
import { startInput, type Input } from "./input-commands.ts";
import { OfferCard } from "./offer-card.tsx";
import { Page } from "./page.tsx";
import { ActionPill, Pill } from "./pill.tsx";
import { Section } from "./section.tsx";
import { mount, type Mounted } from "./test-dom.tsx";

/** The widths the checklist walks: a phone's, a computer's and a TV's page. */
const widths = [360, 1280, 1920] as const;

/** Under this, in pixels at the browser's 16px rem, a part takes its narrow form (36rem). */
const narrowPx = 576;

/** A page's widest, its gutter included (tokens.css's --size-page, 72rem). */
const pagePx = 1152;

/** How far layout's rounding may put a box. */
const roundingPx = 1;

let mounted: Mounted | undefined;
let input: Input | undefined;

afterEach(() => {
  input?.stop();
  input = undefined;
  mounted?.unmount();
  mounted = undefined;
});

/** A button that names itself in the presses it is given. */
function Pressable(props: {
  readonly name: string;
  readonly onPress: (name: string) => void;
}): JSX.Element {
  return (
    <button
      type="button"
      onClick={() => {
        props.onPress(props.name);
      }}
    >
      {props.name}
    </button>
  );
}

/** A page with every layout part, each press written down in order. */
function Everything(props: {
  readonly presses: string[];
  readonly needsYou?: ReactNode;
}): JSX.Element {
  const press = (name: string): void => {
    props.presses.push(name);
  };
  return (
    <Page
      heading="Libraries"
      job="Where the server finds your videos."
      actions={<Pressable name="Add" onPress={press} />}
      needsYou={props.needsYou}
    >
      <Section heading="Films" actions={<Pressable name="Rescan" onPress={press} />}>
        <p>A section.</p>
      </Section>
      <ActionRow
        secondary={<Pressable name="Cancel" onPress={press} />}
        main={<Pressable name="Save" onPress={press} />}
      />
      <ActionPill
        onPress={() => {
          press("Sound on");
        }}
      >
        Sound on
      </ActionPill>
      <OfferCard
        words="Up next"
        detail="Plays in 10 s"
        onTake={() => {
          press("Up next");
        }}
      />
    </Page>
  );
}

/** The names of every control Tab meets, in the order it meets them. */
const tabOrder = ["Add", "Rescan", "Cancel", "Save", "Sound on", "Up next"];

/** The element the selector finds, which must be drawn. */
function found(selector: string): HTMLElement {
  const element = document.querySelector<HTMLElement>(selector);
  if (element === null) {
    throw new Error(`nothing matches ${selector}`);
  }
  return element;
}

/** A button's short name: its label, an offer card's offer, or its words. */
function nameOf(element: Element): string {
  return (
    element.getAttribute("aria-label") ??
    element.querySelector(".offer-card__offer")?.textContent ??
    element.textContent
  );
}

/** The button whose name is name. */
function button(name: string): HTMLElement {
  const match = [...document.querySelectorAll<HTMLElement>("button")].find(
    (each) => nameOf(each) === name,
  );
  if (match === undefined) {
    throw new Error(`no button ${name}`);
  }
  return match;
}

/** The name of what has focus, which must be a button: focus never lands on nothing. */
function focusedName(): string {
  const focused = document.activeElement;
  if (!(focused instanceof HTMLButtonElement)) {
    throw new Error(`focus is on ${String(focused?.tagName)}, not a control`);
  }
  return nameOf(focused);
}

/** Whether what has focus draws the focus ring (tokens.css's :focus-visible): its outline and halo. */
function showsTheRing(): boolean {
  const focused = document.activeElement;
  if (!(focused instanceof HTMLElement) || !focused.matches(":focus-visible")) {
    return false;
  }
  const style = getComputedStyle(focused);
  return style.outlineStyle === "solid" && style.boxShadow !== "none";
}

/** A fingertip's tap on element: its pointer down and up, as a touch, then the click they make. */
function tap(element: HTMLElement): void {
  const box = element.getBoundingClientRect();
  const init = {
    bubbles: true,
    pointerType: "touch",
    pointerId: 7,
    clientX: box.left + box.width / 2,
    clientY: box.top + box.height / 2,
  };
  act(() => {
    element.dispatchEvent(new PointerEvent("pointerdown", init));
    element.dispatchEvent(new PointerEvent("pointerup", init));
    element.dispatchEvent(new MouseEvent("click", { bubbles: true, detail: 1 }));
  });
}

describe.each(widths)("the layout parts at %ipx (D538 §1)", (width) => {
  const isNarrow = width < narrowPx;

  it("draws a page no wider than 72rem, in the window's middle", async () => {
    await page.viewport(width, 800);
    mounted = await mount(<Everything presses={[]} />);
    const drawn = found(".page").getBoundingClientRect();
    expect(drawn.width).toBeLessThanOrEqual(Math.min(width, pagePx) + roundingPx);
    expect(Math.abs(drawn.left - (width - drawn.right))).toBeLessThanOrEqual(roundingPx);
    if (width > pagePx) {
      expect(drawn.width).toBeLessThan(width - 2 * roundingPx);
    }
  });

  it(
    isNarrow
      ? "puts a page's and a section's actions on a line of their own under the heading"
      : "puts a page's and a section's actions on the heading's line, at its end",
    async () => {
      await page.viewport(width, 800);
      mounted = await mount(<Everything presses={[]} />);
      for (const [heading, actions] of [
        [".page__heading", ".page__actions"],
        [".section__heading", ".section__actions"],
      ] as const) {
        const words = found(heading).getBoundingClientRect();
        const buttons = found(actions).getBoundingClientRect();
        if (isNarrow) {
          expect(buttons.top).toBeGreaterThanOrEqual(words.bottom - roundingPx);
        } else {
          expect(buttons.top).toBeLessThan(words.bottom);
          expect(buttons.left).toBeGreaterThan(words.left);
        }
      }
      // The job's line stays under the heading at every width.
      expect(found(".page__job").getBoundingClientRect().top).toBeGreaterThanOrEqual(
        found(".page__heading").getBoundingClientRect().bottom - roundingPx,
      );
    },
  );

  it("puts Needs you first under the heading where something needs the person, and nowhere otherwise", async () => {
    await page.viewport(width, 800);
    mounted = await mount(
      <Everything presses={[]} needsYou={<p>One library cannot be read.</p>} />,
    );
    const sections = [...document.querySelectorAll(".page > .section")];
    expect(sections[0]?.querySelector("h2")?.textContent).toBe("Needs you");
    expect(sections[0]?.getAttribute("aria-labelledby")).toBe(sections[0]?.querySelector("h2")?.id);
    expect(sections[1]?.querySelector("h2")?.textContent).toBe("Films");
    mounted.unmount();
    mounted = await mount(<Everything presses={[]} />);
    expect(document.body.textContent).not.toContain("Needs you");
  });

  it(
    isNarrow
      ? "stacks an action row's buttons full width, the way back above the main action"
      : "lays an action row's buttons at its end, the way back before the main action",
    async () => {
      await page.viewport(width, 800);
      mounted = await mount(<Everything presses={[]} />);
      const row = found(".action-row").getBoundingClientRect();
      const cancel = button("Cancel").getBoundingClientRect();
      const save = button("Save").getBoundingClientRect();
      if (isNarrow) {
        expect(save.top).toBeGreaterThanOrEqual(cancel.bottom);
        expect(cancel.width).toBeCloseTo(row.width, 0);
        expect(save.width).toBeCloseTo(row.width, 0);
      } else {
        expect(Math.abs(save.top - cancel.top)).toBeLessThanOrEqual(roundingPx);
        expect(save.left).toBeGreaterThan(cancel.right);
        expect(Math.abs(save.right - row.right)).toBeLessThanOrEqual(roundingPx);
      }
    },
  );

  it("keeps an offer card within 26rem and within the film less 3rem, its take a fingertip tall", async () => {
    await page.viewport(width, 800);
    mounted = await mount(<Everything presses={[]} />);
    const card = found(".offer-card").getBoundingClientRect();
    const room = found(".offer-card").parentElement?.getBoundingClientRect().width ?? 0;
    expect(card.width).toBeLessThanOrEqual(Math.min(416, room - 48) + roundingPx);
    expect(button("Up next").getBoundingClientRect().height).toBeGreaterThanOrEqual(44);
    expect(button("Sound on").getBoundingClientRect().height).toBeGreaterThanOrEqual(44);
  });

  it("presses each control by a mouse's click", async () => {
    await page.viewport(width, 800);
    const presses: string[] = [];
    mounted = await mount(<Everything presses={presses} />);
    for (const name of tabOrder) {
      await userEvent.click(button(name));
    }
    expect(presses).toEqual(tabOrder);
  });

  it("presses each control by a fingertip's tap", async () => {
    await page.viewport(width, 800);
    const presses: string[] = [];
    mounted = await mount(<Everything presses={presses} />);
    for (const name of tabOrder) {
      tap(button(name));
    }
    expect(presses).toEqual(tabOrder);
  });

  it("meets each control by Tab in reading order, its ring drawn; Enter and Space press, Escape leaves focus where it is", async () => {
    await page.viewport(width, 800);
    const presses: string[] = [];
    mounted = await mount(<Everything presses={presses} />);
    const met: string[] = [];
    for (const name of tabOrder) {
      await userEvent.tab();
      expect(showsTheRing()).toBe(true);
      met.push(focusedName());
      await userEvent.keyboard(name.length % 2 === 0 ? "{Enter}" : " ");
      await userEvent.keyboard("{Escape}");
      expect(focusedName()).toBe(name);
    }
    expect(met).toEqual(tabOrder);
    expect(presses).toEqual(tabOrder);
  });

  it("meets every control by a remote's arrows, its ring drawn and focus never on nothing; OK presses", async () => {
    await page.viewport(width, 800);
    const presses: string[] = [];
    mounted = await mount(<Everything presses={presses} />);
    input = startInput({ goBack: () => false, isNavigating: () => true });
    // Each arrow and where it goes: down the column a phone's page is; on a wider one, down the
    // actions at the end of each heading's line, then along the action row, the way back before the
    // main action; and onto an offer card, which has no ✕ (the owner, 2026-11-23).
    const walk: readonly (readonly [string, string])[] = isNarrow
      ? [
          ["{ArrowDown}", "Rescan"],
          ["{ArrowDown}", "Cancel"],
          ["{ArrowDown}", "Save"],
          ["{ArrowDown}", "Sound on"],
          ["{ArrowDown}", "Up next"],
          ["{ArrowUp}", "Sound on"],
        ]
      : [
          ["{ArrowDown}", "Rescan"],
          ["{ArrowDown}", "Cancel"],
          ["{ArrowRight}", "Save"],
          ["{ArrowLeft}", "Cancel"],
          ["{ArrowUp}", "Rescan"],
        ];
    button("Add").focus();
    for (const [key, name] of walk) {
      await userEvent.keyboard(key);
      expect(focusedName()).toBe(name);
      expect(showsTheRing()).toBe(true);
    }
    if (!isNarrow) {
      button("Sound on").focus();
      for (const [key, name] of [
        ["{ArrowDown}", "Up next"],
        ["{ArrowUp}", "Sound on"],
      ] as const) {
        await userEvent.keyboard(key);
        expect(focusedName()).toBe(name);
        expect(showsTheRing()).toBe(true);
      }
    }
    await userEvent.keyboard("{Enter}");
    expect(presses).toEqual(["Sound on"]);
  });
});

describe("a pill's voice (D538 §1)", () => {
  it("reads a notice out once as it shows, and a seek's never; neither takes a press nor focus", async () => {
    await page.viewport(1280, 800);
    mounted = await mount(
      <main>
        <button type="button">Before</button>
        <Pill>Seeked to 21:41</Pill>
        <Pill voice="volume">Volume 70%</Pill>
        <Pill voice="silent">+10</Pill>
        <button type="button">After</button>
      </main>,
    );
    const pills = [...document.querySelectorAll<HTMLElement>(".pill")];
    expect(pills.map((pill) => pill.tagName)).toEqual(["OUTPUT", "OUTPUT", "P"]);
    expect(getComputedStyle(pills[1] ?? document.body).fontVariantNumeric).toBe("tabular-nums");
    for (const pill of pills) {
      expect(getComputedStyle(pill).pointerEvents).toBe("none");
    }
    button("Before").focus();
    await userEvent.tab();
    expect(focusedName()).toBe("After");
  });

  it("wraps a long notice's words within a phone's width", async () => {
    await page.viewport(360, 800);
    mounted = await mount(
      <div style={{ inlineSize: "100%" }}>
        <Pill>
          A note over the film long enough that it cannot stand on one line of a phone held upright
        </Pill>
      </div>,
    );
    const pill = found(".pill").getBoundingClientRect();
    expect(pill.right).toBeLessThanOrEqual(360 + roundingPx);
    expect(pill.height).toBeGreaterThan(32);
  });
});

describe("an offer card's words (D538 §1)", () => {
  /** A card whose countdown moves on each press of its button. */
  function Counting(): JSX.Element {
    const [left, setLeft] = useState(10);
    return (
      <OfferCard
        words="Still watching?"
        detail={`Stops in ${String(left)} s`}
        buttons={
          <button
            type="button"
            onClick={() => {
              setLeft(left - 1);
            }}
          >
            Continue
          </button>
        }
      />
    );
  }

  it("reads its offer out, not its countdown, and is no button where its own buttons take it", async () => {
    await page.viewport(1280, 800);
    mounted = await mount(<Counting />);
    expect(found(".offer-card__offer").tagName).toBe("OUTPUT");
    expect(found(".offer-card__detail").closest('output, [role="status"], [aria-live]')).toBeNull();
    expect(document.querySelector(".offer-card__take")).toBeNull();
    expect(document.querySelectorAll(".offer-card button")).toHaveLength(1);
    await userEvent.click(button("Continue"));
    expect(found(".offer-card__detail").textContent).toBe("Stops in 9 s");
  });
});

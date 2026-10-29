// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { createContainerMemory } from "./focus-memory.ts";
import { moveFocus } from "./focus-navigation.ts";
import { onCommand, startInput, type CommandDetail, type Input } from "./input-commands.ts";

/** A page of a banner with a link, and a main content of three buttons, and a field outside it. */
function pageOfButtons(): { readonly main: HTMLElement; readonly field: HTMLInputElement } {
  const banner = document.createElement("header");
  const home = document.createElement("a");
  home.href = "/";
  home.textContent = "home";
  banner.append(home);
  const main = document.createElement("main");
  for (const name of ["first", "second", "third"]) {
    const button = document.createElement("button");
    button.textContent = name;
    main.append(button);
  }
  const field = document.createElement("input");
  document.body.append(banner, main, field);
  return { main, field };
}

/** Presses key on whatever has focus, as a remote's press reaches the page. */
function press(key: string, keyCode = 0): KeyboardEvent {
  const event = new KeyboardEvent("keydown", { key, keyCode, bubbles: true, cancelable: true });
  (document.activeElement ?? document.body).dispatchEvent(event);
  return event;
}

/** What a test changes and reads of the input layer's ways: the Backs it went, and navigation. */
interface Ways {
  readonly backs: number[];
  isNavigating: boolean;
}

/** The input layer running, with the Backs it went counted and its navigation switchable. */
function started(): { readonly input: Input; readonly ways: Ways } {
  const ways: Ways = { backs: [], isNavigating: true };
  const input = startInput({
    goBack: () => {
      ways.backs.push(ways.backs.length + 1);
      return true;
    },
    isNavigating: () => ways.isNavigating,
  });
  return { input, ways };
}

describe("the input layer", () => {
  let running: ReturnType<typeof started> | undefined;
  beforeEach(() => {
    running = started();
  });
  afterEach(() => {
    running?.input.stop();
    running = undefined;
    document.body.replaceChildren();
  });

  it("focuses the main content's first control on the first arrow, not the banner's", () => {
    pageOfButtons();
    const event = press("ArrowDown");
    expect(document.activeElement?.textContent).toBe("first");
    expect(event.defaultPrevented).toBe(true);
  });

  it("leaves an arrow inside a text field's value to the field", () => {
    const { field } = pageOfButtons();
    field.value = "abc";
    field.focus();
    field.setSelectionRange(1, 1);
    const event = press("ArrowLeft");
    expect(document.activeElement).toBe(field);
    expect(event.defaultPrevented).toBe(false);
  });

  it("never leaves Down to a range with nothing below it, which would step its value back (the Fire TV's scrub bar, 2026-10-29)", () => {
    const scrub = document.createElement("input");
    scrub.type = "range";
    document.body.append(scrub);
    scrub.focus();
    const event = press("ArrowDown");
    expect(event.defaultPrevented).toBe(true);
    expect(document.activeElement).toBe(scrub);
  });

  it("leaves an arrow to a text field while it composes, even at the value's end", () => {
    const { main } = pageOfButtons();
    const field = document.createElement("input");
    main.append(field);
    field.focus();
    const composing = new KeyboardEvent("keydown", {
      key: "ArrowLeft",
      isComposing: true,
      bubbles: true,
      cancelable: true,
    });
    field.dispatchEvent(composing);
    expect(composing.defaultPrevented).toBe(false);
    expect(press("ArrowLeft").defaultPrevented).toBe(true);
  });

  it("hears no arrow once stopped", () => {
    pageOfButtons();
    running?.input.stop();
    press("ArrowDown");
    expect(document.activeElement).toBe(document.body);
  });

  it("leaves the arrows to the browser while the app's navigation is off", () => {
    pageOfButtons();
    if (running !== undefined) {
      running.ways.isNavigating = false;
    }
    const event = press("ArrowDown");
    expect(event.defaultPrevented).toBe(false);
    expect(document.activeElement).toBe(document.body);
  });

  it("offers a command to what has focus first, and does nothing more with one it answered", () => {
    const { main } = pageOfButtons();
    const heard: CommandDetail[] = [];
    const stop = onCommand(main, (detail, event) => {
      heard.push(detail);
      event.preventDefault();
    });
    try {
      main.querySelector("button")?.focus();
      const event = press("Escape", 27);
      expect(heard).toEqual([{ command: "back", source: "key" }]);
      expect(event.defaultPrevented).toBe(true);
      expect(running?.ways.backs).toEqual([]);
    } finally {
      stop();
    }
  });

  it("goes back in the app's history on a platform's Back, Escape and Backspace outside a field", () => {
    const { field } = pageOfButtons();
    press("Unidentified", 461);
    press("XF86Back", 10_009);
    press("Escape", 27);
    expect(press("Backspace", 8).defaultPrevented).toBe(true);
    expect(running?.ways.backs).toEqual([1, 2, 3, 4]);
    field.focus();
    expect(press("Backspace", 8).defaultPrevented).toBe(false);
    expect(running?.ways.backs).toHaveLength(4);
  });

  it("leaves a shortcut with a modifier to the browser, as Alt+Left's back", () => {
    pageOfButtons();
    const event = new KeyboardEvent("keydown", {
      key: "ArrowLeft",
      altKey: true,
      bubbles: true,
      cancelable: true,
    });
    document.body.dispatchEvent(event);
    expect(event.defaultPrevented).toBe(false);
    expect(document.activeElement).toBe(document.body);
  });

  it("ticks a checkbox by Enter, which a browser's own Enter does not, and leaves a button's Enter to the browser", () => {
    const { main } = pageOfButtons();
    const box = document.createElement("input");
    box.type = "checkbox";
    main.append(box);
    box.focus();
    expect(press("Enter", 13).defaultPrevented).toBe(true);
    expect(box.checked).toBe(true);
    main.querySelector("button")?.focus();
    expect(press("Enter", 13).defaultPrevented).toBe(false);
  });

  it("presses what has focus by a controller's OK, a button as a click", () => {
    const { main } = pageOfButtons();
    const button = main.querySelector("button");
    let clicks = 0;
    button?.addEventListener("click", () => {
      clicks += 1;
    });
    button?.focus();
    expect(running?.input.dispatch("select", "pad")).toBe(true);
    expect(clicks).toBe(1);
  });

  it("moves focus for a controller so that the browser shows it, as it shows a key's", () => {
    // A gamepad's press is no key, so a browser draws no :focus-visible after it unless asked
    // (the owner, Firefox on a Mac, 2026-10-25).
    const { main } = pageOfButtons();
    const first = main.querySelector("button");
    if (first === null) {
      throw new Error("the page has no button");
    }
    const asked: unknown[] = [];
    first.focus = (options?: FocusOptions): void => {
      asked.push(options);
    };
    moveFocus("down", { memory: createContainerMemory(() => true) }, true);
    expect(asked).toEqual([expect.objectContaining({ focusVisible: true })]);
  });

  it("opens a list's choices by OK, and chooses the next by a controller's OK, which cannot work them", () => {
    const { main } = pageOfButtons();
    const list = document.createElement("select");
    for (const name of ["aired", "dvd"]) {
      const option = document.createElement("option");
      option.value = name;
      option.textContent = name;
      list.append(option);
    }
    main.append(list);
    const opened: string[] = [];
    list.showPicker = (): void => {
      opened.push("picker");
    };
    list.focus();
    expect(press("Enter", 13).defaultPrevented).toBe(true);
    expect(opened).toEqual(["picker"]);
    const changes: string[] = [];
    list.addEventListener("change", () => {
      changes.push(list.value);
    });
    running?.input.dispatch("select", "pad");
    running?.input.dispatch("select", "pad");
    expect(changes).toEqual(["dvd", "aired"]);
    expect(opened).toEqual(["picker"]);
  });
});

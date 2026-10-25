// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";
import { act } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { driveFocus, geometricFocus } from "./lab-focus.ts";
import { useFirstPress } from "./lab-keys.ts";
import { mount, type Mounted } from "./test-dom.tsx";

/** A region of three buttons in the document, and a field outside it. */
function regionOfButtons(): { readonly region: HTMLElement; readonly field: HTMLInputElement } {
  const region = document.createElement("div");
  for (const name of ["first", "second", "third"]) {
    const button = document.createElement("button");
    button.textContent = name;
    region.append(button);
  }
  const field = document.createElement("input");
  document.body.append(region, field);
  return { region, field };
}

/** Presses key on whatever has focus, as a remote's press reaches the page. */
function press(key: string): KeyboardEvent {
  const event = new KeyboardEvent("keydown", { key, bubbles: true, cancelable: true });
  (document.activeElement ?? document.body).dispatchEvent(event);
  return event;
}

describe("the input probe's focus driving", () => {
  afterEach(() => {
    document.body.replaceChildren();
  });

  it("focuses the region's first element on an arrow while nothing in it has focus", () => {
    const { region } = regionOfButtons();
    const stop = driveFocus(region, geometricFocus);
    try {
      const event = press("ArrowDown");
      expect(document.activeElement?.textContent).toBe("first");
      expect(event.defaultPrevented).toBe(true);
    } finally {
      stop();
    }
  });

  it("leaves an arrow in a field outside the region to the field", () => {
    const { region, field } = regionOfButtons();
    field.focus();
    const stop = driveFocus(region, geometricFocus);
    try {
      const event = press("ArrowLeft");
      expect(document.activeElement).toBe(field);
      expect(event.defaultPrevented).toBe(false);
    } finally {
      stop();
    }
  });

  it("hears no arrow once stopped", () => {
    const { region } = regionOfButtons();
    driveFocus(region, geometricFocus)();
    press("ArrowDown");
    expect(document.activeElement).toBe(document.body);
  });
});

describe("the input probe's first press", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("is told of the page's first key, and of no key after", async () => {
    let heard = 0;
    function Listener(): JSX.Element {
      useFirstPress(() => {
        heard += 1;
      });
      return <p>listening</p>;
    }
    mounted = await mount(<Listener />);
    act(() => {
      press("MediaPlayPause");
      press("ArrowDown");
    });
    expect(heard).toBe(1);
  });
});

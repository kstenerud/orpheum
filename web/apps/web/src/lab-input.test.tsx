// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { LabInput } from "./lab-input.tsx";
import {
  bodyField,
  buttonWithText,
  click,
  mount,
  servedBy,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A key pressed on the page, as a remote's reaches it. */
function press(key: string, keyCode = 0, target: EventTarget = document.body): KeyboardEvent {
  const event = new KeyboardEvent("keydown", { key, keyCode, bubbles: true, cancelable: true });
  act(() => {
    target.dispatchEvent(event);
  });
  return event;
}

/** The chips lit, by the button each names. */
function litChips(container: Element): (string | undefined)[] {
  return [...container.querySelectorAll<HTMLElement>(".lab__chip--heard")].map(
    (chip) => chip.dataset["button"],
  );
}

describe("the button test", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("moves into its tiles by our navigation on the first arrow, and starts its tone with it", async () => {
    mounted = await mount(withServices(<LabInput />));
    expect(mounted.container.textContent).not.toContain("A quiet tone plays");
    press("ArrowDown");
    expect(document.activeElement?.classList.contains("lab__tile")).toBe(true);
    expect(mounted.container.textContent).toContain("A quiet tone plays");
  });

  it("moves among the page's every control by our navigation, not the tiles' alone", async () => {
    mounted = await mount(withServices(<LabInput />));
    const send = buttonWithText(mounted.container, "Send to server");
    if (send === undefined) {
      throw new Error("the test has no Send button");
    }
    send.focus();
    const arrow = press("ArrowUp", 38, send);
    expect(arrow.defaultPrevented).toBe(true);
    expect(document.activeElement?.classList.contains("lab__tile")).toBe(false);
  });

  it("lights each button's chip as it is heard, a step back as Back, and names the last large", async () => {
    mounted = await mount(withServices(<LabInput />));
    expect(litChips(mounted.container)).toEqual([]);
    press("Unidentified", 179);
    expect(litChips(mounted.container)).toEqual(["playPause"]);
    press("Enter", 13);
    act(() => {
      globalThis.dispatchEvent(new PopStateEvent("popstate"));
    });
    // In the chips' own order, not the order heard.
    expect(litChips(mounted.container)).toEqual(["select", "back", "playPause"]);
    const last = mounted.container.querySelector(".lab__chip--last");
    expect(last?.getAttribute("data-button")).toBe("back");
    expect(mounted.container.querySelector(".lab__heard-name")?.textContent).toBe("Back");
  });

  it("sends the buttons heard with everything the probe sent before", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "POST /lab-results": [200, { id: "K3J9QW2A" }] }, heard, "token");
    mounted = await mount(withServices(<LabInput />, services));
    press("ArrowRight", 39);
    press("MediaPlayPause", 179);
    await click(buttonWithText(mounted.container, "Send to server"));
    const sent = heard.find((request) => request.methodAndPath === "POST /lab-results");
    expect(sent === undefined ? undefined : bodyField(sent, "probe")).toBe("input");
    const sentResult = sent === undefined ? undefined : bodyField(sent, "result");
    const result = typeof sentResult === "object" && sentResult !== null ? sentResult : {};
    expect(Reflect.get(result, "buttonsHeard")).toEqual(["right", "playPause"]);
    for (const field of [
      "mode",
      "latency",
      "latenciesMs",
      "backSteps",
      "backKeysInLog",
      "mediaSessionFired",
      "hasMediaSession",
      "gamepadsSeen",
      "keys",
      "userAgent",
    ]) {
      expect(Reflect.has(result, field)).toBe(true);
    }
  });
});

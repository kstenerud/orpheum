// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { LabInput } from "./lab-input.tsx";
import {
  bodyField,
  buttonWithText,
  click,
  mount,
  refusal,
  servedBy,
  settleUntil,
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

  it("moves into its tiles by our navigation on the first arrow, and plays no tone until its switch is on", async () => {
    // The owner, 2026-10-25: "Do we need the tone to keep playing?" A TV's remote sends play/pause
    // as a key; only a keyboard's media keys need the tone, so it waits for its switch.
    const play = vi.spyOn(HTMLMediaElement.prototype, "play").mockResolvedValue();
    try {
      mounted = await mount(withServices(<LabInput />));
      press("ArrowDown");
      press("MediaPlayPause", 179);
      expect(document.activeElement?.classList.contains("lab__tile")).toBe(true);
      expect(play).not.toHaveBeenCalled();
      const toneSwitch = buttonWithText(
        mounted.container,
        "Test a keyboard's media keys: plays a quiet tone so the browser passes them to this page",
      );
      if (toneSwitch === undefined) {
        throw new Error("the test has no tone switch");
      }
      expect(toneSwitch.closest("details")).not.toBeNull();
      expect(toneSwitch.getAttribute("aria-pressed")).toBe("false");
      await click(toneSwitch);
      expect(play).toHaveBeenCalledTimes(1);
      expect(toneSwitch.getAttribute("aria-pressed")).toBe("true");
    } finally {
      play.mockRestore();
    }
  });

  it("moves among the page's every control by our navigation, not the tiles' alone", async () => {
    mounted = await mount(withServices(<LabInput />));
    const details = mounted.container.querySelector<HTMLElement>(".lab__details > summary");
    if (details === null) {
      throw new Error("the test has no details");
    }
    details.focus();
    const arrow = press("ArrowUp", 38, details);
    expect(arrow.defaultPrevented).toBe(true);
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

  it("sends each press to the server as it comes, with no Send, and says how many under the run's name", async () => {
    const heard: Heard[] = [];
    const services = servedBy({ "POST /lab/input-events": [204, null] }, heard, "token");
    mounted = await mount(withServices(<LabInput />, services));
    const shown = mounted.container;
    expect(buttonWithText(shown, "Send to server")).toBeUndefined();
    const run = /run (?<run>[A-Z2-7]{8})/u.exec(shown.textContent)?.groups?.["run"];
    expect(shown.textContent).toContain(`Sending as you press · run ${run ?? ""} · 0 sent`);
    // The first goes at once; the two that come while it is on its way go together after it.
    press("Enter", 13);
    press("MediaPlayPause", 179);
    act(() => {
      globalThis.dispatchEvent(new PopStateEvent("popstate"));
    });
    await settleUntil(() => shown.textContent.includes("· 3 sent"));
    expect(shown.textContent).toContain(`Sending as you press · run ${run ?? ""} · 3 sent`);
    const requests = heard.filter((request) => request.methodAndPath === "POST /lab/input-events");
    expect(requests.map((request) => bodyField(request, "run"))).toEqual([run, run]);
    const events = requests.map((request) => bodyField(request, "events"));
    const firstAt: unknown = Reflect.get(Object(Reflect.get(Object(events[0]), 0)), "atMs");
    expect(typeof firstAt).toBe("number");
    expect(events).toEqual([
      [
        {
          seq: 0,
          kind: "key",
          button: "select",
          mode: "geometric",
          key: "Enter",
          code: "",
          keyCode: 13,
          isRepeat: false,
          atMs: firstAt,
        },
      ],
      [
        expect.objectContaining({ seq: 1, kind: "key", button: "playPause", keyCode: 179 }),
        expect.objectContaining({ seq: 2, kind: "back", button: "back", mode: "geometric" }),
      ],
    ]);
    expect(bodyField(requests[0] ?? { methodAndPath: "", body: undefined }, "start")).toEqual(
      expect.objectContaining({ mode: "geometric", viewportWidth: window.innerWidth }),
    );
    expect(
      bodyField(requests[1] ?? { methodAndPath: "", body: undefined }, "start"),
    ).toBeUndefined();
    expect(heard.some((request) => request.methodAndPath === "POST /lab/results")).toBe(false);
  });

  it("sends a key's modifiers, place and composing with it, and a context menu asked for", async () => {
    // D407 §10: the volume keys' rule is read off each layout's + and −, and Silk's Menu key off
    // what the page hears of it.
    const heard: Heard[] = [];
    const services = servedBy({ "POST /lab/input-events": [204, null] }, heard, "token");
    mounted = await mount(withServices(<LabInput />, services));
    const shown = mounted.container;
    const plus = new KeyboardEvent("keydown", {
      key: "+",
      code: "Equal",
      keyCode: 187,
      shiftKey: true,
      location: 0,
      bubbles: true,
    });
    const numpadMinus = new KeyboardEvent("keydown", {
      key: "-",
      code: "NumpadSubtract",
      keyCode: 109,
      ctrlKey: true,
      location: 3,
      isComposing: true,
      bubbles: true,
    });
    act(() => {
      document.body.dispatchEvent(plus);
      document.body.dispatchEvent(numpadMinus);
      document.body.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true }));
    });
    await settleUntil(() => shown.textContent.includes("· 3 sent"));
    const events = heard
      .filter((request) => request.methodAndPath === "POST /lab/input-events")
      .flatMap((request) => bodyField(request, "events") as unknown[]);
    expect(events).toEqual([
      expect.objectContaining({ seq: 0, kind: "key", key: "+", shiftKey: true }),
      expect.objectContaining({
        seq: 1,
        kind: "key",
        key: "-",
        ctrlKey: true,
        location: 3,
        isComposing: true,
      }),
      expect.objectContaining({ seq: 2, kind: "contextmenu" }),
    ]);
    expect(events[0]).not.toHaveProperty("location");
    expect(events[1]).not.toHaveProperty("shiftKey");
  });

  it("says when sending fails, and tries again at the next press", async () => {
    const heard: Heard[] = [];
    const services = servedBy(
      { "POST /lab/input-events": [refusal(404, "notFound"), [204, null]] },
      heard,
      "token",
    );
    mounted = await mount(withServices(<LabInput />, services));
    const shown = mounted.container;
    press("Enter", 13);
    await settleUntil(() => shown.textContent.includes("Could not send"));
    expect(shown.textContent).toMatch(
      /Could not send; trying again at the next press · run [A-Z2-7]{8} · 0 sent/u,
    );
    press("MediaPlayPause", 179);
    await settleUntil(() => shown.textContent.includes("· 2 sent"));
    const retried = heard.filter(
      (request) => request.methodAndPath === "POST /lab/input-events",
    )[1];
    const seqs = (retried === undefined ? [] : (bodyField(retried, "events") as unknown[])).map(
      (event): unknown => Reflect.get(Object(event), "seq"),
    );
    expect(seqs).toEqual([0, 1]);
  });
});

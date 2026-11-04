// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ControlledMode, doublePressWithinMs } from "./cast-receiver.tsx";
import { mount, type Mounted } from "./test-dom.tsx";
import { testServices, withServices } from "./test-services.tsx";

/** Presses key on what has focus and lets it go, as a remote's press reaches the page. */
function press(key: string, keyCode = 0): KeyboardEvent {
  const event = new KeyboardEvent("keydown", { key, keyCode, bubbles: true, cancelable: true });
  act(() => {
    const target = document.activeElement ?? document.body;
    target.dispatchEvent(event);
    target.dispatchEvent(new KeyboardEvent("keyup", { key, keyCode, bubbles: true }));
  });
  return event;
}

// A held OK is no second press however the browser sends its repeats (D338 §6; VQ122).
describe("the receiver's controlled mode at OK held", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.useRealTimers();
  });

  /** A receiver's controlled mode, and what it was told of leaving. */
  async function controlled(): Promise<{ readonly onLeave: ReturnType<typeof vi.fn> }> {
    const onLeave = vi.fn();
    mounted = await mount(
      withServices(
        <ControlledMode fromName="Pixel" onTakeBack={onLeave} />,
        testServices("token", "/play/3"),
      ),
    );
    return { onLeave };
  }

  it("stays at OK held, its key's repeats no second press, though two presses as quick leave", async () => {
    vi.useFakeTimers({ toFake: ["performance"] });
    const { onLeave } = await controlled();
    const held = (isRepeat: boolean): void => {
      const event = new KeyboardEvent("keydown", {
        key: "Enter",
        bubbles: true,
        cancelable: true,
        repeat: isRepeat,
      });
      act(() => {
        document.body.dispatchEvent(event);
      });
    };
    held(false);
    vi.advanceTimersByTime(450); // Android's first repeat, inside the window
    held(true);
    vi.advanceTimersByTime(50);
    held(true);
    expect(onLeave).not.toHaveBeenCalled();
    act(() => {
      document.body.dispatchEvent(new KeyboardEvent("keyup", { key: "Enter", bubbles: true })); // let go
    });
    vi.advanceTimersByTime(doublePressWithinMs + 1);
    press("Enter");
    vi.advanceTimersByTime(doublePressWithinMs - 1);
    press("Enter");
    expect(onLeave).toHaveBeenCalledOnce();
  });

  it("stays at OK held where the browser sends its repeats as presses, unmarked and with no key up (Silk, VQ122)", async () => {
    vi.useFakeTimers({ toFake: ["performance"] });
    const { onLeave } = await controlled();
    const down = (): void => {
      act(() => {
        document.body.dispatchEvent(
          new KeyboardEvent("keydown", { key: "Enter", bubbles: true, cancelable: true }),
        );
      });
    };
    down();
    vi.advanceTimersByTime(433); // as the Fire TV sent them on 2026-10-29
    down();
    for (let each = 0; each < 4; each += 1) {
      vi.advanceTimersByTime(35);
      down();
    }
    expect(onLeave).not.toHaveBeenCalled();
  });

  it("tells its session which press took control back, and what the keys did between (VQ122)", async () => {
    vi.useFakeTimers({ toFake: ["performance"] });
    const note = vi.fn();
    const onLeave = vi.fn();
    mounted = await mount(
      withServices(
        <ControlledMode fromName="Pixel" onTakeBack={onLeave} connection={{ note }} />,
        testServices("token", "/play/3"),
      ),
    );
    press("Enter");
    vi.advanceTimersByTime(200);
    press("Enter");
    expect(onLeave).toHaveBeenCalledOnce();
    expect(note).toHaveBeenCalledWith({
      kind: "takenBack",
      by: "select",
      how: "gapMs=200 upsBetween=1",
    });
  });
});

// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { useState, type JSX } from "react";
import { act } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { asPressedGivesUpAfterMs, useAsPressed } from "./as-pressed.ts";
import { mount, type Mounted } from "./test-dom.tsx";

describe("a value pressed on the remote is shown and stepped as pressed until the receiver says it", () => {
  let mounted: Mounted | undefined;
  let say: (value: number) => void = () => undefined;
  const sent: number[] = [];

  /** A stepper: shows its value, each press of + sending the one after it. */
  function Stepper(props: { readonly initial: number }): JSX.Element {
    const [said, setSaid] = useState(props.initial);
    say = (value) => {
      act(() => {
        setSaid(value);
      });
    };
    const [shown, press] = useAsPressed(said, (value) => {
      sent.push(value);
    });
    return (
      <button
        type="button"
        onClick={() => {
          press(shown + 50);
        }}
      >
        {shown}
      </button>
    );
  }

  beforeEach(() => {
    sent.length = 0;
  });
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.useRealTimers();
  });

  /** The stepper drawn, its timers then the test's to step (mounting waits on real ones). */
  async function drawn(initial: number): Promise<void> {
    mounted = await mount(<Stepper initial={initial} />);
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  }

  function pressed(times: number): void {
    for (let index = 0; index < times; index++) {
      act(() => {
        mounted!.container.querySelector("button")!.click();
      });
    }
  }
  const shown = (): string | null => mounted!.container.querySelector("button")!.textContent;

  it("sends two different values for two quick presses, each stepped from the one before", async () => {
    await drawn(100);
    pressed(2);
    expect(sent).toEqual([150, 200]);
    expect(shown()).toBe("200");
  });

  it("does not snap back to the receiver's word for an earlier press, and adopts the last value's", async () => {
    await drawn(100);
    pressed(3);
    say(150);
    expect(shown()).toBe("250");
    say(200);
    expect(shown()).toBe("250");
    say(250);
    expect(shown()).toBe("250");
    say(0); // nothing is outstanding now: the receiver's word is adopted
    expect(shown()).toBe("0");
  });

  it("adopts the receiver's word once it has not said the last value after the give-up", async () => {
    await drawn(100);
    pressed(2);
    say(150);
    expect(shown()).toBe("200");
    act(() => {
      vi.advanceTimersByTime(asPressedGivesUpAfterMs - 1);
    });
    expect(shown()).toBe("200");
    act(() => {
      vi.advanceTimersByTime(1);
    });
    expect(shown()).toBe("150");
  });
});

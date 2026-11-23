// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, useState, type JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { CornerNotes, cornerNoteShownMs, useCornerNotes } from "./corner-notes.tsx";
import { dangerousActionAttribute } from "./focus-navigation.ts";
import { ActionPill } from "./pill.tsx";
import { ReopeningNote } from "./reopening-note.tsx";
import { SoundOnPill } from "./sound-on-pill.tsx";
import { click, mount, type Mounted } from "./test-dom.tsx";

/** The corner's notes, told by a button that tells "Note n" at its nth press. */
function Telling(): JSX.Element {
  const { notes, tell, forget } = useCornerNotes(["Note 0"]);
  const [told, setTold] = useState(0);
  return (
    <>
      <button
        type="button"
        onClick={() => {
          setTold(told + 1);
          tell([`Note ${String(told + 1)}`]);
        }}
      >
        Tell
      </button>
      <CornerNotes notes={notes} onGone={forget} />
    </>
  );
}

describe("the notes at the film's bottom-left (D538 §1)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.useRealTimers();
  });

  /** The notes shown, top to bottom. */
  function shown(): readonly (string | null)[] {
    return [...(mounted?.container.querySelectorAll(".corner-notes .pill") ?? [])].map(
      (each) => each.textContent,
    );
  }

  it("stacks at most three, the newest lowest, each read out as it comes and gone by its own timer", async () => {
    vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"], shouldAdvanceTime: true });
    mounted = await mount(<Telling />);
    const tell = mounted.container.querySelector("button");
    expect(shown()).toEqual(["Note 0"]);
    expect(mounted.container.querySelector(".corner-notes output.pill")).not.toBeNull();
    act(() => {
      vi.advanceTimersByTime(cornerNoteShownMs / 2);
    });
    await click(tell);
    await click(tell);
    await click(tell);
    expect(shown()).toEqual(["Note 1", "Note 2", "Note 3"]);
    act(() => {
      vi.advanceTimersByTime(cornerNoteShownMs / 2);
    });
    await click(tell);
    expect(shown()).toEqual(["Note 2", "Note 3", "Note 4"]);
    act(() => {
      vi.advanceTimersByTime(cornerNoteShownMs / 2);
    });
    expect(shown()).toEqual(["Note 4"]);
    act(() => {
      vi.advanceTimersByTime(cornerNoteShownMs / 2);
    });
    expect(shown()).toEqual([]);
  });
});

describe("the pills that take a press over the film (D538 §1)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("takes no press while disabled, and is marked a dangerous action where it is one", async () => {
    let pressed = 0;
    const press = (): void => {
      pressed += 1;
    };
    mounted = await mount(
      <>
        <ActionPill isDisabled isDangerous onPress={press}>
          Stop casting here
        </ActionPill>
        <ActionPill onPress={press}>Turn the sound on</ActionPill>
      </>,
    );
    const [stop, sound] = [...mounted.container.querySelectorAll("button")];
    expect(stop?.disabled).toBe(true);
    expect(stop?.hasAttribute(dangerousActionAttribute)).toBe(true);
    await click(stop);
    expect(pressed).toBe(0);
    expect(sound?.disabled).toBe(false);
    expect(sound?.hasAttribute(dangerousActionAttribute)).toBe(false);
    await click(sound);
    expect(pressed).toBe(1);
  });

  it("asks for the sound in this device's words, and for the voices instead where both apply", async () => {
    let pressed = 0;
    mounted = await mount(
      <>
        <SoundOnPill />
        <SoundOnPill
          onPress={() => {
            pressed += 1;
          }}
        />
        <SoundOnPill isForVoices onPress={() => undefined} />
      </>,
    );
    const [notice, action, voices] = [...mounted.container.querySelectorAll(".sound-on-pill")];
    // A receiver's, which any press answers, is a notice; a pointer's words here.
    expect(notice?.querySelector("output.pill")?.textContent).toBe("Click to turn the sound on");
    expect(notice?.querySelector("button")).toBeNull();
    await click(action?.querySelector("button.pill--action"));
    expect(pressed).toBe(1);
    expect(voices?.textContent).toBe("Tap to hear everyone's voices");
  });
});

describe("the note while a lost playback opens again (D538 §1)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("is a Pill on the film's stage, so it shows while the film fills the window", async () => {
    mounted = await mount(
      <>
        <ReopeningNote />
        <div className="film-player__stage" />
      </>,
    );
    const stage = mounted.container.querySelector(".film-player__stage");
    expect(stage?.querySelector(".play__reopening output.pill")?.textContent).toBe(
      "The server restarted. Opening the film again where you were…",
    );
  });
});

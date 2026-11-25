// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi, type Mock } from "vitest";

import {
  heldRepeatsAfterMs,
  heldRepeatsEveryMs,
  remoteSoundMovesOf,
  RemotePad,
} from "./cast-remote-pad.tsx";
import type { PlayerSound, PlayerSource } from "./playback-target.ts";
import type { SequenceMoves } from "./player-controls.tsx";
import { buttonNamed, click, mount, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

/** A film of 10 min on the receiver, standing at 1:00, paused or playing; its seeks and presses heard. */
function sourceAt(isPaused = true): PlayerSource & {
  readonly seekTo: Mock<(atMs: number) => void>;
  readonly playOrPause: Mock<() => void>;
} {
  return {
    atMs: 60_000,
    durationMs: 600_000,
    isPaused,
    isReady: true,
    playOrPause: vi.fn<() => void>(),
    seekTo: vi.fn<(atMs: number) => void>(),
  };
}

/** The film's level on the receiver, volume, muted or not; what is set of it heard. */
function soundAt(
  volume: number,
  isMuted = false,
): PlayerSound & {
  readonly setVolume: Mock<(volume: number) => void>;
  readonly setMuted: Mock<(isMuted: boolean) => void>;
} {
  return {
    volume,
    isMuted,
    canSetVolume: true,
    setVolume: vi.fn<(volume: number) => void>(),
    setMuted: vi.fn<(isMuted: boolean) => void>(),
  };
}

describe("the cast remote's pad (D548 §5, §6, §10)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.useRealTimers();
  });

  async function pad(props: {
    readonly source?: PlayerSource;
    readonly moves?: SequenceMoves;
    readonly sound?: PlayerSound | undefined;
    readonly isDisabled?: boolean;
    readonly onShown?: () => void;
  }): Promise<HTMLElement> {
    mounted = await mount(
      withServices(
        <RemotePad
          source={props.source ?? sourceAt()}
          moves={props.moves ?? {}}
          sound={remoteSoundMovesOf(props.sound, props.onShown ?? (() => undefined))}
          isDisabled={props.isDisabled ?? false}
        />,
      ),
    );
    return mounted.container;
  }

  /** The pad's cells in order, each the name of its button, or "" where it is empty. */
  function cellsOf(page: HTMLElement): string[] {
    return [...page.querySelectorAll(".cast-remote__pad-cell")].map(
      (cell) => cell.querySelector("button")?.getAttribute("aria-label") ?? "",
    );
  }

  it("lays out Previous, Next, the jumps, Play and the volume in three rows of three, leaving the cells of what the receiver cannot do empty", async () => {
    const page = await pad({
      moves: { previous: () => undefined, next: () => undefined },
      sound: soundAt(0.7),
    });
    expect(cellsOf(page)).toEqual([
      "Previous episode",
      "",
      "Next episode",
      "Back 10 s",
      "Play",
      "Forward 10 s",
      "Volume down",
      "Mute",
      "Volume up",
    ]);
    mounted?.unmount();
    // No episode before, none next, no level reported: their cells empty, the volume's row gone.
    const bare = await pad({});
    expect(cellsOf(bare)).toEqual(["", "", "", "Back 10 s", "Play", "Forward 10 s"]);
    expect(bare.querySelectorAll(".cast-remote__pad-cell:empty")).toHaveLength(3);
  });

  it("draws each button round at its size, the jumps 72 px, Play 96 px, the rest 64 px", async () => {
    const page = await pad({ moves: { next: () => undefined }, sound: soundAt(0.7) });
    const sizeOf = (name: string): string | undefined =>
      /icon-button--remote-(\d+)/u.exec(buttonNamed(page, name)?.className ?? "")?.[1];
    expect(sizeOf("Next episode")).toBe("64");
    expect(sizeOf("Back 10 s")).toBe("72");
    expect(sizeOf("Play")).toBe("96");
    expect(sizeOf("Forward 10 s")).toBe("72");
    expect(sizeOf("Volume down")).toBe("64");
    expect(sizeOf("Mute")).toBe("64");
    expect(sizeOf("Volume up")).toBe("64");
  });

  it("plays or pauses, jumps 10 s within the film, and moves to the episode before or after", async () => {
    const source = sourceAt(false);
    const previous = vi.fn();
    const next = vi.fn();
    const page = await pad({ source, moves: { previous, next } });
    await click(buttonNamed(page, "Pause"));
    expect(source.playOrPause).toHaveBeenCalledTimes(1);
    await click(buttonNamed(page, "Forward 10 s"));
    expect(source.seekTo).toHaveBeenLastCalledWith(70_000);
    await click(buttonNamed(page, "Back 10 s"));
    expect(source.seekTo).toHaveBeenLastCalledWith(50_000);
    await click(buttonNamed(page, "Previous episode"));
    await click(buttonNamed(page, "Next episode"));
    expect(previous).toHaveBeenCalledTimes(1);
    expect(next).toHaveBeenCalledTimes(1);
  });

  it("steps the film's level by a tenth within 0 and 1, turns a muted film's sound on at Volume up, and shows the level at each press", async () => {
    const onShown = vi.fn();
    const sound = soundAt(0.65);
    const page = await pad({ sound, onShown });
    await click(buttonNamed(page, "Volume up"));
    expect(sound.setVolume).toHaveBeenLastCalledWith(0.75);
    await click(buttonNamed(page, "Volume down"));
    expect(sound.setVolume).toHaveBeenLastCalledWith(0.55);
    expect(sound.setMuted).not.toHaveBeenCalled();
    expect(onShown).toHaveBeenCalledTimes(2);
    mounted?.unmount();
    const full = soundAt(0.95, true);
    const muted = await pad({ sound: full, onShown });
    expect(buttonNamed(muted, "Mute")?.getAttribute("aria-pressed")).toBe("true");
    await click(buttonNamed(muted, "Volume up"));
    expect(full.setMuted).toHaveBeenCalledWith(false);
    expect(full.setVolume).toHaveBeenLastCalledWith(1);
    await click(buttonNamed(muted, "Mute"));
    expect(full.setMuted).toHaveBeenLastCalledWith(false);
    expect(onShown).toHaveBeenCalledTimes(4);
  });

  it("repeats Volume down held: once more after 500 ms, then every 300 ms until the finger lifts, its click no press more", async () => {
    const sound = soundAt(0.5);
    const page = await pad({ sound });
    vi.useFakeTimers();
    const down = buttonNamed(page, "Volume down")!;
    act(() => {
      down.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true, pointerId: 1 }));
    });
    act(() => {
      vi.advanceTimersByTime(heldRepeatsAfterMs - 1);
    });
    expect(sound.setVolume).not.toHaveBeenCalled();
    act(() => {
      vi.advanceTimersByTime(1 + 2 * heldRepeatsEveryMs);
    });
    expect(sound.setVolume).toHaveBeenCalledTimes(3);
    act(() => {
      window.dispatchEvent(new PointerEvent("pointerup", { bubbles: true, pointerId: 1 }));
      down.dispatchEvent(new MouseEvent("click", { bubbles: true, detail: 1 }));
      vi.advanceTimersByTime(5 * heldRepeatsEveryMs);
    });
    expect(sound.setVolume).toHaveBeenCalledTimes(3);
    // A short tap presses once, at its click.
    act(() => {
      down.dispatchEvent(new PointerEvent("pointerdown", { bubbles: true, pointerId: 2 }));
      window.dispatchEvent(new PointerEvent("pointerup", { bubbles: true, pointerId: 2 }));
      down.dispatchEvent(new MouseEvent("click", { bubbles: true, detail: 1 }));
      vi.advanceTimersByTime(heldRepeatsAfterMs * 2);
    });
    expect(sound.setVolume).toHaveBeenCalledTimes(4);
  });

  it("does nothing at a press or a key while the receiver is not reached, each button aria-disabled but kept in the focus order", async () => {
    const source = sourceAt();
    const sound = soundAt(0.5);
    const page = await pad({ source, sound, isDisabled: true });
    const buttons = [...page.querySelectorAll("button")];
    expect(buttons.length).toBeGreaterThan(0);
    for (const button of buttons) {
      expect(button.getAttribute("aria-disabled")).toBe("true");
      expect(button.disabled).toBe(false);
    }
    await click(buttonNamed(page, "Play"));
    await click(buttonNamed(page, "Volume up"));
    act(() => {
      document.dispatchEvent(new KeyboardEvent("keydown", { key: " ", bubbles: true }));
      document.dispatchEvent(new KeyboardEvent("keydown", { key: "m", bubbles: true }));
    });
    expect(source.playOrPause).not.toHaveBeenCalled();
    expect(sound.setVolume).not.toHaveBeenCalled();
    expect(sound.setMuted).not.toHaveBeenCalled();
  });

  /** Presses key on what has focus, or on the page where nothing has, as a keyboard does. */
  function press(key: string, extra: KeyboardEventInit = {}): void {
    const target = document.activeElement ?? document.body;
    act(() => {
      target.dispatchEvent(new KeyboardEvent("keydown", { key, bubbles: true, ...extra }));
    });
  }

  it("plays at Space, seeks at Left and Right while nothing has focus, steps the volume at + = - and the numpad's, and mutes at M alone", async () => {
    const source = sourceAt();
    const sound = soundAt(0.5);
    const page = await pad({ source, sound });
    press(" ");
    expect(source.playOrPause).toHaveBeenCalledTimes(1);
    press("ArrowRight");
    expect(source.seekTo).toHaveBeenLastCalledWith(70_000);
    press("ArrowLeft");
    expect(source.seekTo).toHaveBeenLastCalledWith(50_000);
    press("+");
    press("=");
    press("+", { code: "NumpadAdd" });
    press("-");
    press("-", { code: "NumpadSubtract" });
    expect(sound.setVolume.mock.calls.map(([volume]) => volume)).toEqual([0.6, 0.6, 0.6, 0.4, 0.4]);
    press("m");
    expect(sound.setMuted).toHaveBeenCalledTimes(1);
    press("m", { ctrlKey: true });
    press("M", { metaKey: true });
    press("m", { altKey: true });
    expect(sound.setMuted).toHaveBeenCalledTimes(1);
    // A button has focus: Space is its press, the arrows its focus's, not seeks.
    buttonNamed(page, "Mute")?.focus();
    press(" ");
    press("ArrowRight");
    expect(source.playOrPause).toHaveBeenCalledTimes(1);
    expect(source.seekTo).toHaveBeenCalledTimes(2);
  });

  it("leaves every key to a text field with focus", async () => {
    const source = sourceAt();
    const sound = soundAt(0.5);
    await pad({ source, sound });
    const field = document.createElement("input");
    document.body.append(field);
    field.focus();
    try {
      // Right last: at the field's end it moves focus out of it, as the d-pad does.
      for (const key of [" ", "+", "-", "m", "ArrowRight"]) {
        press(key);
      }
      expect(source.playOrPause).not.toHaveBeenCalled();
      expect(source.seekTo).not.toHaveBeenCalled();
      expect(sound.setVolume).not.toHaveBeenCalled();
      expect(sound.setMuted).not.toHaveBeenCalled();
    } finally {
      field.remove();
    }
  });
});

// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  controlsShown,
  hideAfter,
  overlayAnswer,
  shownWith,
  type ControlsContext,
  type OverlayState,
  type PressContext,
} from "./player-overlay-state.ts";

const hidden: OverlayState = { shown: "hidden", shownBy: "remote" };
const scrub: OverlayState = { shown: "scrub", shownBy: "remote" };
const paused: OverlayState = { shown: "paused", shownBy: "remote" };
const full: OverlayState = { shown: "full", shownBy: "remote" };

/** A television's remote, the film playing, focus on the video. */
const onTv: PressContext = {
  isPaused: false,
  isTelevision: true,
  isOnScrub: false,
  isOnButtons: false,
  hasOffer: false,
  isOnOffer: false,
};

const onComputer: PressContext = { ...onTv, isTelevision: false };

describe("the overlay hidden (D407 §4)", () => {
  it("seeks by Left and Right and shows the scrub bar alone, taking the press", () => {
    expect(overlayAnswer(hidden, "left", onTv)).toEqual({
      state: scrub,
      isTaken: true,
      seekByMs: -10_000,
    });
    expect(overlayAnswer(hidden, "right", onTv)).toEqual({
      state: scrub,
      isTaken: true,
      seekByMs: 10_000,
    });
  });

  it("pauses a playing film by OK and shows the pause sign", () => {
    expect(overlayAnswer(hidden, "select", onTv)).toEqual({
      state: paused,
      isTaken: true,
      isPlayPause: true,
    });
  });

  it("plays a paused film by OK and stays hidden", () => {
    expect(overlayAnswer(hidden, "select", { ...onTv, isPaused: true })).toEqual({
      state: hidden,
      isTaken: true,
      isPlayPause: true,
    });
  });

  it("shows all of it by Up or Down, focus to play, shown by a remote on a television alone", () => {
    for (const command of ["up", "down"] as const) {
      expect(overlayAnswer(hidden, command, onTv)).toEqual({
        state: full,
        isTaken: true,
        focus: "play",
      });
      expect(overlayAnswer(hidden, command, onComputer).state).toEqual({
        shown: "full",
        shownBy: "pointer",
      });
    }
  });

  it("leaves Back to the page", () => {
    expect(overlayAnswer(hidden, "back", onTv)).toEqual({ state: hidden, isTaken: false });
  });
});

describe("the scrub bar alone", () => {
  it("seeks on by Left and Right", () => {
    expect(overlayAnswer(scrub, "left", onTv)).toEqual({
      state: scrub,
      isTaken: true,
      seekByMs: -10_000,
    });
  });

  it("hides by Back, taking it", () => {
    expect(overlayAnswer(scrub, "back", onTv)).toEqual({ state: hidden, isTaken: true });
  });

  it("shows all of it by Up, focus to play", () => {
    expect(overlayAnswer(scrub, "up", onTv)).toEqual({ state: full, isTaken: true, focus: "play" });
  });
});

describe("an offer alone, focus on it (D407 §4, Q3)", () => {
  const onOffer: PressContext = { ...onTv, hasOffer: true, isOnOffer: true };

  it("leaves OK and Back to the offer", () => {
    expect(overlayAnswer(hidden, "select", onOffer)).toEqual({ state: hidden, isTaken: false });
    expect(overlayAnswer(hidden, "back", onOffer)).toEqual({ state: hidden, isTaken: false });
  });

  it("shows the convenience area alone while hidden, and only then (D407 §3)", () => {
    expect(shownWith("hidden", true)).toBe("convenience");
    expect(shownWith("hidden", false)).toBe("hidden");
    expect(shownWith("full", true)).toBe("full");
    expect(shownWith("scrub", true)).toBe("scrub");
  });

  it("presses the offer by OK with no guard, focus elsewhere, the scrub bar showing or not (D407 §3)", () => {
    const offerElsewhere: PressContext = { ...onTv, hasOffer: true };
    for (const state of [hidden, scrub]) {
      expect(overlayAnswer(state, "select", offerElsewhere)).toEqual({
        state,
        isTaken: true,
        isOfferPressed: true,
      });
    }
  });

  it("leaves Back to the offer through the scrub bar, which dismisses it", () => {
    expect(overlayAnswer(scrub, "back", onOffer)).toEqual({ state: hidden, isTaken: false });
  });

  it("still seeks by Left and Right, showing the scrub bar, the offer staying", () => {
    expect(overlayAnswer(hidden, "right", onOffer)).toEqual({
      state: scrub,
      isTaken: true,
      seekByMs: 10_000,
    });
  });

  it("shows all of the overlay by Up or Down", () => {
    expect(overlayAnswer(hidden, "down", onOffer)).toEqual({
      state: full,
      isTaken: true,
      focus: "play",
    });
  });
});

describe("the pause sign and the scrub bar, after OK paused", () => {
  const isPaused: PressContext = { ...onTv, isPaused: true };

  it("plays by OK and hides", () => {
    expect(overlayAnswer(paused, "select", isPaused)).toEqual({
      state: hidden,
      isTaken: true,
      isPlayPause: true,
    });
  });

  it("seeks by Left and Right, the sign staying", () => {
    expect(overlayAnswer(paused, "left", isPaused)).toEqual({
      state: paused,
      isTaken: true,
      seekByMs: -10_000,
    });
  });

  it("shows all of it by Up or Down", () => {
    expect(overlayAnswer(paused, "up", isPaused)).toEqual({
      state: full,
      isTaken: true,
      focus: "play",
    });
  });

  it("hides by Back, the film still paused", () => {
    expect(overlayAnswer(paused, "back", isPaused)).toEqual({ state: hidden, isTaken: true });
  });
});

describe("all of the overlay (D407 §5)", () => {
  it("hides by the first Back", () => {
    expect(overlayAnswer(full, "back", onTv)).toEqual({ state: hidden, isTaken: true });
  });

  it("leaves every press to an open menu, Back closing it first (D407 §8)", () => {
    const inMenu = { ...onTv, isOnButtons: true, isMenuOpen: true };
    for (const command of ["back", "left", "up", "down", "select", "playPause"] as const) {
      expect(overlayAnswer(full, command, inMenu)).toEqual({ state: full, isTaken: false });
    }
  });

  it("seeks by Left and Right on the scrub bar", () => {
    expect(overlayAnswer(full, "left", { ...onTv, isOnScrub: true })).toEqual({
      state: full,
      isTaken: true,
      seekByMs: -10_000,
    });
  });

  // Plan item 17 (2026-11-02): Right meant for Full screen sought the film once the bar had hidden.
  it("walks a row by Left and Right anywhere else, never seeking", () => {
    for (const command of ["left", "right"] as const) {
      expect(overlayAnswer(full, command, { ...onTv, isOnButtons: true })).toEqual({
        state: full,
        isTaken: false,
      });
    }
  });

  it("leaves Up, Down and OK to focus and the focused control on a television", () => {
    for (const command of ["up", "down", "select"] as const) {
      expect(overlayAnswer(full, command, { ...onTv, isOnButtons: true })).toEqual({
        state: full,
        isTaken: false,
      });
    }
  });

  it("puts focus on the video by Down from the buttons on a computer (Q14)", () => {
    expect(overlayAnswer(full, "down", { ...onComputer, isOnButtons: true })).toEqual({
      state: full,
      isTaken: true,
      focus: "video",
    });
    expect(overlayAnswer(full, "down", onComputer)).toEqual({ state: full, isTaken: false });
  });

  // The owner, 2026-11-03, in Firefox: full screen by a double-click or ⤢, the mouse having shown
  // the overlay, Down left focus where no row could be reached by Up or Down.
  it("brings focus outside it, on the video or the page, to play by Up or Down", () => {
    const byPointer: OverlayState = { shown: "full", shownBy: "pointer" };
    for (const command of ["up", "down"] as const) {
      for (const context of [onComputer, onTv]) {
        expect(overlayAnswer(byPointer, command, { ...context, isFocusOutside: true })).toEqual({
          state: byPointer,
          isTaken: true,
          focus: "play",
        });
      }
    }
  });
});

describe("play, pause and the seeking keys, the player's in every state", () => {
  it("pauses with the pause sign, or plays hidden, while not all of it shows", () => {
    expect(overlayAnswer(hidden, "playPause", onTv)).toEqual({ state: paused, isTaken: false });
    expect(overlayAnswer(paused, "playPause", { ...onTv, isPaused: true })).toEqual({
      state: hidden,
      isTaken: false,
    });
  });

  it("shows the scrub bar for a seek while not all of it shows, and leaves all of it as it is", () => {
    expect(overlayAnswer(hidden, "seekForward", onTv)).toEqual({ state: scrub, isTaken: false });
    expect(overlayAnswer(full, "seekBack", onTv)).toEqual({ state: full, isTaken: false });
    expect(overlayAnswer(full, "playPause", onTv)).toEqual({ state: full, isTaken: false });
  });

  // D407 §10: + and − are the player's; the overlay shows the volume they leave, nothing else moving.
  it("leaves + and − to the player in every state, a menu open too, and shows the volume", () => {
    for (const state of [hidden, scrub, paused, full]) {
      for (const command of ["volumeUp", "volumeDown"] as const) {
        expect(overlayAnswer(state, command, onComputer)).toEqual({
          state,
          isTaken: false,
          isVolumeShown: true,
        });
      }
    }
    expect(overlayAnswer(full, "volumeUp", { ...onComputer, isMenuOpen: true })).toEqual({
      state: full,
      isTaken: false,
      isVolumeShown: true,
    });
    expect(overlayAnswer(hidden, "volumeDown", { ...onComputer, hasOffer: true })).toEqual({
      state: hidden,
      isTaken: false,
      isVolumeShown: true,
    });
  });
});

describe("when the overlay hides (D407 §6)", () => {
  const playing = { isPaused: false, isMenuOpen: false, isFocusVisibleIn: false };

  it("hides all of it shown by a remote 3 s after the last press while playing, focus in it or not", () => {
    expect(hideAfter(full, playing)).toBe(3000);
    expect(hideAfter(full, { ...playing, isFocusVisibleIn: true })).toBe(3000);
  });

  it("never hides all of it while the film is paused", () => {
    expect(hideAfter(full, { ...playing, isPaused: true })).toBeUndefined();
  });

  it("holds all of it shown by a pointer while focus a key put there is in it", () => {
    const byPointer: OverlayState = { shown: "full", shownBy: "pointer" };
    expect(hideAfter(byPointer, { ...playing, isFocusVisibleIn: true })).toBeUndefined();
    expect(hideAfter(byPointer, playing)).toBe(3000);
  });

  it("holds all of it while a menu is open", () => {
    expect(hideAfter(full, { ...playing, isMenuOpen: true })).toBeUndefined();
  });

  it("hides the scrub bar after 1.5 s, the pause sign after 3 s, and nothing hidden", () => {
    expect(hideAfter(scrub, playing)).toBe(1500);
    expect(hideAfter(paused, { ...playing, isPaused: true })).toBe(3000);
    expect(hideAfter(hidden, playing)).toBeUndefined();
  });
});

describe("which controls show (D407 §2)", () => {
  const computer: ControlsContext = {
    isTelevision: false,
    isTouch: false,
    isVolumeSettable: true,
    audioTrackCount: 1,
    hasPrevious: false,
    hasNext: false,
    isReceiver: false,
  };

  it("shows a computer's every control but previous, next and a lone audio track", () => {
    expect(controlsShown(computer)).toEqual([
      "up",
      "seekBack",
      "play",
      "seekForward",
      "sound",
      "subtitles",
      "more",
      "fullScreen",
    ]);
  });

  it("shows no sound on a television, a touch screen or where the page cannot set the volume", () => {
    for (const context of [
      { ...computer, isTelevision: true },
      { ...computer, isTouch: true },
      { ...computer, isVolumeSettable: false },
    ]) {
      expect(controlsShown(context)).not.toContain("sound");
      expect(controlsShown(context)).toContain("fullScreen");
    }
  });

  it("shows previous and next within a sequence, the audio tracks where there are more than one", () => {
    const shown = controlsShown({
      ...computer,
      hasPrevious: true,
      hasNext: true,
      audioTrackCount: 2,
    });
    expect(shown).toContain("previous");
    expect(shown).toContain("next");
    expect(shown).toContain("audio");
  });

  it("shows no way up on a cast's receiver", () => {
    expect(controlsShown({ ...computer, isReceiver: true })).not.toContain("up");
  });

  it("shows no menus on a cast's receiver (D407 §12)", () => {
    const shown = controlsShown({ ...computer, audioTrackCount: 2, isReceiver: true });
    expect(shown).not.toContain("subtitles");
    expect(shown).not.toContain("audio");
    expect(shown).not.toContain("more");
  });
});

// SPDX-License-Identifier: MPL-2.0

import { act } from "react";

import { commandEventName } from "./input-commands.ts";

/** Sends a command, as the input layer does, at target; returns whether something took it. */
export function press(command: string, target: EventTarget = document.body): boolean {
  const event = new CustomEvent(commandEventName, {
    detail: { command, source: "key" },
    bubbles: true,
    cancelable: true,
  });
  act(() => {
    target.dispatchEvent(event);
  });
  return event.defaultPrevented;
}

/** Shows the overlay, hidden as the player opens (D418 §1), as a mouse's move does (two moves). */
export function shownByAMouse(): void {
  for (const screenX of [5, 50]) {
    const move = { bubbles: true, pointerType: "mouse", screenX, screenY: 5 };
    act(() => {
      document.body.dispatchEvent(new PointerEvent("pointermove", move));
    });
  }
}

/** A video that says it plays, with play and pause that change what it says. */
export function playingVideo(): HTMLVideoElement {
  const video = document.createElement("video");
  let isPaused = false;
  Object.defineProperty(video, "paused", { get: () => isPaused });
  video.pause = () => {
    isPaused = true;
    video.dispatchEvent(new Event("pause"));
  };
  video.play = () => {
    isPaused = false;
    video.dispatchEvent(new Event("play"));
    return Promise.resolve();
  };
  return video;
}

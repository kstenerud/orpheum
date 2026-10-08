// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { playerMovedItselfEvent, type PlayerNote } from "@orpheum/sdk";
import { useRef, type JSX } from "react";
import { describe, expect, it } from "vitest";

import { diagnoseAtMostEveryMs, useDiagnosisNotes } from "./together-diagnosis.ts";
import { fire, mount } from "./test-dom.tsx";

describe("a following player's diagnosis", () => {
  it("is noted when the player moves itself, at most once in a while", async () => {
    const notes: PlayerNote[] = [];
    let nowMs = 0;
    const connection = {
      note: (note: PlayerNote) => {
        notes.push(note);
      },
    };
    function Watched(): JSX.Element {
      const videoRef = useRef<HTMLVideoElement>(null);
      useDiagnosisNotes(videoRef, connection, () => nowMs);
      // eslint-disable-next-line jsx-a11y-x/media-has-caption -- a test's video, never played
      return <video ref={videoRef} />;
    }
    const mounted = await mount(<Watched />);
    const video = mounted.container.querySelector("video");
    await fire(video, new Event(playerMovedItselfEvent));
    nowMs += diagnoseAtMostEveryMs / 2;
    await fire(video, new Event(playerMovedItselfEvent)); // too soon: not said again
    expect(notes).toEqual([
      {
        kind: "diagnosis",
        why: "movedItself",
        positionMs: 0,
        readyState: 0,
        videoBuffered: "",
        audioBuffered: "",
        lastError: "",
      },
    ]);
    nowMs += diagnoseAtMostEveryMs;
    await fire(video, new Event(playerMovedItselfEvent));
    expect(notes).toHaveLength(2);
    mounted.unmount();
  });
});

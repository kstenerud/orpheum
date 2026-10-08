// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { playerBufferedFirstEvent, playerMovedItselfEvent, type PlayerNote } from "@orpheum/sdk";
import { useRef, type JSX } from "react";
import { describe, expect, it } from "vitest";

import {
  diagnoseAtMostEveryMs,
  noteSized,
  noteTextMost,
  useDiagnosisNotes,
} from "./together-diagnosis.ts";
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
        mapping: "",
        mediaSourceState: "",
      },
    ]);
    nowMs += diagnoseAtMostEveryMs;
    await fire(video, new Event(playerMovedItselfEvent));
    expect(notes).toHaveLength(2);
    // Each playlist's first chunk is said however soon after another (the sync review, 2026-10-08).
    await fire(video, new CustomEvent(playerBufferedFirstEvent, { detail: "main" }));
    await fire(video, new CustomEvent(playerBufferedFirstEvent, { detail: "audio" }));
    expect(
      notes.slice(2).map((note) => (note.kind === "diagnosis" ? note.why : note.kind)),
    ).toEqual(["bufferedFirst", "bufferedFirst"]);
    mounted.unmount();
  });

  it("cuts each text to what the server takes, keeping the latest ranges (the M4 closing audit)", () => {
    const ranges = Array.from(
      { length: 60 },
      (_, index) => `${String(index * 10)}.00-${String(index * 10 + 5)}.00`,
    );
    const sized = noteSized({
      videoBuffered: ranges.join(" "),
      audioBuffered: "",
      lastError: "x".repeat(400),
      mapping: "",
      mediaSourceState: "streaming",
    });
    expect(sized.videoBuffered.length).toBeLessThanOrEqual(noteTextMost);
    expect(sized.videoBuffered.endsWith("590.00-595.00")).toBe(true);
    expect(sized.lastError).toHaveLength(noteTextMost);
    expect(sized.mediaSourceState).toBe("streaming");
  });
});

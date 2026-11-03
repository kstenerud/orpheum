// SPDX-License-Identifier: MPL-2.0

// First, before anything that calls what it fills.
import "./old-engine.ts";

import { startReceiver } from "./caf.ts";
import { LabReceiver, memoryEveryMs } from "./lab.ts";

/**
 * The cast receiver's entry point. For now it is the lab's spike receiver (plans/tv-device-tests.md;
 * casting-2026-10-25.md §5.2 step 2): CAF's receiver, started never to close for being idle, with
 * our own player in place of CAF's, listening on the lab's namespace; the receiver of M6 (D245)
 * grows from what it finds.
 */

/** The namespace the lab's sender (the web app's /lab/cast) and this receiver speak on. */
const labNamespace = "urn:x-cast:org.orpheum.lab";

function start(): void {
  const video = document.createElement("video");
  video.className = "receiver__video";
  const screen = document.createElement("pre");
  screen.className = "receiver__log";
  document.body.append(video, screen);
  let receiver: ReturnType<typeof startReceiver>;
  const lab = new LabReceiver(screen, video, (message) => {
    receiver?.broadcast(message);
  });
  try {
    receiver = startReceiver(
      labNamespace,
      (message) => {
        lab.heard(message);
      },
      (what) => {
        lab.note(what);
      },
    );
    lab.note(receiver === undefined ? "not on a Cast device: CAF did not load" : "CAF started");
  } catch (cause) {
    lab.note(`CAF did not start: ${cause instanceof Error ? cause.message : "failed"}`);
  }
  lab.note(navigator.userAgent);
  setInterval(() => {
    lab.sampleMemory();
  }, memoryEveryMs);
  void lab.report();
}

start();

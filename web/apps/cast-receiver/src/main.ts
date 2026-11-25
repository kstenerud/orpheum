// SPDX-License-Identifier: MPL-2.0

// First, before anything that calls what it fills.
import "./old-engine.ts";

import { startReceiver } from "./caf.ts";
import { castNamespace } from "./guest.ts";
import { closesWhenIdle } from "./idle.ts";
import { memoryEveryMs, ReceiverLog } from "./log.ts";
import { RegularCasts } from "./regular-cast.ts";

/**
 * The cast receiver's entry point: CAF's receiver, started never to close for being idle by CAF,
 * which cannot see our own player in place of CAF's, so it closes itself by its video (idle.ts),
 * listening on the cast's namespace for a regular cast under a grant (D465 §2, D555 §3), its log
 * kept from the start and shown once a sender asks (log.ts).
 */
function start(): void {
  const video = document.createElement("video");
  video.className = "receiver__video";
  const screen = document.createElement("pre");
  screen.className = "receiver__log";
  document.body.append(video, screen);
  const log = new ReceiverLog(screen, video);
  const casts = new RegularCasts(video, log);
  let receiver: ReturnType<typeof startReceiver>;
  try {
    receiver = startReceiver(
      {
        [castNamespace]: (message) => {
          casts.heard(
            message,
            (answer) => {
              receiver?.broadcast(castNamespace, answer);
            },
            // A Watch Together guest's part ended (D518 §2): the TV goes back to its own screen.
            () => {
              log.note("the guest's part ended: going home");
              receiver?.stop();
            },
          );
        },
      },
      (what) => {
        log.note(what);
      },
    );
    log.note(receiver === undefined ? "not on a Cast device: CAF did not load" : "CAF started");
  } catch (cause) {
    log.note(`CAF did not start: ${cause instanceof Error ? cause.message : "failed"}`);
  }
  log.note(navigator.userAgent);
  closesWhenIdle(video, () => {
    log.note("idle for too long: closing");
    receiver?.stop();
  });
  setInterval(() => {
    log.noteMemory();
  }, memoryEveryMs);
}

start();

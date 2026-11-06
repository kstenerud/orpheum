// SPDX-License-Identifier: MPL-2.0

import { codecsOf, diagnosisOf, soundEarlierMsOf, soundRouteOf } from "@orpheum/player";
import type { components } from "@orpheum/sdk";

/**
 * How the video's sound reaches the speakers as it starts playing: through Web Audio or not, the
 * latencies its audio context reports, how the player moves it (earlier by its chunks, held back by
 * Web Audio's delay, and the latency counted toward the device's offset), the codecs its buffers took and where its player placed each
 * track's first chunk. The owner's TCL TV played every film's sound half a second late, YouTube's in
 * time, and nothing in the log said why (2026-10-24).
 */
export function soundOf(video: HTMLVideoElement): components["schemas"]["StartupSoundBody"] {
  const route = soundRouteOf(video);
  const codecs = codecsOf(video);
  const mapping = diagnosisOf(video)?.mapping ?? "";
  const earlierMs = soundEarlierMsOf(video);
  return {
    ...(route.isThroughWebAudio ? { isThroughWebAudio: true } : {}),
    ...(route.baseLatencyMs === undefined ? {} : { baseLatencyMs: route.baseLatencyMs }),
    ...(route.outputLatencyMs === undefined ? {} : { outputLatencyMs: route.outputLatencyMs }),
    ...(earlierMs === 0 ? {} : { earlierMs }),
    ...(route.delayMs === undefined ? {} : { delayMs: route.delayMs }),
    ...(route.latencyCountedMs === undefined ? {} : { latencyCountedMs: route.latencyCountedMs }),
    ...(codecs === "" ? {} : { codecs: codecs.slice(0, 200) }),
    ...(mapping === "" ? {} : { mapping: mapping.slice(0, 300) }),
  };
}

// SPDX-License-Identifier: MPL-2.0

import { codecsOf, diagnosisOf, soundRouteOf } from "@orpheum/player";
import type { components } from "@orpheum/sdk";

/**
 * How the video's sound reaches the speakers as it starts playing: through Web Audio or not, the
 * latencies its audio context reports, the codecs its buffers took and where its player placed each
 * track's first chunk. The owner's TCL TV played every film's sound half a second late, YouTube's in
 * time, and nothing in the log said why (2026-10-24).
 */
export function soundOf(video: HTMLVideoElement): components["schemas"]["StartupSoundBody"] {
  const route = soundRouteOf(video);
  const codecs = codecsOf(video);
  const mapping = diagnosisOf(video)?.mapping ?? "";
  return {
    ...(route.isThroughWebAudio ? { isThroughWebAudio: true } : {}),
    ...(route.baseLatencyMs === undefined ? {} : { baseLatencyMs: route.baseLatencyMs }),
    ...(route.outputLatencyMs === undefined ? {} : { outputLatencyMs: route.outputLatencyMs }),
    ...(codecs === "" ? {} : { codecs: codecs.slice(0, 200) }),
    ...(mapping === "" ? {} : { mapping: mapping.slice(0, 300) }),
  };
}

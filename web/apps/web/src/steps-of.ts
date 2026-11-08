// SPDX-License-Identifier: MPL-2.0

import { diagnosisOf, type PlaybackDiagnosis } from "@orpheum/player";

/** What a seek's report carries of the player's diagnosis. */
export type SeekSteps = Pick<PlaybackDiagnosis, "steps" | "linkSamples">;

/**
 * The player's last steps, its quality's choices among them with why, and the link's last samples
 * apart from them (D352 §5), for a seek's report, as read reads them, the player's diagnosis; none
 * of either when it kept none. A player that dropped from the twin to 360p after a jump on a 77 Mb/s
 * link said nothing of why (the owner's Firefox on the production NAS, 2026-10-20).
 */
export function stepsOf(
  video: HTMLVideoElement,
  read: (video: HTMLVideoElement) => SeekSteps | undefined = diagnosisOf,
): SeekSteps {
  const { steps = "", linkSamples = "" } = read(video) ?? {};
  return {
    ...(steps === "" ? {} : { steps }),
    ...(linkSamples === "" ? {} : { linkSamples }),
  };
}

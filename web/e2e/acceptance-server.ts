// SPDX-License-Identifier: MPL-2.0

/**
 * M4's acceptance's setup (D252): the browser tier's server, with one long film for each network
 * profile, so each run plays from its start for as long as the run lasts.
 */

import { startServer, type Film } from "./server.ts";

/** How long each profile's run lasts, in minutes: ORPHEUM_ACCEPTANCE_MINUTES, 30 when unset. */
export function acceptanceMinutes(): number {
  const minutes = Number(process.env["ORPHEUM_ACCEPTANCE_MINUTES"] ?? "30");
  if (!Number.isFinite(minutes) || minutes <= 0) {
    throw new Error("ORPHEUM_ACCEPTANCE_MINUTES must be a number of minutes above 0");
  }
  return minutes;
}

/** The network profiles of D243 §9, as netem's arguments, each applied to every packet. */
export const networkProfiles = [
  { name: "home", netem: ["delay", "20ms"] },
  { name: "mobile", netem: ["delay", "80ms", "30ms", "loss", "1%"] },
  { name: "congested", netem: ["delay", "150ms", "50ms", "loss", "3%", "rate", "5mbit"] },
] as const;

/** The film a profile's run plays. */
export function acceptanceFilm(profile: string): string {
  return `Acceptance Film ${profile}`;
}

/** A small, long film: the run measures timing, not pictures, over a link of 5 Mbit/s at worst. */
function longFilm(title: string, seconds: number): Film {
  return {
    title,
    year: 2026,
    extension: "mp4",
    ffmpegInputArguments: [
      ...["-f", "lavfi", "-i", `testsrc2=size=320x180:rate=24:duration=${String(seconds)}`],
      ...["-f", "lavfi", "-i", `sine=frequency=440:sample_rate=48000:duration=${String(seconds)}`],
      ...["-c:v", "libx264", "-preset", "veryfast", "-pix_fmt", "yuv420p", "-g", "48"],
      ...["-c:a", "aac", "-ac", "2", "-movflags", "+faststart"],
    ],
  };
}

export default async function acceptanceSetup(): Promise<() => Promise<void>> {
  const seconds = (acceptanceMinutes() + 5) * 60;
  return await startServer(
    networkProfiles.map((profile) => longFilm(acceptanceFilm(profile.name), seconds)),
  );
}

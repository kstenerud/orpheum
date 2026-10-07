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

/** One network profile (D263 §1): what it shapes, and what is held under it (§2). */
export interface NetworkProfile {
  readonly name: string;
  /** netem's arguments: delay, jitter kept in order, loss, and a limit at which it never drops. */
  readonly netem: readonly string[];
  /** tbf's arguments: the bottleneck's rate and its tail-drop queue. */
  readonly bottleneck: readonly string[];
  /** Whether a long-lived bulk download shares the link for the whole run, a household's other use. */
  readonly hasCompetingDownload: boolean;
  /** Whether voice is best effort here for a pair with Firefox, which sends no RED. */
  readonly isVoiceBestEffortWithFirefox: boolean;
}

/**
 * The network profiles, as network-profiles.md §0 builds them from RFC 8867 and 8869 and
 * measurement studies (D263 §1): a netem stage for delay, jitter and loss, its rate of 1 Gbit/s
 * keeping jittered packets in order and its limit high enough never to drop, and a tbf bottleneck
 * with its own tail-drop queue. On the loopback every packet, either way, passes once: the delay is
 * one way.
 */
export const networkProfiles: readonly NetworkProfile[] = [
  {
    name: "home",
    netem: ["delay", "20ms", "limit", "100000"],
    bottleneck: ["rate", "100mbit", "burst", "64kb", "latency", "30ms"],
    hasCompetingDownload: false,
    isVoiceBestEffortWithFirefox: false,
  },
  {
    name: "mobile",
    netem: [
      "delay",
      "40ms",
      "10ms",
      "distribution",
      "normal",
      "rate",
      "1gbit",
      "loss",
      "0.2%",
      "limit",
      "100000",
    ],
    bottleneck: ["rate", "20mbit", "burst", "32kb", "latency", "300ms"],
    hasCompetingDownload: false,
    isVoiceBestEffortWithFirefox: false,
  },
  {
    name: "congested",
    netem: [
      "delay",
      "40ms",
      "5ms",
      "distribution",
      "normal",
      "rate",
      "1gbit",
      "loss",
      "0.5%",
      "limit",
      "100000",
    ],
    bottleneck: ["rate", "5mbit", "burst", "16kb", "latency", "200ms"],
    hasCompetingDownload: true,
    isVoiceBestEffortWithFirefox: false,
  },
  {
    name: "lossy",
    netem: [
      "delay",
      "40ms",
      "10ms",
      "distribution",
      "normal",
      "rate",
      "1gbit",
      "loss",
      "3%",
      "limit",
      "100000",
    ],
    bottleneck: ["rate", "20mbit", "burst", "32kb", "latency", "100ms"],
    hasCompetingDownload: false,
    isVoiceBestEffortWithFirefox: true,
  },
];

/**
 * The profiles a run plays under: ORPHEUM_ACCEPTANCE_PROFILES, comma-separated names, every one when
 * unset. The branch gate runs *home* alone, for a few minutes (D252 §2, D261 §2).
 */
export function chosenProfiles(): readonly NetworkProfile[] {
  const asked = process.env["ORPHEUM_ACCEPTANCE_PROFILES"];
  if (asked === undefined || asked === "") {
    return networkProfiles;
  }
  return asked.split(",").map((name) => {
    const profile = networkProfiles.find((each) => each.name === name.trim());
    if (profile === undefined) {
      throw new Error(`ORPHEUM_ACCEPTANCE_PROFILES names ${name}, not one of the network profiles`);
    }
    return profile;
  });
}

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
    chosenProfiles().map((profile) => longFilm(acceptanceFilm(profile.name), seconds)),
  );
}

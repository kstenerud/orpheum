// SPDX-License-Identifier: MPL-2.0

/**
 * The episode of each series a person last reached in this tab, its page or its playing, so the
 * series' page, returned to by Back or Up, shows that episode's season with it focused (D367 §9):
 * after a binge from S1E10 into season 2 the series shows season 2, not the season 1 its entry held.
 * Only returned to: each step forward (Navigation's go) lets every one go, the page reached noting
 * its own again, so a series opened afresh from a list shows from its top, not an episode played
 * from Continue Watching earlier (the owner's Fire TV, 2026-10-29).
 */

import { useEffect } from "react";

import { resetOnEachTestMount } from "./screen-load.tsx";
import { useServices } from "./services.tsx";

/** An episode reached: its season of the series as its page shows it (none: the default), and it. */
export interface EpisodeReached {
  readonly season: number | undefined;
  readonly episodeId: number;
}

/** The episode last reached of each series, by the series' id; let go of once its page shows it. */
const reached = new Map<number, EpisodeReached>();

// A tab's memory, forgotten at each test's mount as a new tab would start (resetOnEachTestMount).
resetOnEachTestMount(() => {
  reached.clear();
});

/** Notes that the episode episodeId of series seriesId, at season, was reached. */
export function noteEpisodeReached(seriesId: number, episode: EpisodeReached): void {
  reached.set(seriesId, episode);
}

/** The episode of seriesId last reached since its page last showed one, if any. */
export function episodeReachedOf(seriesId: number): EpisodeReached | undefined {
  return reached.get(seriesId);
}

/** Lets go of episode, once the series' page has shown it, unless another was reached since. */
export function letGoOfEpisodeReached(seriesId: number, episode: EpisodeReached): void {
  if (reached.get(seriesId) === episode) {
    reached.delete(seriesId);
  }
}

/** Lets go of every episode reached at each step forward while the app shows. */
export function useEpisodesReachedLetGoOnGoing(): void {
  const { navigation } = useServices();
  useEffect(
    () =>
      navigation.beforeLeaving(() => {
        reached.clear();
      }),
    [navigation],
  );
}

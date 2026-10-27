// SPDX-License-Identifier: MPL-2.0

/**
 * The episode of each series a person last reached in this tab, its page or its playing, so the
 * series' page, returned to by Back or Up, shows that episode's season with it focused (D367 §9):
 * after a binge from S1E10 into season 2 the series shows season 2, not the season 1 its entry held.
 */

/** An episode reached: its season of the series as its page shows it (none: the default), and it. */
export interface EpisodeReached {
  readonly season: number | undefined;
  readonly episodeId: number;
}

/** The episode last reached of each series, by the series' id; let go of once its page shows it. */
const reached = new Map<number, EpisodeReached>();

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

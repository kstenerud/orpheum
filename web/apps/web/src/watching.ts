// SPDX-License-Identifier: MPL-2.0

import type { Api, components, PlayableFormats } from "@orpheum/sdk";

import { noteDecisionTimes } from "./play-state.ts";
import { dataOrThrow } from "./problem.ts";

/** The server's decision on how a file plays here (cut-a §6). */
export type PlaybackDecision = components["schemas"]["StartPlaybackOutputBody"];

/** Asks the server how a file plays in this browser; nothing of where it is (D247 §11). */
export async function requestPlayback(
  api: Api,
  fileId: number,
  playableFormats: PlayableFormats,
): Promise<PlaybackDecision> {
  const askedMs = performance.now();
  const decision = dataOrThrow(
    await api.POST("/playback", {
      body: { fileId, playableFormats },
    }),
  );
  noteDecisionTimes(decision.mediaToken, askedMs, performance.now());
  return decision;
}

/**
 * How long a film page's decision may wait for its Play page before a new one is asked for: half
 * the least media_token_idle_expiry_hours can be, an hour, so its token is never idled out by the
 * time it plays.
 */
export const warmUpKeptMs = 30 * 60_000;

/**
 * The film a person is watching in this tab, and the playbacks asked for it, which the server keeps
 * working on until the person shows they want another film (D258 §2): opening another film's page,
 * or playing another. Going from the player back to the film's page, or anywhere else, keeps it
 * (the owner, 2026-10-07: "Until the user signals intent to watch a different video, we should keep
 * data").
 *
 * A film's page warms it up (D258 §1): it asks how the film plays at once, so the server starts its
 * prepared copy or its chunks, and the Play page takes that decision instead of asking again.
 */
export class Watching {
  private fileId: number | undefined;
  // The playbacks asked for the film, by their media tokens, which a move to another lets go of.
  private readonly tokens = new Set<string>();
  // The film page's decision, until the Play page takes it, and when it was asked for.
  private warm:
    { readonly decision: Promise<PlaybackDecision>; readonly askedMs: number } | undefined;

  constructor(
    private readonly api: Api,
    private readonly nowMs: () => number = () => performance.now(),
  ) {}

  /** A film's page was opened: the film is warmed up, and any other one let go of. */
  warmUp(fileId: number, playableFormats: PlayableFormats): void {
    if (this.fileId === fileId && this.warm !== undefined) {
      return;
    }
    this.watch(fileId);
    const decision = this.ask(fileId, playableFormats);
    // A warm-up that fails is the Play page's to ask again; nothing waits on it here.
    decision.catch(() => undefined);
    this.warm = { decision, askedMs: this.nowMs() };
  }

  /**
   * The Play page asks how the film plays: the film page's decision when it is fresh and on the
   * film's ladder, else a new one, since the warm-up read the keyframes of a film not cut yet and it
   * may be on its ladder now (D260 §1). Another film being watched is let go of first.
   */
  decide(fileId: number, playableFormats: PlayableFormats): Promise<PlaybackDecision> {
    this.watch(fileId);
    const warm = this.warm;
    this.warm = undefined;
    if (warm !== undefined && this.nowMs() - warm.askedMs < warmUpKeptMs) {
      // Its start is timed from Play, not from opening the page: what the person waits for.
      const pressedMs = performance.now();
      return warm.decision.then((decision) => {
        if (decision.onLadder === undefined) {
          return this.ask(fileId, playableFormats);
        }
        noteDecisionTimes(decision.mediaToken, pressedMs, performance.now());
        return decision;
      });
    }
    return this.ask(fileId, playableFormats);
  }

  private async ask(fileId: number, playableFormats: PlayableFormats): Promise<PlaybackDecision> {
    const decision = await requestPlayback(this.api, fileId, playableFormats);
    if (this.fileId === fileId) {
      this.tokens.add(decision.mediaToken);
    } else {
      // Another film was chosen while this one was asked for.
      this.leave(decision.mediaToken);
    }
    return decision;
  }

  /** Makes fileId the film watched, letting go of the playbacks of the one before. */
  private watch(fileId: number): void {
    if (this.fileId === fileId) {
      return;
    }
    for (const token of this.tokens) {
      this.leave(token);
    }
    this.tokens.clear();
    this.warm = undefined;
    this.fileId = fileId;
  }

  private leave(token: string): void {
    // A leave that fails costs only background priority, later, when the token idles out.
    this.api.DELETE("/playback/{token}", { params: { path: { token } } }).catch(() => undefined);
  }
}

const watchingByApi = new WeakMap<Api, Watching>();

/** The tab's Watching, one per API client, so each test's is its own. */
export function watchingOf(api: Api): Watching {
  let watching = watchingByApi.get(api);
  if (watching === undefined) {
    watching = new Watching(api);
    watchingByApi.set(api, watching);
  }
  return watching;
}

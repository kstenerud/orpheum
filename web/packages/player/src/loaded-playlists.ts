// SPDX-License-Identifier: MPL-2.0

/**
 * The playlists a playback's start was answered with, handed to hls.js as loaded (D502 §3). hls.js
 * asks for the master playlist, then for the playlists of the quality and the sound it starts on,
 * each only once the one before is in, three round trips before its first chunk is asked: a Fire
 * TV's took 0.5–0.9 s over WebRTC (start-chain-2026-11-16.md). The decision that starts the
 * playback carries them (POST /playback's onLadder.playlists); hls.js's playlist loader
 * (loadedPlaylistLoader) answers each from them, once, and asks the core for any other.
 */

import type {
  HlsConfig,
  Loader,
  LoaderCallbacks,
  LoaderConfiguration,
  LoaderContext,
  LoaderStats,
  PlaylistLoaderConstructor,
  PlaylistLoaderContext,
} from "hls.js";

import { forItsContext } from "./audio-shift.ts";
import type { LoaderClass } from "./link-shaping.ts";

/** A playlist as the path it names answers it. */
export interface LoadedPlaylist {
  readonly url: string;
  readonly text: string;
}

/** The playlists a playback's start was answered with: its master, and its quality's and sound's. */
export interface StartingPlaylists {
  readonly master: LoadedPlaylist;
  readonly picture: LoadedPlaylist;
  readonly sound?: LoadedPlaylist | undefined;
}

/** The address url names, as hls.js asks for it: whole, against the page's own. */
function absoluteOf(url: string): string {
  return new URL(url, globalThis.location.href).href;
}

/** A load's stats, as hls.js's own loaders begin them. */
function freshStats(): LoaderStats {
  return {
    aborted: false,
    loaded: 0,
    retry: 0,
    total: 0,
    chunkCount: 0,
    bwEstimate: 0,
    loading: { start: 0, first: 0, end: 0 },
    parsing: { start: 0, end: 0 },
    buffering: { start: 0, first: 0, end: 0 },
  };
}

/** The stats of a playlist answered from memory: loaded whole the moment it was asked for. */
function answeredAtOnce(text: string): LoaderStats {
  const now = performance.now();
  const stats = freshStats();
  stats.loading = { start: now, first: now, end: now };
  stats.loaded = text.length;
  stats.total = text.length;
  return stats;
}

/**
 * The class of playlist loader that answers each of loaded, at its address, from memory the first
 * time hls.js asks for it, and loads by carrier every other ask, a playlist asked again among them.
 */
export function loadedPlaylistLoader(
  loaded: StartingPlaylists | undefined,
  carrier: LoaderClass,
): PlaylistLoaderConstructor {
  const each = loaded === undefined ? [] : [loaded.master, loaded.picture, loaded.sound];
  const held = new Map(
    each.flatMap((playlist) =>
      playlist === undefined ? [] : [[absoluteOf(playlist.url), playlist.text] as const],
    ),
  );
  return class LoadedPlaylistLoader implements Loader<PlaylistLoaderContext> {
    context: PlaylistLoaderContext | null = null;

    private readonly inner: Loader<LoaderContext>;
    private answered: LoaderStats | undefined;
    private isAborted = false;

    constructor(config: HlsConfig) {
      this.inner = new carrier(config);
    }

    get stats(): LoaderStats {
      return this.answered ?? this.inner.stats;
    }

    destroy(): void {
      this.inner.destroy();
    }

    abort(): void {
      this.isAborted = true;
      this.inner.abort();
    }

    getCacheAge(): number | null {
      return this.answered === undefined ? (this.inner.getCacheAge?.() ?? null) : null;
    }

    getResponseHeader(name: string): string | null {
      return this.answered === undefined ? (this.inner.getResponseHeader?.(name) ?? null) : null;
    }

    load(
      context: PlaylistLoaderContext,
      config: LoaderConfiguration,
      callbacks: LoaderCallbacks<PlaylistLoaderContext>,
    ): void {
      this.context = context;
      const url = absoluteOf(context.url);
      const text = held.get(url);
      if (text === undefined) {
        this.inner.load(
          context,
          config,
          forItsContext(context, callbacks, () => undefined),
        );
        return;
      }
      held.delete(url);
      const stats = answeredAtOnce(text);
      this.answered = stats;
      // Answered after this ask returns, as a load is.
      queueMicrotask(() => {
        if (!this.isAborted) {
          callbacks.onSuccess({ url, data: text }, stats, context, null);
        }
      });
    }
  };
}

// SPDX-License-Identifier: MPL-2.0

/**
 * A chunked playback's hls.js and the loaders of its chunks and playlists (chunked.ts plays by it):
 * its start's playlists answered as loaded (D502 §3), its start's second picture chunk asked beside
 * its first (D502 §2), each chunk's load followed for a wait watched (link-wait.ts), shaped by the
 * tab's debug cap and drop (D380 §2), its sound moved earlier.
 */

import Hls, { type HlsConfig } from "hls.js";

import { shiftingFragmentLoader, type SoundShift } from "./audio-shift.ts";
import { shapedLoader, type LoaderClass } from "./link-shaping.ts";
import { LinkWaits } from "./link-wait.ts";
import { loadedPlaylistLoader, type StartingPlaylists } from "./loaded-playlists.ts";
import { loadsNoChunkAgain } from "./no-chunk-again.ts";
import { StartReadAhead } from "./start-read-ahead.ts";

/**
 * The loader of a playback's chunks: carrier, hls.js's own unless the page gives a fetch, shaped by
 * the tab's debug cap and drop in the chunk fetch so every carrier is shaped alike (D380 §2), each
 * load followed as it comes for the waits linkWaits watches, its start's second picture chunk asked
 * beside its first (readAhead, D502 §2), its sound moved earlier by soundShift.
 */
export function chunkLoader(
  parts: {
    readonly soundShift: SoundShift;
    readonly readAhead: StartReadAhead;
    readonly linkWaits: LinkWaits;
  },
  carrier: LoaderClass = Hls.DefaultConfig.loader,
): ReturnType<typeof shiftingFragmentLoader> {
  const { soundShift, readAhead, linkWaits } = parts;
  return shiftingFragmentLoader(
    readAhead.loaderOf(linkWaits.loaderOf(shapedLoader(carrier))),
    () => soundShift.earlierMs,
  );
}

/**
 * A playback's hls.js by config, its start's playlists answered from options', its chunks loaded by
 * chunkLoader, the start's read-ahead it reads by, and its loads' waits.
 */
export function chunkedHls(
  config: Partial<HlsConfig>,
  options: { readonly loadedPlaylists?: StartingPlaylists | undefined },
  parts: {
    readonly soundShift: SoundShift;
    readonly abr: { readonly controller: HlsConfig["abrController"] };
  },
): { readonly hls: Hls; readonly readAhead: StartReadAhead; readonly linkWaits: LinkWaits } {
  const readAhead = new StartReadAhead();
  const linkWaits = new LinkWaits(() => performance.now()); // hls.js's clock for its loads
  const hls = new Hls({
    ...config,
    fLoader: chunkLoader({ soundShift: parts.soundShift, readAhead, linkWaits }, config.loader),
    pLoader: loadedPlaylistLoader(
      options.loadedPlaylists,
      config.loader ?? Hls.DefaultConfig.loader,
    ),
    abrController: parts.abr.controller,
  });
  readAhead.readsOn(hls);
  loadsNoChunkAgain(hls); // a switch loads no chunk again that it holds at another (D503 §4)
  return { hls, readAhead, linkWaits };
}

// SPDX-License-Identifier: MPL-2.0

/** What telling a cache hit reads of the page: its Resource Timing, and room kept in its buffer. */
export interface ResourceTimings extends Pick<
  Performance,
  "getEntriesByName" | "clearResourceTimings"
> {
  addEventListener: (type: "resourcetimingbufferfull", listener: () => void) => void;
}

/** The pages whose timing buffer is already kept with room. */
const kept = new WeakSet<ResourceTimings>();

/**
 * How to tell whether the browser took a resource, as it last fetched it, from its cache: its
 * Resource Timing entry transferred nothing for a body it has. hls.js cannot tell (hls.js#5826),
 * and a load under abr-choice.ts's cacheHitUnderMs was the guess (D265 §7; idiosyncrasies.md).
 * Undefined where the browser says nothing: no entry for it, or no transferSize, or a load a service
 * worker took that transferred nothing. Chromium gives a response the worker built, as it builds
 * every chunk a page on WebRTC fetches through its transport (rtc-media.ts), a transferSize of 0 for
 * a body it has, as it gives a cache hit, so every such chunk read as one: Silk on a Fire TV and
 * Chrome on a Mac measured their link with no chunk at all, and reported none (staging, 2026-10-27
 * and 28; idiosyncrasies.md). The buffer, 250 entries unless a page asks for more, is emptied each
 * time it fills, so a film's chunks past the 250th are told too.
 */
export function fromCacheBy(timings: ResourceTimings): (url: string) => boolean | undefined {
  if (!kept.has(timings)) {
    kept.add(timings);
    timings.addEventListener("resourcetimingbufferfull", () => {
      timings.clearResourceTimings();
    });
  }
  return (url) => {
    const entry = timings.getEntriesByName(url, "resource").at(-1);
    if (entry === undefined || !("transferSize" in entry) || !("decodedBodySize" in entry)) {
      return undefined;
    }
    const { transferSize, decodedBodySize } = entry;
    if (
      typeof transferSize !== "number" ||
      typeof decodedBodySize !== "number" ||
      decodedBodySize === 0
    ) {
      return undefined; // a body it does not say the size of: nothing told
    }
    if (transferSize === 0 && "workerStart" in entry && Number(entry.workerStart) > 0) {
      return undefined; // a service worker's answer, or its page's cache: nothing told
    }
    return transferSize === 0;
  };
}

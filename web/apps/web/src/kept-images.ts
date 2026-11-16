// SPDX-License-Identifier: MPL-2.0

/**
 * The core's images the service worker keeps (D485). Over WebRTC the worker builds each image's
 * answer from what the page fetched, so neither the browser's own cache nor any other kept it, and
 * the core's year-long `immutable` (internal/core/api's cacheImmutable) did nothing there: every
 * page opened fetched Home's posters again, some 1.8 MB (the slow-link war game, 2026-11-14). An
 * image's address names its content, its source's SHA-256 and its size (artUrl), so the answer kept
 * by it is the answer for good. Only the page's 200 to the very address asked for is kept: the core
 * answers a size not made yet with a 307 to the nearest one (D212 §5), and a direct path's fetch
 * follows it, which must never be kept under the size asked for. Forgotten as the session changes,
 * as the tab's answers are (D478).
 */

import { artPathPrefix, isAnsweredUnredirected } from "@orpheum/sdk";

/** The cache the core's images are kept in, by their address's path. */
export const keptImagesCache = "orpheum-images";

/**
 * How many images the worker keeps, the oldest kept going first past it: a justified constant, not a
 * setting. Home's rows and the poster grid of a library of about a thousand titles fit in it, and at
 * a poster's tens of kilobytes (portrait-m, D212) it comes to some 50–75 MB, well inside what Chromium
 * and Firefox give an origin on a device with a few gigabytes free. A browser short of room may drop
 * it all, which costs only fetching them again.
 */
export const keptImagesMost = 1500;

/** How many of the oldest go at once past keptImagesMost, so their list is read once per as many kept. */
const keptImagesDroppedAtOnce = keptImagesMost / 10;

/** An image's address whose path names its content: artUrl's, its source's SHA-256 and its size. */
const contentAddressedImage = new RegExp(
  `^${artPathPrefix}[0-9a-f]{64}/[a-z]+-[a-z]+\\.(?:webp|jpeg)$`,
  "u",
);

/** Whether pathname is an image's address that names its content (artUrl). */
export function isContentAddressedImage(pathname: string): boolean {
  return contentAddressedImage.test(pathname);
}

/** The query a page asks for an image again by, once it failed to show (poster.tsx's againUrl). */
export const askedAgainQuery = "again";

/** How many images this worker counts kept since it last read their list; unread when undefined. */
let keptCount: number | undefined;

/**
 * The answer to a load of an image by its content-addressed path: as kept, else as fetched, which
 * is kept behind it (keepBehind, the fetch event's waitUntil) when it is the image's own. Kept by its
 * path alone. A page asks for an image again, by `?again=` (poster.tsx), only once it failed to show,
 * so a kept one asked for again is taken to be broken (D494): it is dropped, told (the page's log),
 * and fetched and kept afresh. Where the browser refuses the cache, it is answered as fetched.
 */
export async function imageAsKept(
  caches: Pick<CacheStorage, "open">,
  load: {
    readonly request: Request;
    readonly fetched: () => Promise<Response>;
    readonly keepBehind: (work: Promise<unknown>) => void;
    readonly told: (note: string) => void;
  },
): Promise<Response> {
  const { request, fetched, keepBehind, told } = load;
  const url = new URL(request.url);
  const key = url.pathname;
  const cache = await caches.open(keptImagesCache).catch(() => undefined);
  const kept = await cache?.match(key).catch(() => undefined);
  if (kept !== undefined && !url.searchParams.has(askedAgainQuery)) {
    return kept;
  }
  if (kept !== undefined) {
    told(
      `the service worker dropped a kept image the page could not show, and fetched it again: ${key}`,
    );
    await cache?.delete(key).catch(() => false);
  }
  const answer = await fetched();
  if (cache !== undefined && isTheImageItself(answer)) {
    keepBehind(keep(cache, key, answer.clone()).catch(() => undefined));
  }
  return answer;
}

/**
 * Whether answer is the image asked for, and only it: a 200 of an image that its page fetched with no
 * redirect followed. A network's answer is never kept here: the browser's own cache keeps those.
 */
function isTheImageItself(answer: Response): boolean {
  return (
    answer.status === 200 &&
    isAnsweredUnredirected(answer) &&
    (answer.headers.get("Content-Type") ?? "").startsWith("image/")
  );
}

/**
 * Keeps answer under key, and drops the oldest kept once more than keptImagesMost are: a cache lists
 * its entries in the order they were kept.
 */
async function keep(cache: Cache, key: string, answer: Response): Promise<void> {
  await cache.put(key, answer);
  keptCount = keptCount === undefined ? (await cache.keys()).length : keptCount + 1;
  if (keptCount <= keptImagesMost) {
    return;
  }
  const keys = await cache.keys();
  const dropped = keys.slice(
    0,
    Math.max(0, keys.length - keptImagesMost + keptImagesDroppedAtOnce),
  );
  await Promise.all(dropped.map((oldest) => cache.delete(oldest)));
  keptCount = keys.length - dropped.length;
}

/**
 * Forgets every image the worker kept, as the session changes, so no one's page draws what an earlier
 * session was shown; nothing where the browser has no caches.
 */
export async function forgetKeptImages(): Promise<void> {
  if (!("caches" in globalThis)) {
    return;
  }
  await globalThis.caches.delete(keptImagesCache).catch(() => undefined);
}

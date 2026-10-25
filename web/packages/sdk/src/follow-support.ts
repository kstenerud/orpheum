// SPDX-License-Identifier: MPL-2.0

/** A page's global by its name, as globalThis holds it, or undefined. */
export type GlobalOf = (name: string) => unknown;

/** This page's globals. */
const pageGlobal: GlobalOf = (name) => Reflect.get(globalThis, name);

/** The containers and codecs hls.js asks a page's MediaSource about, any one of which is enough. */
const askedTypes = [
  "video/mp4;codecs=avc1.42E01E,mp4a.40.2",
  "video/mp4;codecs=av01.0.01M.08",
  "video/mp4;codecs=vp09.00.50.08",
  "audio/mp4;codecs=mp4a.40.2",
  "audio/mp4;codecs=fLaC",
];

/**
 * Whether this page can play in step with a Watch Together group: only hls.js can be nudged and
 * told what to load, so it needs MSE, or on an iPhone ManagedMediaSource, iOS 17.1 and later
 * (D250 §7). Below that an iPhone has only its own HLS, which watches alone.
 *
 * It answers as hls.js's own Hls.isSupported() does (hls.js 1.7, is-supported.ts), without loading
 * hls.js: a page asks it as it starts, and loads the player only to play, since hls.js was most of
 * the home page's script (the audit of 2026-10-25, W1). The player's
 * follow-support.browser.test.ts holds the two equal in each browser the tests run.
 */
export function canFollowAGroup(globalOf: GlobalOf = pageGlobal): boolean {
  const mediaSource = ["ManagedMediaSource", "MediaSource", "WebKitMediaSource"]
    .map(globalOf)
    .find((each) => each !== undefined && each !== null);
  // Any value will do, as for hls.js: an older WebKit's constructors are objects, not functions.
  if (mediaSource === undefined || !hasWholeSourceBuffer(globalOf)) {
    return false;
  }
  const isTypeSupported: unknown = Reflect.get(Object(mediaSource), "isTypeSupported");
  return (
    typeof isTypeSupported === "function" &&
    askedTypes.some((type) => Reflect.apply(isTypeSupported, mediaSource, [type]) === true)
  );
}

/**
 * Whether the page's SourceBuffer, where it exposes one, has the API hls.js needs; an older browser
 * exposes none, which hls.js takes as enough.
 */
function hasWholeSourceBuffer(globalOf: GlobalOf): boolean {
  const sourceBuffer = globalOf("SourceBuffer") ?? globalOf("WebKitSourceBuffer");
  if (sourceBuffer === undefined || sourceBuffer === null) {
    return true;
  }
  const prototype: unknown = Reflect.get(Object(sourceBuffer), "prototype");
  return (
    typeof prototype === "object" &&
    prototype !== null &&
    typeof Reflect.get(prototype, "appendBuffer") === "function" &&
    typeof Reflect.get(prototype, "remove") === "function"
  );
}

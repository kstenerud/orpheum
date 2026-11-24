// SPDX-License-Identifier: MPL-2.0

/**
 * How the browser tests launch their browsers: with a fake microphone, handed over without asking,
 * and host candidates by their addresses, not mDNS names a container cannot resolve, so two pages
 * call each other (D252 §3).
 */

export const chromiumArguments: string[] = [
  "--use-fake-device-for-media-stream",
  "--use-fake-ui-for-media-stream",
  "--disable-features=WebRtcHideLocalIpsWithMdns",
];

export const firefoxUserPrefs = {
  "media.navigator.streams.fake": true,
  "media.navigator.permission.disabled": true,
  "media.peerconnection.ice.obfuscate_host_addresses": false,
};

/**
 * How many test pages each browser runs at once on a machine of `cores` cores: one for every two
 * (the owner, 2026-11-24), the rest left to the dev server, the browsers' own processes and the
 * machine's other work. On 8 cores Firefox took no less time at 6 or 7 pages than at 4, and its
 * tests typing into a field that commits on leaving it failed more often as pages were added;
 * Chromium took about 9 s more at 4 than at 7.
 */
export function pagesAtOnce(cores: number): number {
  return Math.max(1, Math.floor(cores / 2));
}

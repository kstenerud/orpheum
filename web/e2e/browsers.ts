// SPDX-License-Identifier: MPL-2.0

/**
 * How the browser tests launch their browsers: with a fake microphone, handed over without asking,
 * and host candidates by their addresses, not mDNS names a container cannot resolve, so two pages
 * call each other (D252 §3).
 */

export const chromiumArguments = [
  "--use-fake-device-for-media-stream",
  "--use-fake-ui-for-media-stream",
  "--disable-features=WebRtcHideLocalIpsWithMdns",
];

export const firefoxUserPrefs = {
  "media.navigator.streams.fake": true,
  "media.navigator.permission.disabled": true,
  "media.peerconnection.ice.obfuscate_host_addresses": false,
};

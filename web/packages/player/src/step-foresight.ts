// SPDX-License-Identifier: MPL-2.0

/**
 * How a player names the quality it foresees stepping down to (D532 §1): by the stream its
 * playlist's address names. The word itself, its event and its reader are the sdk's
 * (step-down-foresight.ts), which a film's page reads without the player.
 */

/**
 * The stream a quality's playlist address names: the folder its stream.m3u8 is in, as the core's
 * master playlist writes it ("720p/stream.m3u8"); undefined for an address naming none.
 */
export function pictureStreamOf(uri: string | undefined): string | undefined {
  const parts = uri?.split(/[?#]/u)[0]?.split("/");
  if (parts === undefined || parts.length < 2 || parts.at(-1) !== "stream.m3u8") {
    return undefined;
  }
  const stream = parts.at(-2);
  return stream === "" ? undefined : stream;
}

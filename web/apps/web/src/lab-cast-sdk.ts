// SPDX-License-Identifier: MPL-2.0

/**
 * Google's Cast sender SDK as the lab's cast probe uses it (plans/tv-device-tests.md;
 * casting-2026-10-25.md §5.2 step 2): the client's adapter (cast-sdk.ts), loaded at the probe's own
 * load, with what the probe alone does besides: Google's Default Media Receiver handed a URL, and the
 * remote player's changes logged.
 */

import {
  answerOf,
  at,
  call,
  channelOf,
  make,
  openCastSession,
  type CastChannel,
} from "./cast-sdk.ts";

export { loadCastSender } from "./cast-sdk.ts";

/** Google's Default Media Receiver, which needs no registration. */
export const defaultReceiverAppId = "CC1AD845";

/** The namespace the lab's sender and receiver speak on. */
export const labNamespace = "urn:x-cast:org.orpheum.lab";

/** A session with a receiver, as the probe uses it. */
export interface CastSession extends CastChannel {
  /** Has the receiver load url, of contentType, as HLS in fMP4 segments; answers its error code, if any. */
  readonly load: (url: string, contentType: string) => Promise<string | undefined>;
  /** Tells listener each change of the remote player's state, by its field and value. */
  readonly onPlayer: (listener: (field: string, value: unknown) => void) => void;
}

/**
 * Opens a session with the receiver app appId, asking the person to pick a device (the press that
 * asks must be the person's); fails where they pick none or it cannot launch.
 */
export async function castTo(appId: string): Promise<CastSession> {
  return sessionOf(await openCastSession(appId));
}

/** Has session load url, of contentType, as HLS in fMP4; answers the receiver's error code, if any. */
async function loadOn(
  session: unknown,
  url: string,
  contentType: string,
): Promise<string | undefined> {
  const castMedia = (...path: readonly string[]): unknown =>
    at(globalThis, "chrome", "cast", "media", ...path);
  const media = make(["chrome", "cast", "media", "MediaInfo"], url, contentType);
  Reflect.set(media, "hlsSegmentFormat", castMedia("HlsSegmentFormat", "FMP4"));
  Reflect.set(media, "hlsVideoSegmentFormat", castMedia("HlsVideoSegmentFormat", "FMP4"));
  Reflect.set(media, "streamType", castMedia("StreamType", "BUFFERED"));
  const request = make(["chrome", "cast", "media", "LoadRequest"], media);
  const code = await answerOf(call(session, "loadMedia", request));
  return typeof code === "string" ? code : undefined;
}

/** The probe's view of the SDK's session. */
function sessionOf(session: unknown): CastSession {
  return {
    ...channelOf(session, labNamespace),
    load: async (url, contentType) => await loadOn(session, url, contentType),
    onPlayer: (listener) => {
      const player = make(["cast", "framework", "RemotePlayer"]);
      const controller = make(["cast", "framework", "RemotePlayerController"], player);
      const anyChange = at(globalThis, "cast", "framework", "RemotePlayerEventType", "ANY_CHANGE");
      call(controller, "addEventListener", anyChange, (event: unknown) => {
        const field = at(event, "field");
        listener(typeof field === "string" ? field : "", at(event, "value"));
      });
    },
  };
}

/**
 * Whether a remote player's change is one the probe logs: its state, what it loaded, its length and
 * its errors; not each step of its position, which it reads otherwise.
 */
export function isLoggedPlayerField(field: string): boolean {
  return ["playerState", "isMediaLoaded", "duration", "isConnected", "displayStatus"].includes(
    field,
  );
}

// SPDX-License-Identifier: MPL-2.0

import {
  carryMediaOver,
  connectRtc,
  dataOrThrow,
  type Api,
  type SwitchableFetch,
} from "@orpheum/sdk";

import { storageThatNeverThrows, type StringStorage } from "./browser-storage.ts";
import type { Session } from "./session.ts";

/** The address parameter that asks for the WebRTC path: `?transport=webrtc`. */
export const transportParameter = "transport";

/** What the WebRTC path is called where the console says requests moved to it. */
export const pathName = "WebRTC";

/** Where the choice is kept for the rest of the tab's life, as the address loses it on navigating. */
const transportStorageKey = "orpheum.transport";

/**
 * Whether this tab sends its requests over WebRTC data channels (D273 §8). Until the discovery
 * signals for a core HTTPS cannot reach (plans/webrtc-transport.md W4), the path is asked for by
 * `?transport=webrtc`, which tests and the owner use to exercise it; the tab keeps the choice.
 */
export function wantsWebrtc(search: string, tabStorage: StringStorage | undefined): boolean {
  const storage = storageThatNeverThrows(tabStorage);
  const asked = new URLSearchParams(search).get(transportParameter);
  if (asked !== null) {
    storage.setItem(transportStorageKey, asked);
  }
  return storage.getItem(transportStorageKey) === "webrtc";
}

/**
 * Once the session is signed in, connects to the core over WebRTC through its offer endpoint and
 * moves the API's requests onto the data channels, then the loads the browser makes by itself through
 * the service worker; a failure leaves them on HTTPS, and says why.
 */
export function moveToWebrtcWhenSignedIn(
  app: { readonly session: Session; readonly api: Api; readonly routed: SwitchableFetch },
  report: (problem: unknown) => void,
): void {
  const { session, api, routed } = app;
  let isStarted = false;
  const start = (): void => {
    if (isStarted || session.token() === undefined) {
      return;
    }
    isStarted = true;
    connectRtc(async (offer) => dataOrThrow(await api.POST("/rtc/offer", { body: offer })))
      .then(async (transport) => {
        routed.use(transport.fetch, pathName);
        await carryMediaOver(transport.fetch, session.token);
      })
      .catch((problem: unknown) => {
        isStarted = false;
        report(problem);
      });
  };
  session.subscribe(start);
  start();
}

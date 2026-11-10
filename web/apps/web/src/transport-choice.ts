// SPDX-License-Identifier: MPL-2.0

import {
  carryMediaOver,
  tellMediaCarriers,
  connectRtc,
  dataOrThrow,
  reaching,
  rtcTogetherSocket,
  type Api,
  type OpenTogetherSocket,
  type RtcTransport,
  type Signaller,
  type SwitchableFetch,
} from "@orpheum/sdk";

import type { Session } from "./session.ts";
import { mediaCredentialsOf } from "./together-playback.ts";
import { pathName } from "./transport-wanted.ts";

/**
 * The signaller of the direct mode: posts each offer to the core's own offer endpoint. A post that
 * reached no server fails as Unreached (reaching), so the transport tries again after unreachedRetryMs
 * rather than after its growing wait, as the discovery's does.
 */
export function directSignaller(api: Api): Signaller {
  return async (offer, signal) =>
    dataOrThrow(await reaching(api.POST("/rtc/offer", { body: offer, signal: signal ?? null })));
}

/** How a tab connects over WebRTC and carries its media over: the SDK's; a test's own fakes. */
export interface WebrtcWays {
  readonly connect: (
    signaller: Signaller,
  ) => Promise<Pick<RtcTransport, "fetch" | "openChannel" | "carrierOf">>;
  readonly tellMediaCarriers: typeof tellMediaCarriers;
  readonly carryMediaOver: typeof carryMediaOver;
}

const sdkWebrtcWays: WebrtcWays = {
  connect: (signaller) => connectRtc(signaller),
  tellMediaCarriers,
  carryMediaOver,
};

/**
 * Once the session is signed in, connects to the core over WebRTC through its offer endpoint and
 * moves the API's requests onto the data channels, then the loads the browser makes by itself through
 * the service worker, and tells pathMoves once they are, so a film page there starts no preload the
 * Play page would load again (D260 §3's revision of 2026-10-28); a failure leaves them on HTTPS, and
 * says why.
 */
export function moveToWebrtcWhenSignedIn(
  app: {
    readonly session: Session;
    readonly api: Api;
    readonly routed: SwitchableFetch;
    /** Moves the session sockets the app opens from now on to open. */
    readonly switchTogetherSocket: (open: OpenTogetherSocket) => void;
    /** Told once the media are carried over, so PathMoves.isOverWebrtc says so (services.tsx). */
    readonly pathMoves: { readonly carriedOverWebrtc: () => void };
  },
  report: (problem: unknown) => void,
  ways: WebrtcWays = sdkWebrtcWays,
): void {
  const { session, api, routed } = app;
  let isStarted = false;
  const start = (): void => {
    if (isStarted || session.token() === undefined) {
      return;
    }
    isStarted = true;
    ways
      .connect(directSignaller(api))
      .then(async (transport) => {
        routed.use(transport.fetch, pathName);
        app.switchTogetherSocket(rtcTogetherSocket(() => transport.openChannel()));
        ways.tellMediaCarriers((url) => transport.carrierOf(url));
        await ways.carryMediaOver(transport.fetch, mediaCredentialsOf(session), pathName);
        app.pathMoves.carriedOverWebrtc();
      })
      .catch((problem: unknown) => {
        isStarted = false;
        report(problem);
      });
  };
  session.subscribe(start);
  start();
}

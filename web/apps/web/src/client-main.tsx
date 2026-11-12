// SPDX-License-Identifier: MPL-2.0

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import {
  browserFetch,
  browserPlayableFormats,
  browserTogetherSocket,
  browserVoiceDevices,
  canFollowAGroup,
  createApi,
  keepPageAudio,
  reportFailure,
  reportPathNotes,
  sayMediaGoesOverHttp,
  SwitchableFetch,
  type Api,
} from "@orpheum/sdk";
import { StrictMode } from "react";
import { createRoot, type Root } from "react-dom/client";

import { ownScriptOf, pageUpdateWatch } from "./app-update.ts";
import { App } from "./app.tsx";
import {
  onBrowserOnline,
  reachesUnderLoader,
  takeMediaCarriers,
  type Reaches,
} from "./client-reaches.ts";
import { removeLeftOverKeys } from "./client-keys.ts";
import { storageIfAllowed, tabStorage } from "./browser-storage.ts";
import { sendFailuresToCore } from "./failure-reports.ts";
import { askGuestCastGrantOf } from "./google-cast.ts";
import { followPressesForRefusals } from "./start-refused.ts";
import { followPressesOfPlay } from "./video-pressed-for-play.ts";
import { LinkDebug } from "./link-debug.ts";
import { loaderOf, loaderRefusal, type OrpheumLoader } from "./loader-interface.ts";
import { createNavigation } from "./navigation.ts";
import { isFilmPlayingOn, reportPageFreezes } from "./page-freezes.ts";
import { notePolicyViolations } from "./policy-violations.ts";
import { newerClientOver, reportReloadNote, watchForNewerClient } from "./newer-client.ts";
import { i18n } from "./i18n.ts";
import { createPathMoves, ServicesContext } from "./services.tsx";
import { createSession, type Session } from "./session.ts";
import {
  checkSessionWhileShown,
  createLiveOpenMark,
  signingOutWithNoneHeld,
  watchForSignOut,
} from "./signed-out-watch.ts";
import { mediaCredentialsOf } from "./together-playback.ts";
import { wantsWebrtc } from "./transport-wanted.ts";

const rootElement = document.getElementById("root");
if (rootElement === null) {
  throw new Error("index.html has no #root element; the app cannot mount");
}

// A film plays as a video app's, the silent switch not muting it, and the session is healed as the
// page comes back from the background (page-audio.ts).
keepPageAudio(document);
const navigation = createNavigation(window);
const root = createRoot(rootElement);
// What the page's report-only policy would refuse goes to the core's log, found on the devices
// before the policy is enforced (policy-violations.ts).
notePolicyViolations(document);
// The page's freezes of a second or more go to the core's log, on every screen, saying which.
reportPageFreezes({
  screen: () => navigation.route().screen,
  isFilmPlaying: () => isFilmPlayingOn(document),
});

/**
 * The tab's debug link (D380): its cap kept in the tab's session storage, a drop timed on the page's
 * clock and ended with the browser's `online`.
 */
const linkDebug = new LinkDebug(
  tabStorage(),
  {
    nowMs: () => performance.now(),
    setTimeout: (run, ms) => setTimeout(run, ms),
    clearTimeout: (timer) => {
      clearTimeout(timer);
    },
  },
  window,
);

/** Mounts the app over session on root, its requests sent as reaches says, shaped by linkDebug. */
function mountApp(sessionGiven: Session, given: Reaches, mountedOn: Root): Api {
  // A sign-out waits for what is held over the page to go, so its leaving is never cancelled.
  const session = signingOutWithNoneHeld(sessionGiven, navigation);
  const reaches = linkDebug.shape(given);
  const api = createApi({ getToken: session.token, fetch: reaches.fetch });
  api.use(watchForSignOut(session, navigation));
  // A device removed elsewhere goes to sign in as soon as its page is looked at, not at its next click;
  // while the live connection is open, it says so itself, and the minute's check rests.
  const liveOpen = createLiveOpenMark();
  checkSessionWhileShown(
    async () => await api.GET("/me"),
    { token: session.token, isLiveOpen: liveOpen.isOpen },
    document,
  );
  api.use(pageUpdateWatch.middleware(reaches.ownScript));
  sendFailuresToCore(api, session, () => window.location.pathname);
  // A film the browser will not start says how long ago the page's last press was (start-refused.ts,
  // VQ272).
  followPressesForRefusals();
  // An iPhone's Play page plays in a video readied by the press that opened it (VQ297).
  followPressesOfPlay(document);
  // A screen's code gone with an update reloads the page into the newer client, once, said here.
  watchForNewerClient(newerClientOver(api, reaches.ownScript));
  reportReloadNote();
  if ("serviceWorker" in navigator) {
    // Where the worker sent the page's media, and why, goes to the core's log with its failures.
    reportPathNotes(navigator.serviceWorker, (note) => {
      reportFailure(`the service worker: ${note}`);
    });
  }
  const services = {
    api,
    session,
    navigation,
    playableFormats: browserPlayableFormats,
    openTogetherSocket: reaches.openTogetherSocket,
    canFollowAGroup,
    voiceDevices: browserVoiceDevices,
    fetchMedia: browserFetch,
    realm: reaches.realm,
    // Under a discovery's loader, what the lab's cast page hands its receiver to reach the core from
    // away; only a Chromecast away from home shows it whole (VQ256).
    throughDiscovery: reaches.throughDiscovery,
    // The loader's Cast receiver application, for the Cast menu's Chromecast (D465 §5), and the grant
    // for one cast it hands the device it launches on (D465 §2).
    castReceiverAppId: reaches.castReceiverAppId,
    askGuestCastGrant: askGuestCastGrantOf(api),
    willLeaveWhenSignedOut: reaches.willLeaveWhenSignedOut,
    pathMoves: reaches.pathMoves,
    switchHousehold: reaches.switchHousehold,
    codeAddress: reaches.codeAddress,
    onLocalNetworkAsking: reaches.onLocalNetworkAsking,
    onPathBack: reaches.onPathBack,
    linkDebug,
    liveOpen,
  };
  mountedOn.render(
    <StrictMode>
      <ServicesContext value={services}>
        <App />
      </ServicesContext>
    </StrictMode>,
  );
  return api;
}

/** Runs the app on the core's own origin: its own session, and the browser's own fetch. */
function mountOnOwnOrigin(): void {
  if ("serviceWorker" in navigator) {
    sayMediaGoesOverHttp(navigator.serviceWorker);
  }
  const routed = new SwitchableFetch();
  let openTogetherSocket = browserTogetherSocket(window.location.origin);
  const session = createSession(storageIfAllowed(() => window.localStorage));
  const pathMoves = createPathMoves();
  const api = mountApp(
    session,
    {
      fetch: routed.fetch,
      openTogetherSocket: (listeners) => openTogetherSocket(listeners),
      pathMoves,
      ownScript: ownScriptOf(document),
      onPathBack: onBrowserOnline,
    },
    root,
  );
  if (
    wantsWebrtc(
      window.location.search,
      storageIfAllowed(() => window.sessionStorage),
    )
  ) {
    // The path's code, the WebRTC SDK with it, loads only for a tab that asked for it (D273 §8; the
    // architecture audit of 2026-11-10, Startup 6). It moves once signed in, so a sign-in while it
    // loads is moved as it comes.
    import("./transport-choice.ts").then(
      ({ moveToWebrtcWhenSignedIn }) => {
        moveToWebrtcWhenSignedIn(
          {
            session,
            api,
            routed,
            switchTogetherSocket: (open) => {
              openTogetherSocket = open;
            },
            pathMoves,
          },
          reportFailure,
        );
      },
      (problem: unknown) => {
        reportFailure(problem);
      },
    );
  }
}

/**
 * Runs the app under a discovery's loader, which reached the core and keeps the session at this
 * client origin (D308): every request, the session socket and the media go the loader's way.
 */
function mountUnderLoader(loader: OrpheumLoader): void {
  loader.setMediaCredentials(mediaCredentialsOf(loader.session));
  takeMediaCarriers(loader);
  mountApp(loader.session, reachesUnderLoader(loader, document), root);
}

// What an older release kept, and no part of this one reads, goes (the owner, 2026-10-17).
removeLeftOverKeys([
  storageIfAllowed(() => window.localStorage),
  storageIfAllowed(() => window.sessionStorage),
]);
const loader = loaderOf(window);
if (loader === undefined) {
  mountOnOwnOrigin();
} else {
  const refusal = loaderRefusal(loader);
  if (refusal === undefined) {
    mountUnderLoader(loader);
  } else {
    rootElement.textContent = i18n.t("loader.refused", { ...refusal });
  }
}

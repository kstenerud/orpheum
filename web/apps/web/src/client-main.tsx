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
import { LinkDebug } from "./link-debug.ts";
import { loaderOf, loaderRefusal, type OrpheumLoader } from "./loader-interface.ts";
import { createNavigation } from "./navigation.ts";
import { isFilmPlayingOn, reportPageFreezes } from "./page-freezes.ts";
import {
  newerClientOver,
  reportReloadForNewerClient,
  watchForNewerClient,
} from "./newer-client.ts";
import { i18n } from "./i18n.ts";
import { createPathMoves, ServicesContext } from "./services.tsx";
import { createSession, type Session } from "./session.ts";
import { checkSessionWhileShown, watchForSignOut } from "./signed-out-watch.ts";
import { mediaCredentialsOf } from "./together-playback.ts";
import { moveToWebrtcWhenSignedIn, wantsWebrtc } from "./transport-choice.ts";

const rootElement = document.getElementById("root");
if (rootElement === null) {
  throw new Error("index.html has no #root element; the app cannot mount");
}

// A film plays as a video app's, the silent switch not muting it, and the session is healed as the
// page comes back from the background (page-audio.ts).
keepPageAudio(document);
const navigation = createNavigation(window);
const root = createRoot(rootElement);
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
function mountApp(session: Session, given: Reaches, mountedOn: Root): Api {
  const reaches = linkDebug.shape(given);
  const api = createApi({ getToken: session.token, fetch: reaches.fetch });
  api.use(watchForSignOut(session, navigation));
  // A device removed elsewhere goes to sign in as soon as its page is looked at, not at its next click.
  checkSessionWhileShown(async () => await api.GET("/me"), session, document);
  api.use(pageUpdateWatch.middleware(reaches.ownScript));
  sendFailuresToCore(api, session, () => window.location.pathname);
  // A screen's code gone with an update reloads the page into the newer client, once, said here.
  watchForNewerClient(newerClientOver(api, reaches.ownScript));
  reportReloadForNewerClient();
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
    pathMoves: reaches.pathMoves,
    switchHousehold: reaches.switchHousehold,
    codeAddress: reaches.codeAddress,
    onLocalNetworkAsking: reaches.onLocalNetworkAsking,
    onPathBack: reaches.onPathBack,
    linkDebug,
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
  const api = mountApp(
    session,
    {
      fetch: routed.fetch,
      openTogetherSocket: (listeners) => openTogetherSocket(listeners),
      pathMoves: createPathMoves(),
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
    moveToWebrtcWhenSignedIn(
      {
        session,
        api,
        routed,
        switchTogetherSocket: (open) => {
          openTogetherSocket = open;
        },
      },
      reportFailure,
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

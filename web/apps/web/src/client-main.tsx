// SPDX-License-Identifier: MPL-2.0

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { canFollowAGroup } from "@orpheum/player";
import {
  browserFetch,
  browserPlayableFormats,
  browserTogetherSocket,
  browserVoiceDevices,
  createApi,
  keepPageAudio,
  reportFailure,
  SwitchableFetch,
  type Api,
  type OpenTogetherSocket,
} from "@orpheum/sdk";
import { StrictMode } from "react";
import { createRoot, type Root } from "react-dom/client";

import { builtScriptOf, ownScriptOf, pageUpdateWatch } from "./app-update.ts";
import { App } from "./app.tsx";
import { removeLeftOverKeys } from "./client-keys.ts";
import { storageIfAllowed } from "./browser-storage.ts";
import { sendFailuresToCore } from "./failure-reports.ts";
import { loaderOf, loaderRefusal, type OrpheumLoader } from "./loader-interface.ts";
import { createNavigation } from "./navigation.ts";
import { i18n } from "./i18n.ts";
import {
  createPathMoves,
  pathMovesHeardFrom,
  ServicesContext,
  type PathMoves,
} from "./services.tsx";
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

/** How the app reaches its core: what its requests are sent by, and the session socket's opener. */
interface Reaches {
  readonly fetch: (request: Request) => Promise<Response>;
  readonly openTogetherSocket: OpenTogetherSocket;
  readonly pathMoves: PathMoves;
  /** The page's own client script, which the core's answers are compared with for an update. */
  readonly ownScript: string | undefined;
  readonly realm?: string | undefined;
  readonly switchHousehold?: (() => void) | undefined;
}

/** Mounts the app over session on root, its requests sent as reaches says. */
function mountApp(session: Session, reaches: Reaches, mountedOn: Root): Api {
  const api = createApi({ getToken: session.token, fetch: reaches.fetch });
  api.use(watchForSignOut(session, navigation));
  // A device removed elsewhere goes to sign in as soon as its page is looked at, not at its next click.
  checkSessionWhileShown(async () => await api.GET("/me"), session, document);
  api.use(pageUpdateWatch.middleware(reaches.ownScript));
  sendFailuresToCore(api, session, () => window.location.pathname);
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
  const pathMoves = pathMovesHeardFrom(loader.onPathMove);
  loader.setMediaCredentials(mediaCredentialsOf(loader.session));
  mountApp(
    loader.session,
    {
      fetch: loader.fetch,
      openTogetherSocket: loader.openTogetherSocket,
      pathMoves,
      ownScript: builtScriptOf(document),
      realm: loader.core.realm,
      switchHousehold: loader.switchHousehold,
    },
    root,
  );
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

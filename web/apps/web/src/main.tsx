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
  carryMediaOver,
  createApi,
  directFetch,
  keepPageAudio,
  reportFailure,
  rtcTogetherSocket,
  SwitchableFetch,
  type Api,
  type OpenTogetherSocket,
} from "@orpheum/sdk";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { ownScriptOf, pageUpdateWatch } from "./app-update.ts";
import { App } from "./app.tsx";
import { browserStorage, storageIfAllowed } from "./browser-storage.ts";
import { createNavigation } from "./navigation.ts";
import { ServicesContext } from "./services.tsx";
import { DiscoveryGate, type GateParts } from "./discovery-gate.tsx";
import {
  discoveryClientApiOf,
  discoveryHasRealms,
  heldWith,
  rememberCore,
  startOf,
} from "./discovery-start.ts";
import { signInThisDevice } from "./accounts.ts";
import { newIdempotencyKey } from "./idempotent.ts";
import { sendFailuresToCore } from "./failure-reports.ts";
import { mediaCredentialsOf } from "./together-playback.ts";
import { readMembership } from "./together-storage.ts";
import { corePrefix, createSession, type Session } from "./session.ts";
import { checkSessionWhileShown, watchForSignOut } from "./signed-out-watch.ts";
import { moveToWebrtcWhenSignedIn, pathName, wantsWebrtc } from "./transport-choice.ts";

const rootElement = document.getElementById("root");
if (rootElement === null) {
  throw new Error("index.html has no #root element; the app cannot mount");
}

// A film plays as a video app's, the silent switch not muting it, and the session is healed as the
// page comes back from the background (page-audio.ts).
keepPageAudio(document);
const navigation = createNavigation(window);
const routed = new SwitchableFetch();
const root = createRoot(rootElement);
const localStore = storageIfAllowed(() => window.localStorage);
// The session socket the app opens: the browser's WebSocket, until the WebRTC path takes over.
let openTogetherSocket = browserTogetherSocket(window.location.origin);

/** Mounts the app over session, its requests sent through routed, naming the realm signed in to. */
function mountApp(session: Session, realm?: string): Api {
  const api = createApi({ getToken: session.token, fetch: routed.fetch });
  api.use(watchForSignOut(session, navigation));
  // A device removed elsewhere goes to sign in as soon as its page is looked at, not at its next click.
  checkSessionWhileShown(async () => await api.GET("/me"), session, document);
  api.use(pageUpdateWatch.middleware(ownScriptOf(document)));
  sendFailuresToCore(api, session, () => window.location.pathname);
  const services = {
    api,
    session,
    navigation,
    playableFormats: browserPlayableFormats,
    openTogetherSocket: ((listeners) => openTogetherSocket(listeners)) satisfies OpenTogetherSocket,
    canFollowAGroup,
    voiceDevices: browserVoiceDevices,
    fetchMedia: browserFetch,
    realm,
  };
  root.render(
    <StrictMode>
      <ServicesContext value={services}>
        <App />
      </ServicesContext>
    </StrictMode>,
  );
  return api;
}

// A build for a discovery's origin names its client API (webrtc-transport.md W4c); the core's own
// build names none, and its page is the core's.
const discoveryClientApi = discoveryClientApiOf(document);
if (discoveryClientApi === "") {
  const session = createSession(localStore);
  const api = mountApp(session);
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
} else {
  const parts: GateParts = {
    client: { clientApi: discoveryClientApi },
    hasRealms: discoveryHasRealms(document),
    realmStorage: localStore,
    sessionOf: (core) =>
      heldWith(
        core,
        createSession(localStore, corePrefix(core.coreId)),
        readMembership(browserStorage(), "viewer"),
      ),
    onConnected: (core, reached, signIn) => {
      rememberCore(localStore, core);
      const session = createSession(localStore, corePrefix(core.coreId));
      if (reached.kind === "direct") {
        // The core's own name: the page's requests and media go there, the session by its header.
        const send = directFetch(reached.origin);
        const name = `the core's name, ${reached.origin}`;
        routed.use(send, name);
        openTogetherSocket = browserTogetherSocket(reached.origin);
        void carryMediaOver(send, mediaCredentialsOf(session), name).catch(reportFailure);
      } else {
        const { transport } = reached;
        routed.use(transport.fetch, pathName);
        openTogetherSocket = rtcTogetherSocket(() => transport.openChannel());
        void carryMediaOver(transport.fetch, mediaCredentialsOf(session), pathName).catch(
          reportFailure,
        );
      }
      if (signIn === undefined) {
        mountApp(session, core.realm);
        return;
      }
      // A realm sign-in's person typed their name and password once: they sign in to the core with
      // them at once, over the path just reached, and the app opens signed in (D297 §2).
      const api = createApi({ getToken: session.token, fetch: routed.fetch });
      void signInThisDevice({ api, session }, signIn, newIdempotencyKey())
        .catch(reportFailure)
        .finally(() => {
          mountApp(session, core.realm);
        });
    },
  };
  root.render(
    <StrictMode>
      <DiscoveryGate start={startOf(window.location.hash, localStore)} parts={parts} />
    </StrictMode>,
  );
}

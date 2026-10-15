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
  namingFailures,
  reportFailure,
  rtcTogetherSocket,
  PathKeeper,
  SwitchableFetch,
  type Api,
  type FetchRequest,
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
  forgetCore,
  heldWith,
  rememberCore,
  startOf,
  whenSignedOut,
  type Reached,
  type Reaching,
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

/** What a path is called in the console, and what sends on it. */
function sendingOn(path: Reached): { readonly send: FetchRequest; readonly name: string } {
  const name = path.kind === "direct" ? `the core's name, ${path.origin}` : pathName;
  const send = path.kind === "direct" ? directFetch(path.origin) : path.transport.fetch;
  return { send: namingFailures(send, name), name };
}

/** How often the page asks whether a better path than the one in use answers now (W2c item 6). */
const betterPathEveryMs = 30_000;

/**
 * Sends the page's requests on the paths reaching found, moving to another when the one in use fails
 * and to a better one when it answers later, as at home once Firefox's local-network prompt is
 * allowed (path-keeper.ts); each path takes the session's media credentials and the session socket.
 */
function keepPath(reaching: Reaching, session: Session): { readonly close: () => void } {
  const keeper = new PathKeeper<Reached>(reaching.first, {
    sendOn: (path) => sendingOn(path).send,
    another: reaching.another,
    onUse: (path) => {
      const { send, name } = sendingOn(path);
      // eslint-disable-next-line no-console -- which path a tab's requests take is read in its console (D273 §7)
      console.info(`Orpheum: requests now go over ${name}`);
      openTogetherSocket =
        path.kind === "direct"
          ? browserTogetherSocket(path.origin)
          : rtcTogetherSocket(() => path.transport.openChannel());
      void carryMediaOver(send, mediaCredentialsOf(session), name).catch(reportFailure);
    },
  });
  routed.use(keeper.fetch, "the path kept working");
  const lookForBetter = (): void => {
    void reaching.better(keeper.path).then((better) => {
      if (better !== undefined) {
        keeper.moveTo(better);
      }
    });
  };
  const every = setInterval(lookForBetter, betterPathEveryMs);
  window.addEventListener("online", lookForBetter);
  return {
    close: () => {
      clearInterval(every);
      window.removeEventListener("online", lookForBetter);
      if (keeper.path.kind === "webrtc") {
        keeper.path.transport.close();
      }
    },
  };
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
    forgetCore: () => {
      forgetCore(localStore);
    },
    onConnected: async (core, reaching, signIn) => {
      const session = createSession(localStore, corePrefix(core.coreId));
      const keeper = keepPath(reaching, session);
      if (signIn !== undefined) {
        // A realm sign-in's person typed their name and password once: they sign in to the core with
        // them at once, over the path just reached, and the app opens signed in (D297 §2). One that
        // fails leaves them at the realm sign-in, never at the core's own over this connection.
        const api = createApi({ getToken: session.token, fetch: routed.fetch });
        try {
          await signInThisDevice({ api, session }, signIn, newIdempotencyKey());
        } catch (cause: unknown) {
          keeper.close();
          throw cause;
        }
      }
      rememberCore(localStore, core);
      // Signed out, the page starts again from nothing kept: the realm sign-in, any realm, any person.
      whenSignedOut(session, () => {
        forgetCore(localStore);
        window.location.reload();
      });
      mountApp(session, core.realm);
    },
  };
  root.render(
    <StrictMode>
      <DiscoveryGate start={startOf(window.location.hash, localStore)} parts={parts} />
    </StrictMode>,
  );
}

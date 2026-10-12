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
  type OpenTogetherSocket,
} from "@orpheum/sdk";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { ownScriptOf, pageUpdateWatch } from "./app-update.ts";
import { App } from "./app.tsx";
import { storageIfAllowed } from "./browser-storage.ts";
import { createNavigation } from "./navigation.ts";
import { ServicesContext } from "./services.tsx";
import { createSession } from "./session.ts";
import { watchForSignOut } from "./signed-out-watch.ts";
import { moveToWebrtcWhenSignedIn, wantsWebrtc } from "./transport-choice.ts";

const rootElement = document.getElementById("root");
if (rootElement === null) {
  throw new Error("index.html has no #root element; the app cannot mount");
}

// A film plays as a video app's, the silent switch not muting it, and the session is healed as the
// page comes back from the background (page-audio.ts).
keepPageAudio(document);
const session = createSession(storageIfAllowed(() => window.localStorage));
const navigation = createNavigation(window);
const routed = new SwitchableFetch();
const api = createApi({ getToken: session.token, fetch: routed.fetch });
api.use(watchForSignOut(session, navigation));
api.use(pageUpdateWatch.middleware(ownScriptOf(document)));
// The session socket the app opens: the browser's WebSocket, until the WebRTC path takes over.
let openTogetherSocket = browserTogetherSocket(window.location.origin);
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
const services = {
  api,
  session,
  navigation,
  playableFormats: browserPlayableFormats,
  openTogetherSocket: ((listeners) => openTogetherSocket(listeners)) satisfies OpenTogetherSocket,
  canFollowAGroup,
  voiceDevices: browserVoiceDevices,
  fetchMedia: browserFetch,
};

createRoot(rootElement).render(
  <StrictMode>
    <ServicesContext value={services}>
      <App />
    </ServicesContext>
  </StrictMode>,
);

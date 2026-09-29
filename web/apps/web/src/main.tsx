// SPDX-License-Identifier: MPL-2.0

import "./tokens.css";
import "./i18n.ts";

import { browserMenu, createApi } from "@orpheum/sdk";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./app.tsx";
import { createNavigation } from "./navigation.ts";
import { ServicesContext } from "./services.tsx";
import { createSession, type TokenStorage } from "./session.ts";
import { watchForSignOut } from "./signed-out-watch.ts";

const rootElement = document.getElementById("root");
if (rootElement === null) {
  throw new Error("index.html has no #root element; the app cannot mount");
}

// A private window may refuse storage outright; the session then lasts as long as the page.
let storage: TokenStorage | undefined;
try {
  storage = window.localStorage;
} catch {
  storage = undefined;
}
const session = createSession(storage);
const navigation = createNavigation(window);
const api = createApi({ getToken: session.token });
api.use(watchForSignOut(session, navigation));
const services = { api, session, navigation, playableFormats: browserMenu };

createRoot(rootElement).render(
  <StrictMode>
    <ServicesContext value={services}>
      <App />
    </ServicesContext>
  </StrictMode>,
);

// SPDX-License-Identifier: MPL-2.0

import "./tokens.css";
import "./i18n.ts";

import { browserMenu, createApi } from "@orpheum/sdk";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";

import { App } from "./app.tsx";
import { storageIfAllowed } from "./browser-storage.ts";
import { createNavigation } from "./navigation.ts";
import { ServicesContext } from "./services.tsx";
import { createSession } from "./session.ts";
import { watchForSignOut } from "./signed-out-watch.ts";

const rootElement = document.getElementById("root");
if (rootElement === null) {
  throw new Error("index.html has no #root element; the app cannot mount");
}

const session = createSession(storageIfAllowed(() => window.localStorage));
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

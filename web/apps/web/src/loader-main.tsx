// SPDX-License-Identifier: MPL-2.0

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { StrictMode, type JSX } from "react";
import { createRoot } from "react-dom/client";

import { storageIfAllowed } from "./browser-storage.ts";
import type { ClientFilesMessage } from "./client-files.ts";
import { i18n } from "./i18n.ts";
import { AtClientOriginPage } from "./loader-at-client-origin.tsx";
import { startAtLoaderOrigin } from "./loader-at-loader-origin.tsx";
import { coreIdOfOrigin, loaderConfigOf } from "./loader-config.ts";
import { removeUnownedKeys } from "./browser-storage.ts";
import { clearCookies } from "./loader-sweep.ts";

/** The keys the loader owns at its own origin: the household list, and the remembered realm. */
const ownedAtLoaderOrigin = new Set(["orpheum.loader.households", "orpheum.discovery.realm"]);

const rootElement = document.getElementById("root");
if (rootElement === null) {
  throw new Error("index.html has no #root element; the loader cannot mount");
}
const root = createRoot(rootElement);
const localStore = storageIfAllowed(() => window.localStorage);

/** Tells the service worker controlling the page the client's files, once it has kept them. */
async function tellWorker(message: ClientFilesMessage): Promise<void> {
  const controller = navigator.serviceWorker.controller;
  if (controller === null) {
    throw new Error(
      "no service worker controls the page, so the server's client cannot be checked",
    );
  }
  const { port1, port2 } = new MessageChannel();
  const kept = new Promise<void>((resolve) => {
    port1.onmessage = () => {
      resolve();
    };
  });
  controller.postMessage(message, [port2]);
  await kept;
}

/** The loader's page, at whichever origin it is served. */
function loaderPage(): JSX.Element | undefined {
  const config = loaderConfigOf(document);
  if (config === undefined) {
    return <p>{i18n.t("loader.notConfigured")}</p>;
  }
  // No page the loader serves sets a cookie: any here was set by another core's page (D308 §7).
  clearCookies(
    {
      read: () => document.cookie,
      write: (cookie) => {
        document.cookie = cookie;
      },
    },
    window.location.hostname,
  );
  const coreId = coreIdOfOrigin(window.location.origin, config.clientOrigins);
  if (coreId === undefined) {
    removeUnownedKeys(localStore, (key) => ownedAtLoaderOrigin.has(key));
    // The whole client's own worker, from before the loader, is no longer this origin's.
    void navigator.serviceWorker
      .getRegistrations()
      .then(async (registrations) => {
        await Promise.all(
          registrations.map(async (registration) => await registration.unregister()),
        );
      })
      .catch(() => undefined);
    return startAtLoaderOrigin({
      config,
      storage: localStore,
      location: window.location,
      referrer: document.referrer,
      now: () => Date.now(),
    });
  }
  return (
    <AtClientOriginPage
      at={{
        config,
        coreId,
        storage: localStore,
        location: window.location,
        history: window.history,
        loaderOrigin: config.loaderOrigin,
        unmount: () => {
          root.unmount();
        },
        tellWorker,
      }}
    />
  );
}

const page = loaderPage();
if (page !== undefined) {
  root.render(<StrictMode>{page}</StrictMode>);
}

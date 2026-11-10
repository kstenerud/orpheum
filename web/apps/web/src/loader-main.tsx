// SPDX-License-Identifier: MPL-2.0

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { noteForTheLog } from "@orpheum/sdk";
import { StrictMode, type JSX } from "react";
import { createRoot } from "react-dom/client";

import { storageIfAllowed, tabStorage } from "./browser-storage.ts";
import type { ClientFilesMessage } from "./client-files.ts";
import { i18n } from "./i18n.ts";
import { guardFormsOk } from "./form-ok.ts";
import { coreIdOfOrigin, loaderConfigOf } from "./loader-config.ts";
import { removeUnownedKeys } from "./browser-storage.ts";
import { clearCookies } from "./loader-sweep.ts";
import { loadRole, waitMs, type PartLoadParts } from "./part-load.ts";

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

/** What the loader's role is loaded with: as main.tsx loads the loader (part-load.ts). */
const roleLoadParts: PartLoadParts = {
  tab: tabStorage(),
  reload: () => {
    window.location.reload();
  },
  wait: waitMs,
};

/**
 * The loader's page, at whichever origin it is served. Each origin's role is loaded as it is
 * needed, so a page parses only the one it runs (web architecture audit, 2026-11-10: a television
 * at a client's origin parsed the loader origin's too); the service worker keeps a role's file as it
 * keeps the loader's (sw.ts keptLoaderFile, D308 §7).
 */
async function loaderPage(): Promise<JSX.Element | undefined> {
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
    const { startAtLoaderOrigin } = await loadRole(
      "loader's role at its own origin",
      async () => await import("./loader-at-loader-origin.tsx"),
      roleLoadParts,
    );
    return startAtLoaderOrigin({
      config,
      storage: localStore,
      location: window.location,
      referrer: document.referrer,
      now: () => Date.now(),
    });
  }
  const { AtClientOriginPage } = await loadRole(
    "loader's role at a client's origin",
    async () => await import("./loader-at-client-origin.tsx"),
    roleLoadParts,
  );
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
        showFailure: (failure) => {
          // The loader's root is down: the failure gets a root of its own.
          const shown = document.createElement("div");
          document.body.append(shown);
          createRoot(shown).render(<StrictMode>{failure}</StrictMode>);
        },
      }}
    />
  );
}

// The loader's pages run no input layer: OK on its sign-in's "remember my realm" submitted the form
// on the owner's Fire TV (2026-10-30). The client it runs later keeps its own guard as well.
guardFormsOk({ target: window, focused: () => document.activeElement }, noteForTheLog);
void loaderPage().then((page) => {
  if (page !== undefined) {
    root.render(<StrictMode>{page}</StrictMode>);
  }
});

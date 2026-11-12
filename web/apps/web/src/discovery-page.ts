// SPDX-License-Identifier: MPL-2.0

/**
 * What a discovery's page says of itself in its meta elements, read with nothing else loaded: the
 * shell's entry (main.tsx), app-update.ts and client-reaches.ts read it, and importing it from
 * discovery-start.ts brought the discovery's and WebRTC's SDK into the script every start parses
 * (the web client's architecture audit of 2026-11-10, Startup 5). Imports nothing.
 */

/** The meta element a discovery's page names its client API by; the core's own page has none. */
const clientApiMetaName = "orpheum-discovery-client-api";

/**
 * The client API of the discovery whose page this is, as the page names it, or "" on the core's own
 * origin: the server tells the client, which reads no environment (D204).
 */
export function discoveryClientApiOf(page: Pick<Document, "querySelector">): string {
  return page.querySelector(`meta[name="${clientApiMetaName}"]`)?.getAttribute("content") ?? "";
}

/**
 * The meta element a discovery's loader names its Google Cast receiver application by (D465 §5):
 * staging's, unpublished, or production's, published, each at its own loader's /cast/.
 */
const castReceiverAppIdMetaName = "orpheum-cast-receiver-app-id";

/**
 * The Cast receiver application of the loader that served this page (D465 §5), which the Cast menu's
 * Chromecast launches; undefined where the page names none: the core's own page, or a loader that
 * names no application yet.
 */
export function castReceiverAppIdOf(page: Pick<Document, "querySelector">): string | undefined {
  const appId = page
    .querySelector(`meta[name="${castReceiverAppIdMetaName}"]`)
    ?.getAttribute("content");
  return appId === undefined || appId === null || appId === "" ? undefined : appId;
}

/** The meta element a paid discovery's page says it has realms by (D298 §1). */
const realmsMetaName = "orpheum-discovery-realms";

/** Whether the discovery whose page this is has realms, so a person signs in by one there. */
export function discoveryHasRealms(page: Pick<Document, "querySelector">): boolean {
  return page.querySelector(`meta[name="${realmsMetaName}"]`)?.getAttribute("content") === "true";
}

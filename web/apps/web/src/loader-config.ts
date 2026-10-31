// SPDX-License-Identifier: MPL-2.0

/**
 * What a discovery's loader is told by the page the discovery serves it in (D308): the discovery's
 * client API, whether it has realms, and the pattern of its client origins, each in a meta element,
 * since the loader reads no environment (D204).
 */

import { discoveryClientApiOf, discoveryHasRealms } from "./discovery-start.ts";

/** The meta element a discovery names its client origins by: "https://{core}.client.example". */
const clientOriginsMetaName = "orpheum-client-origins";

/** The meta element a discovery names the loader's own origin by: "https://discovery.example". */
const loaderOriginMetaName = "orpheum-loader-origin";

/** The part of the pattern a core's id takes. */
const corePlace = "{core}";

/** What a core's id is: lowercase letters and digits, one DNS label. */
const coreIdPattern = /^[a-z0-9]{1,63}$/u;

/** What the loader is told by its page. */
export interface LoaderConfig {
  readonly clientApi: string;
  readonly hasRealms: boolean;
  /** Each core's client origin, its id in place of "{core}": "https://{core}.client.example". */
  readonly clientOrigins: string;
  /** The loader's own origin, where the household list is (D308 §8). */
  readonly loaderOrigin: string;
}

/** What page tells the loader, or undefined for a page that is no discovery's loader. */
export function loaderConfigOf(page: Pick<Document, "querySelector">): LoaderConfig | undefined {
  const clientApi = discoveryClientApiOf(page);
  const named = (name: string): string =>
    page.querySelector(`meta[name="${name}"]`)?.getAttribute("content") ?? "";
  const clientOrigins = named(clientOriginsMetaName);
  const loaderOrigin = named(loaderOriginMetaName);
  if (clientApi === "" || loaderOrigin === "" || clientOrigins.split(corePlace).length !== 2) {
    return undefined;
  }
  return { clientApi, hasRealms: discoveryHasRealms(page), clientOrigins, loaderOrigin };
}

/** The core whose client origin origin is, or undefined at an origin that is none's: the loader's. */
export function coreIdOfOrigin(origin: string, clientOrigins: string): string | undefined {
  const [before = "", after = ""] = clientOrigins.split(corePlace);
  if (!origin.startsWith(before) || !origin.endsWith(after)) {
    return undefined;
  }
  const coreId = origin.slice(before.length, origin.length - after.length);
  return coreIdPattern.test(coreId) ? coreId : undefined;
}

/**
 * The client origin of coreId, built from the pattern alone and never from a URL the page was given,
 * so no link sends the browser anywhere else (the audit's open redirect); undefined for an id that is
 * not one.
 */
export function clientOriginOf(coreId: string, clientOrigins: string): string | undefined {
  return coreIdPattern.test(coreId) ? clientOrigins.replace(corePlace, coreId) : undefined;
}

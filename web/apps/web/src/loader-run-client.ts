// SPDX-License-Identifier: MPL-2.0

/**
 * How the discovery's loader runs the core's own client once it reached the core (D308 §6): the
 * core's signed manifest of its client's files is checked by the key the link, the hand-off or this
 * origin's memory named, the client's version against the discovery's floor and its page against its
 * hash; the files are handed to the service worker, which answers none whose hash is not the
 * manifest's; and the client's own scripts and styles are added to this page, with the loader's
 * interface on the window for it to take.
 */

import {
  createApi,
  fromBase64,
  noteStep,
  verifyClientManifest,
  type FetchRequest,
} from "@orpheum/sdk";

import {
  manifestOf,
  sha256Of,
  type ClientFilesMessage,
  type ClientManifest,
} from "./client-files.ts";
import { loaderProperty, type OrpheumLoader } from "./loader-interface.ts";

/** Why the loader does not run the core's client. */
export class ClientRefused extends Error {
  readonly reason: "unsigned" | "refusedVersion" | "pageChanged";

  constructor(reason: ClientRefused["reason"], message: string) {
    super(message);
    this.reason = reason;
  }
}

/** One of the client page's styles or scripts: its element's tag and attributes. */
export interface ClientPart {
  readonly tag: "link" | "script";
  readonly attributes: readonly (readonly [string, string])[];
}

/** What running the client takes: the path to the core, the discovery's floor, and the page. */
export interface RunParts {
  readonly send: FetchRequest;
  readonly fingerprint: string;
  /** The client versions the discovery refuses, as it says; none when it says nothing. */
  readonly refusedVersions: () => Promise<readonly string[]>;
  /** A page to read the client's page in, inertly. */
  readonly inertPage: Document;
  /** Puts the loader on the window and adds the client's parts to the page, which runs them. */
  readonly mount: (loader: OrpheumLoader, parts: readonly ClientPart[]) => void;
  /** Tells the service worker the client's files, settling once it has kept them. */
  readonly tellWorker: (message: ClientFilesMessage) => Promise<void>;
}

/** The core's signed manifest, checked by the key whose fingerprint is held. */
export async function checkedManifest(
  send: FetchRequest,
  fingerprint: string,
): Promise<ClientManifest> {
  const api = createApi({ getToken: () => undefined, fetch: send });
  const { data, response } = await api.GET("/client-manifest");
  if (data === undefined) {
    throw new ClientRefused(
      "unsigned",
      `the server answered ${String(response.status)} for its client's manifest`,
    );
  }
  const manifest = fromBase64(data.manifest);
  const isSigned = await verifyClientManifest({
    publicKey: fromBase64(data.publicKey),
    fingerprint,
    manifest,
    signature: fromBase64(data.signature),
  });
  const parsed = isSigned ? manifestOf(new TextDecoder().decode(manifest)) : undefined;
  if (parsed === undefined) {
    throw new ClientRefused(
      "unsigned",
      "the server's client manifest is not signed by its own key",
    );
  }
  return parsed;
}

/** The client's page, fetched over the path and checked by the hash its manifest names. */
async function checkedPage(send: FetchRequest, manifest: ClientManifest): Promise<string> {
  const named = manifest.files.find((file) => file.path === "index.html");
  // The core answers its page at "/": Go's file server sends a request for /index.html there.
  const answer = await send(new Request("/"));
  const bytes = await answer.arrayBuffer();
  if (named === undefined || !answer.ok || (await sha256Of(bytes)) !== named.sha256) {
    throw new ClientRefused("pageChanged", "the server's client page is not the one it signed");
  }
  return new TextDecoder().decode(bytes);
}

/**
 * The client page's styles and scripts: only those its own files hold, under its assets, never an
 * inline script or one from elsewhere.
 */
export function clientPartsOf(page: Document, clientPage: string): readonly ClientPart[] {
  // A template's content is inert: nothing in it loads or runs while it is read.
  const inert = page.createElement("template");
  inert.innerHTML = clientPage;
  const parts: ClientPart[] = [];
  for (const part of inert.content.querySelectorAll("link[href], script[src]")) {
    const tag = part.tagName === "LINK" ? "link" : "script";
    const address = part.getAttribute(tag === "link" ? "href" : "src") ?? "";
    if (address.startsWith("/assets/")) {
      parts.push({
        tag,
        attributes: [...part.attributes].map(
          (attribute) => [attribute.name, attribute.value] as const,
        ),
      });
    }
  }
  return parts;
}

/** The query a client's script is run by, apart from the same address the loader ran. */
const clientRun = "client";

/**
 * Adds parts to page, each made anew so a script runs, with loader on its window for the client to
 * take: what RunParts.mount does in a browser. A script that does not load is told to onFailed by its
 * address, since the loader's page is down by then and the page would otherwise stay blank (the
 * owner's Mac, staging, 2026-10-29).
 */
export function mountClient(
  page: Document,
  run: {
    readonly loader: OrpheumLoader;
    readonly parts: readonly ClientPart[];
    readonly onFailed: (address: string) => void;
  },
): void {
  const { loader, parts, onFailed } = run;
  Reflect.set(page.defaultView ?? {}, loaderProperty, loader);
  for (const part of parts) {
    const made = page.createElement(part.tag);
    for (const [name, value] of part.attributes) {
      // A script by an address of its own: the loader and the client of one release are one build,
      // and a module the loader already ran is not run again by the same address. The worker finds
      // the file by its path alone.
      made.setAttribute(
        name,
        part.tag === "script" && name === "src" ? `${value}?${clientRun}` : value,
      );
    }
    if (part.tag === "script") {
      made.addEventListener(
        "error",
        () => {
          onFailed(made.getAttribute("src") ?? "");
        },
        { once: true },
      );
    }
    page.head.append(made);
  }
}

/**
 * Checks the core's client, its manifest, version and page, and hands its files to the service
 * worker; returns what runs it on this page under a loader. Throws ClientRefused, having run none
 * of it, when any check fails, so the loader's page still says why.
 */
export async function checkedClient(parts: RunParts): Promise<(loader: OrpheumLoader) => void> {
  noteStep("loading the core's client");
  const manifest = await checkedManifest(parts.send, parts.fingerprint);
  if ((await parts.refusedVersions()).includes(manifest.version)) {
    throw new ClientRefused(
      "refusedVersion",
      `the discovery refuses the server's client, ${manifest.version}`,
    );
  }
  const clientPage = await checkedPage(parts.send, manifest);
  await parts.tellWorker({ kind: "orpheum-client-files", files: manifest.files });
  const clientParts = clientPartsOf(parts.inertPage, clientPage);
  noteStep("the core's client is checked and ready to run");
  return (loader) => {
    parts.mount(loader, clientParts);
  };
}

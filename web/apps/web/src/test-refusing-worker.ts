// SPDX-License-Identifier: MPL-2.0

/**
 * A service worker for part-load.browser.test.ts: an origin across a deploy, which refuses a role's
 * module the first times it is asked for, whatever its query, and serves it after. The count of
 * refusals is its own address's query (`?refusals=N`). It answers its pages' own loads with a page
 * that hands its window an import, so a test imports modules in a document of their own, under the
 * worker. Registered by that test alone, as a module; never built into the app.
 */

/** The path of the role's module the worker refuses, then serves. */
export const refusedRolePath = "/refused-role.js";

/** A load the worker is asked to answer. */
interface RefusingFetchEvent {
  readonly request: Request;
  respondWith(response: Promise<Response> | Response): void;
}

/** An install or activate, which the worker can hold open. */
interface RefusingExtendableEvent {
  waitUntil(work: Promise<unknown>): void;
}

/** The worker's scope, as far as this one uses it. */
interface RefusingWorkerScope {
  readonly location: { readonly search: string };
  readonly clients: { claim(): Promise<void> };
  skipWaiting(): Promise<void>;
  addEventListener(type: "fetch", listener: (event: RefusingFetchEvent) => void): void;
  addEventListener(
    type: "install" | "activate",
    listener: (event: RefusingExtendableEvent) => void,
  ): void;
}

function isRefusingWorkerScope(scope: unknown): scope is RefusingWorkerScope {
  return (
    typeof scope === "object" && scope !== null && "clients" in scope && "skipWaiting" in scope
  );
}

/** Each page's own load: a page whose window imports a module at an address by importAt. */
const pageHtml =
  "<!doctype html><title>refusing</title>" +
  "<script type=module>window.importAt = (address) => import(address);</script>";

/** The role's module, once it comes: it exports the role's function, as the real one does. */
const roleModule = 'export function startTheRole() { return "the role"; }';

const scope: unknown = globalThis;
if (isRefusingWorkerScope(scope)) {
  let refusals = Number(new URLSearchParams(scope.location.search).get("refusals") ?? "0");
  scope.addEventListener("install", (event) => {
    event.waitUntil(scope.skipWaiting());
  });
  scope.addEventListener("activate", (event) => {
    event.waitUntil(scope.clients.claim());
  });
  scope.addEventListener("fetch", (event) => {
    if (event.request.mode === "navigate") {
      event.respondWith(new Response(pageHtml, { headers: { "Content-Type": "text/html" } }));
      return;
    }
    if (new URL(event.request.url).pathname !== refusedRolePath) {
      return;
    }
    if (refusals > 0) {
      refusals -= 1;
      event.respondWith(new Response("gone", { status: 404 }));
      return;
    }
    event.respondWith(new Response(roleModule, { headers: { "Content-Type": "text/javascript" } }));
  });
}

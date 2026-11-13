// SPDX-License-Identifier: MPL-2.0

import { base64url, clientManifestMessage, type FetchRequest } from "@orpheum/sdk";

import { sha256Of } from "./client-files.ts";

/**
 * A core's client page for the loader's tests: its own style and script under its assets, and a
 * script from elsewhere and an inline one, which the loader runs neither of.
 */
export const clientPage =
  "<!doctype html><html><head>" +
  '<link rel="stylesheet" href="/assets/index-a.css">' +
  '<script type="module" crossorigin src="/assets/index-b.js"></script>' +
  '<script src="https://elsewhere.example/x.js"></script>' +
  "<script>alert(1)</script>" +
  '</head><body><div id="root"></div></body></html>';

/**
 * A core with its own key, which signs the manifest of a client whose page is clientPage and serves
 * page at "/": a page other than clientPage is one changed since it was signed.
 */
export async function coreServing(page: string): Promise<{
  readonly send: FetchRequest;
  readonly fingerprint: string;
}> {
  const keys = await crypto.subtle.generateKey("Ed25519", true, ["sign", "verify"]);
  const publicKey = new Uint8Array(await crypto.subtle.exportKey("raw", keys.publicKey));
  const pageBytes = new TextEncoder().encode(clientPage);
  const manifest = new TextEncoder().encode(
    JSON.stringify({
      version: "v0.2.0-nas.9",
      script: "index-b.js",
      files: [
        { path: "index.html", size: pageBytes.length, sha256: await sha256Of(pageBytes.buffer) },
      ],
    }),
  );
  const signature = new Uint8Array(
    await crypto.subtle.sign("Ed25519", keys.privateKey, clientManifestMessage(manifest)),
  );
  const send: FetchRequest = (request) => {
    const path = new URL(request.url).pathname;
    if (path === "/api/v1/client-manifest") {
      return Promise.resolve(
        Response.json({
          manifest: base64url(manifest),
          signature: base64url(signature),
          publicKey: base64url(publicKey),
        }),
      );
    }
    return Promise.resolve(path === "/" ? new Response(page) : new Response("", { status: 404 }));
  };
  const fingerprint = base64url(new Uint8Array(await crypto.subtle.digest("SHA-256", publicKey)));
  return { send, fingerprint };
}

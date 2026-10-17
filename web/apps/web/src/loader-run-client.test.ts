// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { base64url, clientManifestMessage, type FetchRequest } from "@orpheum/sdk";
import { describe, expect, it } from "vitest";

import type { ClientFilesMessage } from "./client-files.ts";
import { sha256Of } from "./client-files.ts";
import type { OrpheumLoader } from "./loader-interface.ts";
import {
  checkedClient,
  ClientRefused,
  type ClientPart,
  type RunParts,
} from "./loader-run-client.ts";

const clientPage =
  "<!doctype html><html><head>" +
  '<link rel="stylesheet" href="/assets/index-a.css">' +
  '<script type="module" crossorigin src="/assets/index-b.js"></script>' +
  '<script src="https://elsewhere.example/x.js"></script>' +
  "<script>alert(1)</script>" +
  '</head><body><div id="root"></div></body></html>';

/** A core with its own key, which signs the manifest of a client whose page is page. */
async function coreServing(page: string): Promise<{
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
    return Promise.resolve(
      path === "/index.html" ? new Response(page) : new Response("", { status: 404 }),
    );
  };
  const fingerprint = base64url(new Uint8Array(await crypto.subtle.digest("SHA-256", publicKey)));
  return { send, fingerprint };
}

/** The parts of a run against core, the worker's word and the parts mounted heard. */
function partsOf(
  core: { readonly send: FetchRequest; readonly fingerprint: string },
  heard: { readonly told: ClientFilesMessage[]; readonly mounted: (readonly ClientPart[])[] },
  refused: readonly string[] = [],
): RunParts {
  return {
    ...core,
    refusedVersions: () => Promise.resolve(refused),
    inertPage: document,
    mount: (_loader, parts) => {
      heard.mounted.push(parts);
    },
    tellWorker: (message) => {
      heard.told.push(message);
      return Promise.resolve();
    },
  };
}

/** What a loader hands the client, as far as the run reads it. */
const loader = { major: 1 } as const satisfies Partial<OrpheumLoader>;

describe("the core's client, as the loader runs it (D308 §6)", () => {
  it("runs only its own styles and scripts, once its manifest holds under the held key", async () => {
    const heard = { told: [] as ClientFilesMessage[], mounted: [] as (readonly ClientPart[])[] };
    const run = await checkedClient(partsOf(await coreServing(clientPage), heard));
    expect(heard.told.map((message) => message.files.map((file) => file.path))).toEqual([
      ["index.html"],
    ]);
    expect(heard.mounted).toEqual([]);
    run(loader as unknown as OrpheumLoader);
    expect(heard.mounted).toEqual([
      [
        {
          tag: "link",
          attributes: [
            ["rel", "stylesheet"],
            ["href", "/assets/index-a.css"],
          ],
        },
        {
          tag: "script",
          attributes: [
            ["type", "module"],
            ["crossorigin", ""],
            ["src", "/assets/index-b.js"],
          ],
        },
      ],
    ]);
  });

  it("runs none of it under another key, with another page, or at a version the discovery refuses", async () => {
    const heard = { told: [] as ClientFilesMessage[], mounted: [] as (readonly ClientPart[])[] };
    const served = await coreServing(clientPage);
    const refusals: string[] = [];
    for (const parts of [
      partsOf({ ...served, fingerprint: "another-core's-fingerprint" }, heard),
      partsOf(await coreServing(clientPage.replace("index-b", "index-evil")), heard),
      partsOf(served, heard, ["v0.2.0-nas.9"]),
    ]) {
      try {
        await checkedClient(parts);
        refusals.push("ran");
      } catch (problem: unknown) {
        refusals.push(problem instanceof ClientRefused ? problem.reason : "failed");
      }
    }
    expect(refusals).toEqual(["unsigned", "pageChanged", "refusedVersion"]);
    expect(heard.told).toEqual([]);
  });
});

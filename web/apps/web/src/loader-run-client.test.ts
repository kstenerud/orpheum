// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import { forgetSteps, stepsSoFar, type FetchRequest } from "@orpheum/sdk";
import { describe, expect, it } from "vitest";

import type { ClientFilesMessage } from "./client-files.ts";
import type { OrpheumLoader } from "./loader-interface.ts";
import {
  checkedClient,
  ClientRefused,
  mountClient,
  type ClientPart,
  type RunParts,
} from "./loader-run-client.ts";
import { clientPage, coreServing } from "./test-signed-core.ts";

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
    forgetSteps();
    const run = await checkedClient(partsOf(await coreServing(clientPage), heard));
    // Told in what happened, should the page fail after.
    expect(stepsSoFar().map((step) => step.replace(/^\d+ ms /, ""))).toEqual([
      "loading the core's client",
      "the core's client is checked and ready to run",
    ]);
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

  it("tells a script of the client's that did not load by its address, never leaving the page blank (the owner's Mac, 2026-10-29)", async () => {
    const failed: string[] = [];
    const loader = { major: 1 } as unknown as OrpheumLoader;
    mountClient(document, {
      loader,
      parts: [
        { tag: "link", attributes: [["href", "/assets/index-a.css"]] },
        { tag: "script", attributes: [["src", "/assets/index-b.js"]] },
      ],
      onFailed: (address) => {
        failed.push(address);
      },
    });
    document.head
      .querySelector('link[href="/assets/index-a.css"]')
      ?.dispatchEvent(new Event("error"));
    // happy-dom loads no script, and says so by the script's error, as a browser does for a 404.
    await new Promise((resolve) => {
      setTimeout(resolve, 0);
    });
    expect(failed).toEqual(["/assets/index-b.js?client"]);
  });
});

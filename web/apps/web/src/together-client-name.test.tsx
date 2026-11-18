// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { JSX } from "react";
import { afterEach, describe, expect, it } from "vitest";

import { mount, servedBy } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";
import { useVoice } from "./together-voice.tsx";
import { useTogether } from "./use-together.ts";

/**
 * A page a discovery's loader runs the core's client on (D308): the discovery's meta, the loader's
 * own script, and the core's client the loader added by its "?client" address.
 */
function pageUnderALoader(): void {
  document.head.innerHTML =
    '<meta name="orpheum-discovery-client-api" content="https://api.example/client/v1">' +
    '<script type="module" crossorigin src="/assets/index-L0ader1.js"></script>' +
    '<script type="module" crossorigin src="/assets/index-C0re22.js?client"></script>';
}

/** A device's connection to its session, as a guest's page has it. */
function Device(): JSX.Element {
  useTogether({ credential: "viewer-credential" });
  return <p>device</p>;
}

/** A person's voice, as a page has it. */
function Voice(): JSX.Element {
  useVoice({ sessionId: 1, credential: "viewer-credential" }, undefined);
  return <p>voice</p>;
}

afterEach(() => {
  document.head.innerHTML = "";
});

describe("the client a Watch Together device names in its hello, for the core's log (agent-failures A28)", () => {
  it("is the core's client the loader runs, on a discovery's page: a guest's AirPlay grant (VQ364) was tied to no build, the log saying client=\"\"", async () => {
    pageUnderALoader();
    const server = fakeTogetherServer();
    const mounted = await mount(
      withServices(<Device />, { ...servedBy({}), openTogetherSocket: server.openSocket }),
    );
    await server.accept();
    expect(server.heard[0]).toMatchObject({ type: "hello", client: "index-C0re22.js" });
    mounted.unmount();
  });

  it("is the core's client the loader runs for the voice's connection too", async () => {
    pageUnderALoader();
    const server = fakeTogetherServer();
    const mounted = await mount(
      withServices(<Voice />, { ...servedBy({}), openTogetherSocket: server.openSocket }),
    );
    await server.accept();
    expect(server.heard[0]).toMatchObject({ type: "hello", client: "index-C0re22.js" });
    mounted.unmount();
  });
});

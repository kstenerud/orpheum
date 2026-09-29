// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, describe, expect, it } from "vitest";

import { Identify } from "./identify.tsx";
import { Libraries } from "./libraries.tsx";
import { Settings } from "./settings.tsx";
import {
  button,
  click,
  mount,
  refusal,
  servedBy,
  submit,
  unreachable,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

const library = {
  id: 3,
  name: "Films",
  kind: "movies",
  metadataSource: "nfo_then_tmdb",
  changeDetection: "manual",
  isWritable: false,
  createdAt: 1,
  filings: {},
  roots: [
    {
      nodeName: "local",
      path: "/media/films",
      lastScanOutcome: "withheld",
      withheldReason: "gone",
    },
  ],
};

const waitingItem = {
  id: 7,
  kind: "movie",
  name: "Variety",
  identification: "ambiguous",
  libraryId: 1,
};

const secret = { name: "tmdb_api_key", description: "TMDB", isSet: true, needsReentry: false };

const setting = {
  key: "scan_on_node_hello",
  kind: "bool",
  description: "scan on hello",
  value: true,
  startingValue: false,
  isModified: true,
  isExposed: false,
  setAt: 1,
};

/** What the viewer is told for each refusal these tests send, and never the server's note. */
const conflict = "That clashes with something that already exists.";
const notFound = "That is no longer here: it may have been removed. Go back and try again.";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** Expects the screen to say text, and nothing of the server's note for developers. */
function expectSays(text: string): void {
  const shown = mounted?.container.textContent ?? "";
  expect(shown).toContain(text);
  expect(shown).not.toContain("a note for developers");
  expect(shown).not.toContain("Something went wrong");
}

describe("a screen the server refuses tells the viewer in their words (cut-a §5.2)", () => {
  it("creating a library", async () => {
    const services = servedBy({
      "GET /libraries": [200, { libraries: [] }],
      "GET /nodes": [200, { nodes: [] }],
      "POST /libraries": refusal(409, "conflict"),
    });
    mounted = await mount(withServices(<Libraries />, services));
    await submit(mounted.container.querySelector("form"));
    expectSays(conflict);
  });

  it("scanning a library, and answering its removals", async () => {
    const services = servedBy({
      "GET /libraries": [200, { libraries: [library] }],
      "GET /nodes": [200, { nodes: [] }],
      "POST /libraries/3/scan": refusal(404, "not_found"),
      "POST /libraries/3/removals": refusal(409, "conflict"),
    });
    mounted = await mount(withServices(<Libraries />, services));
    await click(button(mounted.container, "Scan now"));
    expectSays(notFound);
    await click(button(mounted.container, "Remove what the last scan did not find"));
    expectSays(conflict);
  });

  it("looking a title up again, and choosing its match", async () => {
    const services = servedBy({
      "GET /items": [200, { items: [waitingItem] }],
      "POST /items/7/refresh": refusal(404, "not_found"),
      "GET /items/7/candidates": [
        200,
        { provider: "tmdb", candidates: [{ providerId: "101", name: "Variety" }] },
      ],
      "POST /items/7/match": refusal(409, "conflict"),
    });
    mounted = await mount(withServices(<Identify />, services));
    await click(button(mounted.container, "Look again"));
    expectSays(notFound);
    await click(button(mounted.container, "Choose its match"));
    await click(button(mounted.container, "This one"));
    expectSays(conflict);
  });

  it("saving a secret, and resetting a setting", async () => {
    const services = servedBy({
      "GET /secrets": [200, { secrets: [secret] }],
      "GET /config": [200, { settings: [setting] }],
      "PUT /secrets/tmdb_api_key": refusal(409, "conflict"),
      "POST /config/scan_on_node_hello/reset": refusal(404, "not_found"),
    });
    mounted = await mount(withServices(<Settings />, services));
    await submit(mounted.container.querySelector('form[aria-labelledby="secret-tmdb_api_key"]'));
    expectSays(conflict);
    await click(button(mounted.container, "Reset to the starting value"));
    expectSays(notFound);
  });
});

describe("a request that never reaches the server says so, rather than nothing", () => {
  const cannotReach = "The server could not be reached. Check the connection and try again.";

  it("scanning a library", async () => {
    const services = servedBy({
      "GET /libraries": [200, { libraries: [library] }],
      "GET /nodes": [200, { nodes: [] }],
      "POST /libraries/3/scan": unreachable,
    });
    mounted = await mount(withServices(<Libraries />, services));
    await click(button(mounted.container, "Scan now"));
    expectSays(cannotReach);
  });

  it("creating a library", async () => {
    const services = servedBy({
      "GET /libraries": [200, { libraries: [] }],
      "GET /nodes": [200, { nodes: [] }],
      "POST /libraries": unreachable,
    });
    mounted = await mount(withServices(<Libraries />, services));
    await submit(mounted.container.querySelector("form"));
    expectSays(cannotReach);
  });

  it("looking a title up again", async () => {
    const services = servedBy({
      "GET /items": [200, { items: [waitingItem] }],
      "POST /items/7/refresh": unreachable,
    });
    mounted = await mount(withServices(<Identify />, services));
    await click(button(mounted.container, "Look again"));
    expectSays(cannotReach);
  });

  it("resetting a setting", async () => {
    const services = servedBy({
      "GET /secrets": [200, { secrets: [secret] }],
      "GET /config": [200, { settings: [setting] }],
      "POST /config/scan_on_node_hello/reset": unreachable,
    });
    mounted = await mount(withServices(<Settings />, services));
    await click(button(mounted.container, "Reset to the starting value"));
    expectSays(cannotReach);
  });
});

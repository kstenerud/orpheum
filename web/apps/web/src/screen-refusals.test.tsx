// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { components } from "@orpheum/sdk";
import { afterEach, describe, expect, it } from "vitest";

import { Identify } from "./identify.tsx";
import { CreateLibraryPage } from "./create-library.tsx";
import { LibraryPage } from "./library-manage.tsx";
import { Settings } from "./settings.tsx";
import {
  buttonWithText,
  click,
  enter,
  mount,
  refusal,
  servedBy,
  submit,
  unreachable,
  type Answer,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

const library: components["schemas"]["LibraryBody"] = {
  id: 3,
  name: "Films",
  kind: "film",
  metadataSource: "nfoThenTmdb",
  changeDetection: "manual",
  isWritable: false,
  createdAt: 1,
  fileCountsByFiling: {},
  roots: [
    {
      nodeName: "local",
      rootPath: "/media/films",
      lastScanOutcome: "withheld",
      withheldReason: "gone",
      missingMountPaths: [],
    },
  ],
};

const waitingItem: components["schemas"]["TitleEntryBody"] = {
  id: 7,
  kind: "film",
  name: "Théo",
  identificationState: "ambiguous",
};

const secret: components["schemas"]["SecretBody"] = {
  name: "tmdbApiKey",
  description: "TMDB",
  isSet: true,
  needsReentry: false,
};

const setting = {
  key: "shouldScanOnNodeHello",
  kind: "bool",
  description: "scan on hello",
  value: true,
  startingValue: false,
  isModified: true,
  isImportant: true,
  section: "libraries",
  setAt: 1,
};

/** The hours a new library checked on an interval starts at, which the create form reads. */
const libraryCheck = {
  key: "newLibraryDefaultScanEveryHours",
  kind: "integer",
  description: "hours between checks",
  value: 24,
  startingValue: 24,
  isModified: false,
  isImportant: true,
  section: "libraries",
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
      "GET /config": [200, { settings: [libraryCheck] }],
      "POST /libraries": refusal(409, "conflict"),
    });
    mounted = await mount(withServices(<CreateLibraryPage />, services));
    await submit(mounted.container.querySelector("form"));
    expectSays(conflict);
  });

  it("scanning a library, and answering its removals", async () => {
    const services = servedBy({
      "GET /libraries": [200, { libraries: [library] }],
      "GET /nodes": [200, { nodes: [] }],
      "GET /config": [200, { settings: [libraryCheck] }],
      "POST /libraries/3/scan": refusal(404, "notFound"),
      "POST /libraries/3/roots/removals": refusal(409, "conflict"),
    });
    mounted = await mount(withServices(<LibraryPage libraryId={3} />, services));
    await click(buttonWithText(mounted.container, "Scan now"));
    expectSays(notFound);
    await click(buttonWithText(mounted.container, "Remove what the last scan did not find"));
    expectSays(conflict);
  });

  it("looking a title up again, and choosing its match", async () => {
    const services = servedBy({
      "GET /items": [200, { items: [waitingItem] }],
      "POST /items/7/refresh": refusal(404, "notFound"),
      "GET /items/7/candidates": [
        200,
        { provider: "tmdb", candidates: [{ providerId: "101", name: "Théo" }] },
      ],
      "POST /items/7/match": refusal(409, "conflict"),
    });
    mounted = await mount(withServices(<Identify />, services));
    await click(buttonWithText(mounted.container, "Look again"));
    expectSays(notFound);
    await click(buttonWithText(mounted.container, "Choose its match"));
    await click(buttonWithText(mounted.container, "This one"));
    expectSays(conflict);
  });

  it("saving a secret, and resetting a setting", async () => {
    const services = servedBy({
      "GET /secrets": [200, { secrets: [secret] }],
      "GET /system/update": [200, { version: "v0.3.1", checkedAt: 0 }],
      "GET /config": [200, { settings: [setting] }],
      "PUT /secrets/tmdbApiKey": refusal(409, "conflict"),
      "POST /config/shouldScanOnNodeHello/reset": refusal(404, "notFound"),
    });
    mounted = await mount(withServices(<Settings />, services));
    const secretForm = 'form[aria-labelledby="secret-tmdbApiKey"]';
    await enter(mounted.container.querySelector(`${secretForm} input`), "a new key");
    await submit(mounted.container.querySelector(secretForm));
    expectSays(conflict);
    await click(
      mounted.container.querySelector('button[aria-label^="Return shouldScanOnNodeHello"]'),
    );
    expectSays(notFound);
  });
});

describe("a refused setting is named beside it, in the form's words", () => {
  it("saving a setting's value the server does not accept", async () => {
    const services = servedBy({
      "GET /secrets": [200, { secrets: [secret] }],
      "GET /system/update": [200, { version: "v0.3.1", checkedAt: 0 }],
      "GET /config": [200, { settings: [setting, libraryCheck] }],
      "PUT /config": [
        422,
        {
          error: {
            code: "invalidRequest",
            message: "a note for developers",
            detail: {
              problems: [
                {
                  location: "body.valuesByKey.newLibraryDefaultScanEveryHours",
                  message: "too big",
                },
              ],
            },
          },
        },
      ],
    });
    mounted = await mount(withServices(<Settings />, services));
    const form = 'form[aria-labelledby="setting-newLibraryDefaultScanEveryHours"]';
    await enter(mounted.container.querySelector(`${form} input`), "100000");
    await submit(mounted.container.querySelector(form));
    expect(mounted.container.querySelector(`${form} [role="alert"]`)?.textContent).toBe(
      "The server did not accept what was entered in newLibraryDefaultScanEveryHours. Correct what is there and try again.",
    );
    const other = 'form[aria-labelledby="setting-shouldScanOnNodeHello"] [role="alert"]';
    expect(mounted.container.querySelector(other)).toBeNull();
    expectSays("newLibraryDefaultScanEveryHours. Correct what is there");
  });
});

describe("a request that never reaches the server says so, rather than nothing", () => {
  const cannotReach = "The server could not be reached. Check the connection and try again.";

  it("scanning a library", async () => {
    const services = servedBy({
      "GET /libraries": [200, { libraries: [library] }],
      "GET /nodes": [200, { nodes: [] }],
      "GET /config": [200, { settings: [libraryCheck] }],
      "POST /libraries/3/scan": unreachable,
    });
    mounted = await mount(withServices(<LibraryPage libraryId={3} />, services));
    await click(buttonWithText(mounted.container, "Scan now"));
    expectSays(cannotReach);
  });

  it("creating a library", async () => {
    const services = servedBy({
      "GET /libraries": [200, { libraries: [] }],
      "GET /nodes": [200, { nodes: [] }],
      "GET /config": [200, { settings: [libraryCheck] }],
      "POST /libraries": unreachable,
    });
    mounted = await mount(withServices(<CreateLibraryPage />, services));
    await submit(mounted.container.querySelector("form"));
    expectSays(cannotReach);
  });

  it("looking a title up again", async () => {
    const services = servedBy({
      "GET /items": [200, { items: [waitingItem] }],
      "POST /items/7/refresh": unreachable,
    });
    mounted = await mount(withServices(<Identify />, services));
    await click(buttonWithText(mounted.container, "Look again"));
    expectSays(cannotReach);
  });

  it("resetting a setting", async () => {
    const services = servedBy({
      "GET /secrets": [200, { secrets: [secret] }],
      "GET /system/update": [200, { version: "v0.3.1", checkedAt: 0 }],
      "GET /config": [200, { settings: [setting] }],
      "POST /config/shouldScanOnNodeHello/reset": unreachable,
    });
    mounted = await mount(withServices(<Settings />, services));
    await click(
      mounted.container.querySelector('button[aria-label^="Return shouldScanOnNodeHello"]'),
    );
    expectSays(cannotReach);
  });
});

describe("a match TMDB refuses says why and who can fix it", () => {
  /** TMDB's refusal as the server gives it: providerError, why in detail.reason. */
  function providerRefusal(reason: string): Answer {
    return [
      503,
      { error: { code: "providerError", message: "a note for developers", detail: { reason } } },
    ];
  }

  it("looking up the candidates with no key set", async () => {
    const services = servedBy({
      "GET /items": [200, { items: [waitingItem] }],
      "GET /items/7/candidates": providerRefusal("noKey"),
    });
    mounted = await mount(withServices(<Identify />, services));
    await click(buttonWithText(mounted.container, "Choose its match"));
    expectSays("No TMDB key is set up. An admin can set it in Settings.");
  });

  it("choosing a match with a refused key, and looking again while TMDB is not answering", async () => {
    const services = servedBy({
      "GET /items": [200, { items: [waitingItem] }],
      "GET /items/7/candidates": [
        200,
        { provider: "tmdb", candidates: [{ providerId: "101", name: "Théo" }] },
      ],
      "POST /items/7/match": providerRefusal("keyRefused"),
      "POST /items/7/refresh": providerRefusal("unavailable"),
    });
    mounted = await mount(withServices(<Identify />, services));
    await click(buttonWithText(mounted.container, "Choose its match"));
    await click(buttonWithText(mounted.container, "This one"));
    expectSays("TMDB refused its key. An admin can change it in Settings.");
    await click(buttonWithText(mounted.container, "Look again"));
    expectSays("TMDB is not answering right now. Try again later.");
  });
});

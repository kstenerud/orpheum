// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { createNavigation } from "./navigation.ts";
import type { Services } from "./services.tsx";
import { SystemDiscovery } from "./system-discovery.tsx";
import {
  buttonWithText,
  click,
  enter,
  mount,
  refusal,
  servedBy,
  type Answer,
  type Answers,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { fakePlace, withServices, type FakePlace } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** The discovery the core suggests, as its status names it (D391 §7). */
const suggestedIssuer = "https://discovery.example";

/** A core connected to no discovery. */
const none = {
  issuer: "",
  suggestedIssuer,
  rootKeys: [],
  rootVersion: 0,
  isSuggested: false,
  documentAt: 0,
  fingerprint: "",
  published: [],
  probe: [],
  claimExpiresAt: 0,
  removedAt: 0,
  lastHeartbeatAt: 0,
  deadEndsLastWeek: 0,
  portMapping: { isWanted: false, checkedAt: 0, mappings: [], problems: [] },
};

/** A core enrolled with the suggested discovery, waiting to be linked to an account. */
const pending = {
  ...none,
  issuer: suggestedIssuer,
  isSuggested: true,
  coreId: "k7xq2",
  state: "pending",
  claimCode: "WXYZ-2345",
  claimExpiresAt: 1_790_000_000_000,
};

/** The account page the core answers Connect with. */
const linkUrl = "https://account.discovery.example/link?token=t0k3n";

/** What a press of Connect leaves for: the account page, as the server answers it. */
const linked: Answer = [200, { linkUrl }];

/** Services answered by answers, at a browser's address path, whose leaving the place keeps. */
function servedAt(
  path: string,
  answers: Answers,
  heard: Heard[] = [],
): { readonly place: FakePlace; readonly services: Services } {
  const place = fakePlace(path);
  return {
    place,
    services: { ...servedBy(answers, heard), navigation: createNavigation(place, false) },
  };
}

/** A refusal of what was sent at location, as the core's invalidRequestAt makes it. */
function refusedAt(location: string): Answer {
  return [
    422,
    {
      error: {
        code: "invalidRequest",
        message: "a note for developers",
        detail: { problems: [{ location, message: "a note for developers" }] },
      },
    },
  ];
}

/** The Connect section, by its heading. */
const connect = '[aria-labelledby="system-discovery-connect"]';

/** What the page says went wrong, in its alert. */
function alertText(container: HTMLElement): string | null | undefined {
  return container.querySelector('[role="alert"]')?.textContent;
}

it("connects by one press: the suggested discovery prefilled, and the browser sent to the account page the server answers (D527 §2.1)", async () => {
  const heard: Heard[] = [];
  const { place, services } = servedAt(
    "/system",
    { "GET /system/discovery": [200, none], "POST /system/discovery/link": linked },
    heard,
  );
  mounted = await mount(withServices(<SystemDiscovery />, services));
  const container = mounted.container;
  expect(container.textContent).toContain("Connect to a discovery service");
  expect(container.textContent).toContain("Not linked to an account yet.");
  expect(container.querySelector<HTMLInputElement>(`${connect} input`)?.value).toBe(
    "discovery.example",
  );
  // No password is asked before the server asks for it (D527 §2.2).
  expect(container.querySelector(`${connect} input[type="password"]`)).toBeNull();
  await click(buttonWithText(container, "Connect"));
  expect(heard.map((each) => each.methodAndPath)).toEqual([
    "GET /system/discovery",
    "POST /system/discovery/link",
  ]);
  expect(heard[1]?.body).toEqual({ issuer: "discovery.example" });
  expect(place.left).toEqual([linkUrl]);
});

it("puts Connect under the Network page's one heading, now that it is a page of its own (D528 §1)", async () => {
  const { services } = servedAt("/network", { "GET /system/discovery": [200, none] });
  mounted = await mount(withServices(<SystemDiscovery />, services));
  const headings = [...mounted.container.querySelectorAll("h1, h2")].map(
    (heading) => `${heading.tagName}: ${heading.textContent}`,
  );
  expect(headings).toEqual(["H1: Network", "H2: Connect to a discovery service"]);
});

it("connects a server already waiting at its discovery without naming one, and offers no Connect once it is linked, naming the account", async () => {
  const heard: Heard[] = [];
  const { place, services } = servedAt(
    "/system",
    { "GET /system/discovery": [200, pending], "POST /system/discovery/link": linked },
    heard,
  );
  mounted = await mount(withServices(<SystemDiscovery />, services));
  const container = mounted.container;
  expect(container.textContent).toContain(
    `Connected to the Orpheum service, at ${suggestedIssuer}.`,
  );
  expect(container.textContent).toContain("Waiting to be claimed.");
  await click(buttonWithText(container, "Connect"));
  expect(heard[1]?.body).toEqual({});
  expect(place.left).toEqual([linkUrl]);
  mounted.unmount();
  const claimed = {
    ...pending,
    state: "claimed",
    claimCode: undefined,
    account: "karl@example.com",
  };
  mounted = await mount(
    withServices(<SystemDiscovery />, servedBy({ "GET /system/discovery": [200, claimed] })),
  );
  expect(mounted.container.textContent).toContain("Claimed: clients can find this server.");
  expect(mounted.container.textContent).toContain("Enrolled with the account karl@example.com.");
  expect(buttonWithText(mounted.container, "Connect")).toBeUndefined();
  expect(mounted.container.querySelector("details")).toBeNull();
});

it("asks the admin's password only when the server asks for it, and says when it isn't theirs (D527 §2.2)", async () => {
  const heard: Heard[] = [];
  const { place, services } = servedAt(
    "/system",
    {
      "GET /system/discovery": [200, pending],
      "POST /system/discovery/link": [
        refusedAt("body.password"),
        refusedAt("body.password"),
        linked,
      ],
    },
    heard,
  );
  mounted = await mount(withServices(<SystemDiscovery />, services));
  const container = mounted.container;
  await click(buttonWithText(container, "Connect"));
  expect(alertText(container)).toBe(
    "Your password on this server is needed once, so you can sign in from away. Enter it and press Connect again.",
  );
  const password = container.querySelector(`${connect} input[type="password"]`);
  expect(password).not.toBeNull();
  await enter(password, "a guess");
  await click(buttonWithText(container, "Connect"));
  expect(alertText(container)).toBe("That isn't your password. Nothing was linked.");
  await enter(password, "owner-password");
  await click(buttonWithText(container, "Connect"));
  const links = heard.filter((each) => each.methodAndPath === "POST /system/discovery/link");
  expect(links.map((each) => each.body)).toEqual([
    {},
    { password: "a guess" },
    { password: "owner-password" },
  ]);
  expect(place.left).toEqual([linkUrl]);
});

it("says each of Connect's refusals in its own words, and goes nowhere (D527 §4)", async () => {
  const cases: readonly (readonly [Answer, string])[] = [
    [
      refusedAt("header.Origin"),
      "Your browser didn't say which page to come back to. Open this server's page in a browser and press Connect there.",
    ],
    [
      refusedAt("body.issuer"),
      "This version of the server doesn't know the keys of discovery.example yet, so it can't trust it. Update the server, or connect to another discovery.",
    ],
    [refusal(403, "forbidden"), "Only an admin of this server can link it."],
    [
      refusal(409, "conflict"),
      "The server stopped being connected to the discovery while Connect was under way. Press Connect again.",
    ],
    [
      [
        502,
        {
          error: {
            code: "providerError",
            message: "a note for developers",
            failedHop: "provider",
            detail: { reason: "unavailable" },
          },
        },
      ],
      "The discovery didn't answer, so nothing was linked. Try again in a few minutes.",
    ],
    [
      refusedAt("body"),
      "The server couldn't start the link. Press Connect again; if this comes back, the server's log says why.",
    ],
  ];
  for (const [answer, words] of cases) {
    const { place, services } = servedAt("/system", {
      "GET /system/discovery": [200, none],
      "POST /system/discovery/link": answer,
    });
    mounted = await mount(withServices(<SystemDiscovery />, services));
    await click(buttonWithText(mounted.container, "Connect"));
    expect(alertText(mounted.container)).toBe(words);
    expect(place.left).toEqual([]);
    mounted.unmount();
    mounted = undefined;
  }
});

it("connects to another discovery once its identity is confirmed, and says when it is not the one confirmed (D393 §4)", async () => {
  const heard: Heard[] = [];
  const { place, services } = servedAt(
    "/system",
    {
      "GET /system/discovery": [200, none],
      "POST /system/discovery/identify": [
        200,
        { issuer: "https://home.example", identity: "od1-home", name: "Home", isSuggested: false },
      ],
      "POST /system/discovery/link": [refusedAt("body.identity"), linked],
    },
    heard,
  );
  mounted = await mount(withServices(<SystemDiscovery />, services));
  const container = mounted.container;
  await enter(container.querySelector(`${connect} input`), "home.example");
  await click(buttonWithText(container, "Connect"));
  expect(container.textContent).toContain("Home, at home.example, shows this identity.");
  await click(buttonWithText(container, "They are the same: connect"));
  expect(alertText(container)).toBe(
    "That discovery's identity is not the one confirmed. Nothing was connected; ask its operator.",
  );
  await click(buttonWithText(container, "They are the same: connect"));
  const links = heard.filter((each) => each.methodAndPath === "POST /system/discovery/link");
  expect(links.map((each) => each.body)).toEqual([
    { issuer: "home.example", identity: "od1-home" },
    { issuer: "home.example", identity: "od1-home" },
  ]);
  expect(place.left).toEqual([linkUrl]);
});

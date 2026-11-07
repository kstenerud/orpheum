// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { SystemDiscovery } from "./system-discovery.tsx";
import {
  buttonWithText,
  click,
  enter,
  mount,
  refusal,
  servedBy,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

/** The discovery the core suggests, as its status names it (D391 §7). */
const suggestedIssuer = "https://discovery.example";

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

it("connects to the discovery the server suggests, by its name and the admin's password, and shows the code to claim the server by (D273, D391)", async () => {
  const heard: Heard[] = [];
  const pending = {
    ...none,
    issuer: suggestedIssuer,
    coreApi: "https://api.discovery.example/core/v1",
    rootKeys: ["SHA256:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG"],
    rootVersion: 1,
    isSuggested: true,
    identity: "od1-service",
    coreId: "k7xq2",
    state: "pending",
    claimCode: "WXYZ-2345",
    claimExpiresAt: 1_790_000_000_000,
  };
  const services = servedBy(
    {
      "GET /system/discovery": [
        [200, none],
        [200, pending],
      ],
      "POST /system/discovery": [200, pending],
    },
    heard,
  );
  mounted = await mount(withServices(<SystemDiscovery />, services));
  const container = mounted.container;
  // The suggestion is the server's, offered as a person types it.
  expect(container.querySelector<HTMLInputElement>("form input")?.value).toBe("discovery.example");
  await enter(container.querySelector('form input[type="password"]'), "owner-password");
  await click(buttonWithText(container, "Connect"));
  expect(heard.map((each) => each.methodAndPath)).toEqual([
    "GET /system/discovery",
    "POST /system/discovery",
    "GET /system/discovery",
  ]);
  expect(heard[1]?.body).toEqual({ issuer: "discovery.example", password: "owner-password" });
  const text = container.textContent;
  expect(text).toContain(`Connected to the Orpheum service, at ${suggestedIssuer}.`);
  expect(text).toContain("Its identity is od1-service.");
  // The keys it trusts the discovery by, for the admin to compare (D391 §2).
  expect(text).toContain(
    "Trusted by the discovery's keys SHA256:abcdefghijklmnopqrstuvwxyz0123456789ABCDEFG (version 1).",
  );
  expect(text).toContain("Waiting to be claimed.");
  expect(text).toContain("enter the code WXYZ-2345");
});

it("says the server cannot trust the suggested discovery when this build knows none of its keys (D391 §7)", async () => {
  const issuerRefused = [
    422,
    {
      error: {
        code: "invalid_request",
        message: "a note for developers",
        detail: { problems: [{ location: "body.issuer", message: "no root keys" }] },
      },
    },
  ] as const;
  const services = servedBy({
    "GET /system/discovery": [200, none],
    "POST /system/discovery": issuerRefused,
    "POST /system/discovery/identify": issuerRefused,
  });
  mounted = await mount(withServices(<SystemDiscovery />, services));
  const container = mounted.container;
  await enter(container.querySelector('form input[type="password"]'), "owner-password");
  await click(buttonWithText(container, "Connect"));
  expect(container.querySelector('[role="alert"]')?.textContent).toBe(
    "This version of the server doesn't know the keys of discovery.example yet, so it can't trust it. Update the server, or connect to another discovery.",
  );
  // Another name refused there is no discovery's name.
  await enter(container.querySelector("form input"), "http://elsewhere.example");
  await click(buttonWithText(container, "Connect"));
  expect(container.querySelector('[role="alert"]')?.textContent).toBe(
    "That isn't a discovery's name. Type the name it gave you, as in discovery.example.",
  );
});

it("shows another discovery's identity to compare, and connects with it only once confirmed (D393 §4)", async () => {
  const heard: Heard[] = [];
  const pending = {
    ...none,
    issuer: "https://home.example",
    identity: "od1-home",
    name: "Home",
    state: "pending",
    claimCode: "WXYZ-2345",
    claimExpiresAt: 1_790_000_000_000,
  };
  const services = servedBy(
    {
      "GET /system/discovery": [
        [200, none],
        [200, pending],
      ],
      "POST /system/discovery/identify": [
        200,
        { issuer: "https://home.example", identity: "od1-home", name: "Home", isSuggested: false },
      ],
      "POST /system/discovery": [200, pending],
    },
    heard,
  );
  mounted = await mount(withServices(<SystemDiscovery />, services));
  const container = mounted.container;
  await enter(container.querySelector("form input"), "home.example");
  await enter(container.querySelector('form input[type="password"]'), "owner-password");
  await click(buttonWithText(container, "Connect"));
  // Nothing enrolled yet: the identity is shown, for the admin to compare with the operator's.
  expect(heard.map((each) => each.methodAndPath)).toEqual([
    "GET /system/discovery",
    "POST /system/discovery/identify",
  ]);
  expect(container.textContent).toContain("Home, at home.example, shows this identity.");
  expect(container.textContent).toContain("od1-home");
  await click(buttonWithText(container, "They are the same: connect"));
  expect(heard[2]?.body).toEqual({
    issuer: "home.example",
    password: "owner-password",
    identity: "od1-home",
  });
  expect(container.textContent).toContain("Connected to Home, at https://home.example.");
});

it("says where a claimed server is reached from, and warns of the discovery's deprecation", async () => {
  const services = servedBy({
    "GET /system/discovery": [
      200,
      {
        ...none,
        issuer: "https://discovery.example.com",
        documentProblem: "no location served an address document that holds",
        documentStaleAt: 1_790_000_000_000,
        name: "Home",
        state: "claimed",
        realm: "karlnas.live",
        reachableFrom: "ipv6",
        published: ["k7xq2.orpheum-core.com"],
        deadEndsLastWeek: 3,
        deprecatedUntil: "2027-03-01",
        clientOrigin: "https://k7xq2.client.example",
        fingerprint: "k7xq-2m4p-9tbd-hc3w-0vze-8gnr-5sya-j6kf-1qmw-p4dt-xb7c-re2h-9n0g",
        webrtcPaths: [
          { path: "direct_ipv6", count: 0 },
          { path: "punched_ipv4", count: 3 },
          { path: "hairpinned_ipv4", count: 2 },
          { path: "failed", count: 1 },
        ],
        portMapping: {
          isWanted: true,
          checkedAt: 1_790_000_000_000,
          mappings: [
            {
              method: "upnp",
              internal: "[2a0f:1000:1::20]:443",
              external: "[2a0f:1000:1::20]:443",
              expiresAt: 0,
            },
          ],
          problems: ["pcp: the router at 192.168.1.1:5351 did not answer"],
        },
      },
    ],
  });
  mounted = await mount(withServices(<SystemDiscovery />, services));
  const text = mounted.container.textContent;
  expect(text).toContain("Claimed: clients can find this server.");
  // The realm the discovery gave it, which only it shows (D298 §3).
  expect(text).toContain("at the discovery's page by the realm karlnas.live");
  expect(text).toContain("Clients with IPv6 reach it directly; the rest connect over WebRTC.");
  expect(text).toContain("Published as k7xq2.orpheum-core.com.");
  // How often a client found no way in, as the discovery counted (D293 §7).
  expect(text).toContain("Times in the last week a device found no way to reach this server: 3.");
  expect(text).toContain("Giving this machine a public IPv6 address helps the most");
  expect(text).toContain("stops speaking this server's protocol on 2027-03-01");
  // Where its links open, and the fingerprint they carry (D293 §4; the protocol's §3.1).
  expect(text).toContain("Links to this server open at https://k7xq2.client.example.");
  // No newer address document held: the server says it keeps the last it trusted (D391 §3).
  expect(text).toContain("the server keeps the last ones it trusted");
  // An expired document: the addresses may be stale, and the page says since when (the audit's F9).
  expect(text).toContain("this server cannot confirm they are current");
  expect(text).toContain("Connected to Home, at https://discovery.example.com.");
  expect(text).toContain(
    "fingerprint is k7xq-2m4p-9tbd-hc3w-0vze-8gnr-5sya-j6kf-1qmw-p4dt-xb7c-re2h-9n0g",
  );
  expect(text).toContain("[2a0f:1000:1::20]:443 reaches [2a0f:1000:1::20]:443, by UPnP.");
  expect(text).toContain("pcp: the router at 192.168.1.1:5351 did not answer");
  expect(text).toContain("through the router's NAT over IPv4: 3");
  expect(text).toContain("failed to connect: 1");
  // A hairpin is told apart from a punch: its devices are likely at home, coming the long way (D354).
  expect(text).toContain("out through the provider and back over IPv4");
  expect(text).not.toContain("directly over IPv6");
});

it("lists the house's IPv6 networks the discovery recognises devices at home by (D356 §10)", async () => {
  const witnessedAt = 1_790_000_000_000;
  const services = servedBy({
    "GET /system/discovery": [
      200,
      {
        ...none,
        issuer: "https://discovery.example.com",
        state: "claimed",
        houseNetworks: [
          {
            network: "2a0f:1000:1:2::/64",
            source: "learnt",
            device: "192.168.1.30",
            witnessedAt,
          },
          { network: "2a0f:3000:9:8::/64", source: "configured", witnessedAt: 0 },
        ],
      },
    ],
  });
  mounted = await mount(withServices(<SystemDiscovery />, services));
  const text = mounted.container.textContent;
  expect(text).toContain("recognises devices at home by your home's IPv6 networks");
  expect(text).toContain("2a0f:1000:1:2::/64, learnt from the device at 192.168.1.30");
  expect(text).toContain("2a0f:3000:9:8::/64, named in the config. It goes stale");
});

it("says nothing of the house's networks while there are none", async () => {
  const services = servedBy({
    "GET /system/discovery": [
      200,
      {
        ...none,
        issuer: "https://discovery.example.com",
        state: "claimed",
        houseNetworks: [],
      },
    ],
  });
  mounted = await mount(withServices(<SystemDiscovery />, services));
  expect(mounted.container.textContent).not.toContain("IPv6 networks");
});

it("tells the week's dead ends by the cause each device judged, beside their count, a word it doesn't know as another reason (D398 §7)", async () => {
  const services = servedBy({
    "GET /system/discovery": [
      200,
      {
        ...none,
        issuer: "https://discovery.example.com",
        state: "claimed",
        deadEndsLastWeek: 5,
        deadEndCauses: [
          { cause: "noIpv6", count: 2 },
          { cause: "none", count: 1 },
          { cause: "<b>made up</b>", count: 1 },
          { cause: "other", count: 1 },
        ],
      },
    ],
  });
  mounted = await mount(withServices(<SystemDiscovery />, services));
  const text = mounted.container.textContent;
  expect(text).toContain("Times in the last week a device found no way to reach this server: 5.");
  expect(text).toContain(
    "Why, as each device judged it: 2 on a network that seemed to have no IPv6, while this server had no IPv4 way in; 2 for another reason; 1 from a device too old to say why.",
  );
  expect(text).not.toContain("made up");
});

// The owner's Firefox 158 (2026-11-07): its secure connection never completed, and its dead end
// would have read as one where the device couldn't tell why.
it("tells a dead end whose secure connection never completed as that", async () => {
  const services = servedBy({
    "GET /system/discovery": [
      200,
      {
        ...none,
        issuer: "https://discovery.example.com",
        state: "claimed",
        deadEndsLastWeek: 1,
        deadEndCauses: [{ cause: "secureUnfinished", count: 1 }],
      },
    ],
  });
  mounted = await mount(withServices(<SystemDiscovery />, services));
  expect(mounted.container.textContent).toContain(
    "Why, as each device judged it: 1 where the server answered but the secure connection never completed in the device's browser.",
  );
});

it("tells the dead ends' count alone when the discovery keeps no causes", async () => {
  const services = servedBy({
    "GET /system/discovery": [
      200,
      { ...none, issuer: "https://discovery.example.com", state: "claimed", deadEndsLastWeek: 3 },
    ],
  });
  mounted = await mount(withServices(<SystemDiscovery />, services));
  const text = mounted.container.textContent;
  expect(text).toContain("Times in the last week a device found no way to reach this server: 3.");
  expect(text).not.toContain("as each device judged it");
});

const pendingAtTheService = {
  ...none,
  issuer: suggestedIssuer,
  coreId: "k7xq2",
  state: "pending",
  claimCode: "WXYZ-2345",
  claimExpiresAt: 1_790_000_000_000,
};

it("claims a waiting server by the key its household's account made, and names the account (the protocol's §2.2)", async () => {
  const heard: Heard[] = [];
  const claimed = {
    ...pendingAtTheService,
    state: "claimed",
    claimCode: undefined,
    claimExpiresAt: 0,
    account: "household@example.com",
  };
  const services = servedBy(
    {
      "GET /system/discovery": [
        [200, pendingAtTheService],
        [200, claimed],
      ],
      "POST /system/discovery/claim": [200, claimed],
    },
    heard,
  );
  mounted = await mount(withServices(<SystemDiscovery />, services));
  const container = mounted.container;
  await enter(container.querySelector("form input"), "  abcd efgh-jkmn pqrs ");
  await enter(container.querySelector('form input[type="password"]'), "owner-password");
  await click(buttonWithText(container, "Claim"));
  expect(heard.map((each) => each.methodAndPath)).toEqual([
    "GET /system/discovery",
    "POST /system/discovery/claim",
    "GET /system/discovery",
  ]);
  // The key goes as typed but for the ends, the discovery reading its spaces and dashes; the admin's
  // password with it, from which the server makes their realm key (D297 §3).
  expect(heard[1]?.body).toEqual({ key: "abcd efgh-jkmn pqrs", password: "owner-password" });
  const text = container.textContent;
  expect(text).toContain("Claimed: clients can find this server.");
  expect(text).toContain("Enrolled with the account household@example.com.");
  expect(container.querySelector("form")).toBeNull();
});

it("says a key the discovery refused was not accepted, and keeps the form", async () => {
  const services = servedBy({
    "GET /system/discovery": [200, pendingAtTheService],
    "POST /system/discovery/claim": refusal(422, "invalid_request"),
  });
  mounted = await mount(withServices(<SystemDiscovery />, services));
  const container = mounted.container;
  await enter(container.querySelector("form input"), "ABCDEFGHJKMNPQRS");
  await enter(container.querySelector('form input[type="password"]'), "owner-password");
  await click(buttonWithText(container, "Claim"));
  expect(container.querySelector('[role="alert"]')?.textContent).toBe(
    "The discovery didn't accept that key. Check it, or make a new one on your account.",
  );
  expect(container.querySelector("form input")).not.toBeNull();
});

it("says a password that isn't the admin's was refused, and that the key still works", async () => {
  const services = servedBy({
    "GET /system/discovery": [200, pendingAtTheService],
    "POST /system/discovery/claim": [
      422,
      {
        error: {
          code: "invalid_request",
          message: "a note for developers",
          detail: {
            problems: [{ location: "body.password", message: "that is not your password" }],
          },
        },
      },
    ],
  });
  mounted = await mount(withServices(<SystemDiscovery />, services));
  const container = mounted.container;
  await enter(container.querySelector("form input"), "ABCDEFGHJKMNPQRS");
  await enter(container.querySelector('form input[type="password"]'), "a-typo");
  await click(buttonWithText(container, "Claim"));
  expect(container.querySelector('[role="alert"]')?.textContent).toBe(
    "That isn't your password. Nothing was claimed, and the key still works.",
  );
});

it("tells a server removed from its account that another took its place (D312 §15)", async () => {
  const services = servedBy({
    "GET /system/discovery": [
      200,
      {
        ...pendingAtTheService,
        state: "released",
        claimCode: undefined,
        removedAt: 1_790_000_000_000,
      },
    ],
  });
  mounted = await mount(withServices(<SystemDiscovery />, services));
  const text = mounted.container.textContent;
  expect(text).toContain("Another server was enrolled with this server's account at");
  expect(text).toContain("Clients no longer find this one through the discovery.");
  expect(mounted.container.querySelector("form")).toBeNull();
});

it("shows no removal to a server too old to say whether it was removed, which sends no removedAt", async () => {
  const fromAnOlderServer = Object.fromEntries(
    Object.entries({ ...pendingAtTheService, state: "claimed" }).filter(
      ([key]) => key !== "removedAt",
    ),
  );
  const services = servedBy({ "GET /system/discovery": [200, fromAnOlderServer] });
  mounted = await mount(withServices(<SystemDiscovery />, services));
  expect(mounted.container.textContent).not.toContain("Another server was enrolled");
  expect(mounted.container.querySelector('[role="alert"]')).toBeNull();
});

it("shows a claimed server's home address once its certificate is held, and before then that it is coming (D292 §6, D297 §6)", async () => {
  const claimed = {
    ...none,
    issuer: suggestedIssuer,
    state: "claimed",
    homeAddresses: ["https://192-168-1-250.k7xq2.orpheum-core.com:8443"],
  };
  const services = servedBy({
    "GET /system/discovery": [
      [200, { ...claimed, isHomeAddressReady: false }],
      [200, { ...claimed, isHomeAddressReady: true }],
    ],
  });
  mounted = await mount(withServices(<SystemDiscovery />, services));
  expect(mounted.container.textContent).toContain("This server's home address is being set up");
  expect(mounted.container.querySelector(".home-address")).toBeNull();
  mounted.unmount();
  mounted = await mount(withServices(<SystemDiscovery />, services));
  const link = mounted.container.querySelector<HTMLAnchorElement>(".home-address a");
  expect(link?.href).toBe("https://192-168-1-250.k7xq2.orpheum-core.com:8443/");
  expect(mounted.container.querySelector(".home-address svg, .home-address img")).not.toBeNull();
  expect(mounted.container.textContent).not.toContain("is being set up");
});

it("tells a refused copy that another serves, and asks again only while it stands aside (D437 §3)", async () => {
  const heard: Heard[] = [];
  const claimed = { ...pendingAtTheService, state: "claimed", claimCode: undefined };
  const stoodAside = { ...claimed, stoodAsideAt: 1_790_000_000_000 };
  const services = servedBy(
    {
      "GET /system/discovery": [
        [200, stoodAside],
        [200, claimed],
      ],
      "POST /system/discovery/ask-again": [204, undefined],
    },
    heard,
  );
  mounted = await mount(withServices(<SystemDiscovery />, services));
  const container = mounted.container;
  expect(container.textContent).toContain(
    "Another copy of this server serves. This one stopped asking at",
  );
  await click(buttonWithText(container, "Ask again"));
  expect(heard.map((each) => each.methodAndPath)).toEqual([
    "GET /system/discovery",
    "POST /system/discovery/ask-again",
    "GET /system/discovery",
  ]);
  expect(container.textContent).not.toContain("Another copy of this server serves.");
  expect(buttonWithText(container, "Ask again")).toBeUndefined();
});

it("tells the serving copy that a copy of it tried to start, from where, and was refused (D437 §4)", async () => {
  const services = servedBy({
    "GET /system/discovery": [
      200,
      {
        ...pendingAtTheService,
        state: "claimed",
        claimCode: undefined,
        copyRefusedAt: 1_790_000_000_000,
        copyRefusedFrom: "203.0.113.9",
      },
    ],
  });
  mounted = await mount(withServices(<SystemDiscovery />, services));
  expect(mounted.container.textContent).toContain(
    "A copy of this server tried to start from 203.0.113.9 at",
  );
  expect(buttonWithText(mounted.container, "Ask again")).toBeUndefined();
});

// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { SystemDiscovery, serviceDiscoveryUrl } from "./system-discovery.tsx";
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

const none = {
  baseUrl: "",
  fingerprint: "",
  published: [],
  probe: [],
  claimExpiresAt: 0,
  removedAt: 0,
  lastHeartbeatAt: 0,
  deadEndsLastWeek: 0,
  portMapping: { isWanted: false, checkedAt: 0, mappings: [], problems: [] },
};

it("connects to the paid discovery by default and shows the code to claim the server by (D273)", async () => {
  const heard: Heard[] = [];
  const pending = {
    ...none,
    baseUrl: serviceDiscoveryUrl,
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
  await click(buttonWithText(container, "Connect"));
  expect(heard.map((each) => each.methodAndPath)).toEqual([
    "GET /system/discovery",
    "POST /system/discovery",
    "GET /system/discovery",
  ]);
  expect(heard[1]?.body).toEqual({ baseUrl: serviceDiscoveryUrl });
  const text = container.textContent;
  expect(text).toContain(`Connected to ${serviceDiscoveryUrl}.`);
  expect(text).toContain("Waiting to be claimed.");
  expect(text).toContain("enter the code WXYZ-2345");
});

it("says where a claimed server is reached from, and warns of the discovery's deprecation", async () => {
  const services = servedBy({
    "GET /system/discovery": [
      200,
      {
        ...none,
        baseUrl: "https://discovery.example.com/core/v1",
        state: "claimed",
        realm: "karlnas.live",
        reachableFrom: "ipv6",
        published: ["k7xq2.orpheum-core.com"],
        deadEndsLastWeek: 3,
        deprecatedUntil: "2027-03-01",
        clientOrigin: "https://app.orpheum-app.com",
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
  expect(text).toContain("Links to this server open at https://app.orpheum-app.com.");
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

const pendingAtTheService = {
  ...none,
  baseUrl: serviceDiscoveryUrl,
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
    baseUrl: serviceDiscoveryUrl,
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

// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { afterEach, expect, it } from "vitest";

import { SystemDiscovery, serviceDiscoveryUrl } from "./system-discovery.tsx";
import { buttonWithText, click, mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
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
  lastHeartbeatAt: 0,
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
        reachableFrom: "ipv6",
        published: ["k7xq2.orpheum-core.com"],
        deprecatedUntil: "2027-03-01",
        clientOrigin: "https://app.orpheum-app.com",
        fingerprint: "k7xq-2m4p-9tbd-hc3w-0vze-8gnr-5sya-j6kf-1qmw-p4dt-xb7c-re2h-9n0g",
        webrtcPaths: [
          { path: "direct_ipv6", count: 0 },
          { path: "punched_ipv4", count: 3 },
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
  expect(text).toContain("Clients with IPv6 reach it directly; the rest connect over WebRTC.");
  expect(text).toContain("Published as k7xq2.orpheum-core.com.");
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
  expect(text).not.toContain("directly over IPv6");
});

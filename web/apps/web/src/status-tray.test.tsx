// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, expect, it } from "vitest";

import { tabStorage } from "./browser-storage.ts";
import { knownDetailsOf } from "./hosted-details.ts";
import { LiveProvider } from "./live.tsx";
import { NeedsAttention } from "./needs-attention.tsx";
import { StatusTray } from "./status-tray.tsx";
import { mount, servedBy, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";
import { forgetMembership, keepMembership } from "./together-storage.ts";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  window.sessionStorage.clear();
  window.localStorage.clear();
});

const nothingWrong = {
  rootCount: 0,
  diskCount: 0,
  waitingRootCount: 0,
  unreadCount: 0,
  matchCount: 0,
};
const somethingWrong = { ...nothingWrong, waitingRootCount: 1, unreadCount: 2 };
const hostedSession = {
  sessionId: 7,
  nextRights: { canPlayPause: true, canSeek: true },
  links: [],
  members: [{ id: 1, name: "Karl", isHost: true, devices: [] }],
};

/** The tray's sign named label: a link, white (off) or blue (on), and where it leads. */
function sign(label: string): { readonly isOn: boolean; readonly href: string | null } | undefined {
  const found = mounted?.container.querySelector(`.status-tray a[aria-label="${label}"]`);
  return found === null || found === undefined
    ? undefined
    : {
        isOn: found.classList.contains("status-tray__button--on"),
        href: found.getAttribute("href"),
      };
}

it("always shows Cast's sign, white, leading to the devices to cast to, and Watch Together's to one who may host (D364 §12, §13)", async () => {
  mounted = await mount(
    withServices(<StatusTray isAdmin={false} canHostWatchTogether={false} />, servedBy({})),
  );
  expect(sign("Cast to a device")).toEqual({ isOn: false, href: "/casting" });
  expect(sign("Watch Together")).toBeUndefined();
  mounted.unmount();
  mounted = await mount(
    withServices(<StatusTray isAdmin={false} canHostWatchTogether />, servedBy({})),
  );
  expect(sign("Watch Together")).toEqual({ isOn: false, href: "/watch-together" });
});

it("has Cast's sign on a film's Play page take the film to the casting page, for a cast to start with (D364 §1)", async () => {
  const services = servedBy({});
  services.navigation.go({ screen: "play", fileId: 9 });
  mounted = await mount(
    withServices(<StatusTray isAdmin={false} canHostWatchTogether={false} />, services),
  );
  expect(sign("Cast to a device")).toEqual({ isOn: false, href: "/casting?file=9" });
});

it("turns Watch Together's sign blue for the host, leading to the session's page, and for a signed-in guest, to theirs (D364 §13, §14)", async () => {
  window.sessionStorage.setItem(
    "orpheum.together.host",
    JSON.stringify({ sessionId: 7, credential: "host-credential" }),
  );
  mounted = await mount(
    withServices(
      <StatusTray isAdmin={false} canHostWatchTogether />,
      servedBy({ "GET /together/7": [200, hostedSession] }),
    ),
  );
  expect(sign("Watch Together is on: its controls")).toEqual({
    isOn: true,
    href: "/watch-together",
  });
  mounted.unmount();
  window.sessionStorage.clear();
  window.localStorage.setItem(
    "orpheum.together.viewer",
    JSON.stringify({ sessionId: 8, credential: "guest-credential" }),
  );
  // A role that may not host still sees the session it is in, and returns to it by the sign.
  mounted = await mount(
    withServices(<StatusTray isAdmin={false} canHostWatchTogether={false} />, servedBy({})),
  );
  expect(sign("Watch Together is on: its controls")).toEqual({ isOn: true, href: "/together" });
  window.localStorage.clear();
});

it("has the hosted session's details at hand as the sign shows, so its page shows them at once (the owner, 2026-10-12)", async () => {
  window.sessionStorage.setItem(
    "orpheum.together.host",
    JSON.stringify({ sessionId: 17, credential: "host-credential" }),
  );
  const heard: Heard[] = [];
  mounted = await mount(
    withServices(
      <StatusTray isAdmin={false} canHostWatchTogether />,
      servedBy({ "GET /together/17": [200, { ...hostedSession, sessionId: 17 }] }, heard),
    ),
  );
  expect(heard.map((each) => each.methodAndPath)).toContain("GET /together/17");
  expect(knownDetailsOf(17)?.sessionId).toBe(17);
  window.sessionStorage.clear();
});

it("turns Cast's sign blue while this device casts, naming the receiver and what plays, and leads to Watch Together's page while that is on too (D364 §7, §13)", async () => {
  const server = fakeTogetherServer();
  const services = { ...servedBy({}, [], "token"), openTogetherSocket: server.openSocket };
  mounted = await mount(
    withServices(
      <LiveProvider>
        <StatusTray isAdmin={false} canHostWatchTogether />
      </LiveProvider>,
      services,
    ),
  );
  await server.accept();
  const casting = {
    castId: 5,
    receiverDeviceId: 2,
    receiverName: "Fire TV",
    fileId: 3,
    titleName: "Metropolis",
    durationMs: 0,
    isAccepted: true,
  };
  await server.say({ type: "sending", sending: casting });
  expect(sign("Casting Metropolis to Fire TV: its remote")).toEqual({ isOn: true, href: "/casting" });
  await server.say({ type: "sending", sending: { ...casting, fileId: 0, titleName: "" } });
  expect(sign("Casting to Fire TV, ready: its remote")).toEqual({ isOn: true, href: "/casting" });
  await act(async () => {
    keepMembership(tabStorage(), "host", { sessionId: 7, credential: "host-credential" });
    await Promise.resolve();
  });
  expect(sign("Casting to Fire TV, ready: its remote")?.href).toBe("/watch-together");
  await act(async () => {
    forgetMembership(tabStorage(), "host");
    await Promise.resolve();
  });
  await server.say({ type: "sending" });
  expect(sign("Cast to a device")).toEqual({ isOn: false, href: "/casting" });
});

it("shows an admin the yellow sign when something needs them, leading to the page of it, and not otherwise", async () => {
  mounted = await mount(
    withServices(
      <StatusTray isAdmin canHostWatchTogether={false} />,
      servedBy({ "GET /needs-attention": [200, somethingWrong] }),
    ),
  );
  const link = mounted.container.querySelector('[aria-label="Something needs your attention"]');
  expect(link?.getAttribute("href")).toBe("/needs-attention");
  expect(link?.querySelector(".warning-icon")).not.toBeNull();
  mounted.unmount();
  mounted = await mount(
    withServices(
      <StatusTray isAdmin canHostWatchTogether={false} />,
      servedBy({ "GET /needs-attention": [200, nothingWrong] }),
    ),
  );
  expect(
    mounted.container.querySelector('[aria-label="Something needs your attention"]'),
  ).toBeNull();
});

it("the page of what needs the admin counts each kind, a placeholder until it is designed", async () => {
  mounted = await mount(
    withServices(<NeedsAttention />, servedBy({ "GET /needs-attention": [200, somethingWrong] })),
  );
  const text = mounted.container.textContent;
  expect(text).toContain("1 library folders' scans wait for you");
  expect(text).toContain("2 files or folders could not be read.");
  expect(text).not.toContain("offline");
});

it("tells an admin of a series whose episodes were likely read wrongly, leading to it (D306 §6)", async () => {
  const readWrongly = {
    ...nothingWrong,
    seriesReadWrongly: [{ seriesId: 606, name: "Quillwick", unplacedCount: 24 }],
  };
  mounted = await mount(
    withServices(
      <StatusTray isAdmin canHostWatchTogether={false} />,
      servedBy({ "GET /needs-attention": [200, readWrongly] }),
    ),
  );
  expect(
    mounted.container.querySelector('[aria-label="Something needs your attention"]'),
  ).not.toBeNull();
  mounted.unmount();
  mounted = await mount(
    withServices(<NeedsAttention />, servedBy({ "GET /needs-attention": [200, readWrongly] })),
  );
  expect(mounted.container.querySelector('a[href="/series/606"]')?.textContent).toBe("Quillwick");
  expect(mounted.container.textContent).toContain(": 24 of its episodes have no number");
});

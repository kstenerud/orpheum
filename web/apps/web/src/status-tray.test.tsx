// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, expect, it } from "vitest";

import { knownDetailsOf } from "./hosted-details.ts";
import { LiveProvider } from "./live.tsx";
import { NeedsAttention } from "./needs-attention.tsx";
import { currentNotice } from "./notice.ts";
import { CastSignOverTheFilm, StatusTray } from "./status-tray.tsx";
import { areTrayDialogsLoaded } from "./tray-dialog-code.tsx";
import { closeTrayDialog } from "./tray-dialog.ts";
import {
  buttonNamed,
  buttonWithText,
  click,
  mount,
  refusal,
  servedBy,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { fakeTogetherServer } from "./test-together.ts";

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
/** The server's word of updates where it knows of none, which Needs attention reads too. */
const noUpdate = { "GET /system/update": [200, { version: "v0.3.1", checkedAt: 1 }] } as const;
const hostedSession = {
  sessionId: 7,
  nextRights: { canPlayPause: true, canSeek: true },
  links: [],
  members: [{ id: 1, name: "Karl", isHost: true, devices: [] }],
};

/**
 * The tray's sign named label: white (off) or blue (on), and where it leads, a link's address, or null
 * for a button, which opens its dialog over the page (D384 §2).
 */
function sign(label: string): { readonly isOn: boolean; readonly href: string | null } | undefined {
  const found = mounted?.container.querySelector(
    `.status-tray a[aria-label="${label}"], .status-tray button[aria-label="${label}"]`,
  );
  return found === null || found === undefined
    ? undefined
    : {
        isOn: found.classList.contains("icon-button--running"),
        href: found.getAttribute("href"),
      };
}

it("always shows Cast's sign, white, a button that opens its dialog, and Watch Together's to one who may host (D364 §12, §13; D384 §2)", async () => {
  mounted = await mount(
    withServices(<StatusTray isAdmin={false} canHostWatchTogether={false} />, servedBy({})),
  );
  expect(sign("Cast to a device")).toEqual({ isOn: false, href: null });
  expect(sign("Watch Together")).toBeUndefined();
  mounted.unmount();
  mounted = await mount(
    withServices(<StatusTray isAdmin={false} canHostWatchTogether />, servedBy({})),
  );
  expect(sign("Watch Together")).toEqual({ isOn: false, href: null });
});

/** Presses the tray's sign named label and waits for its dialog's code to load and open. */
async function openDialogOf(label: string): Promise<HTMLDialogElement> {
  const button = mounted?.container.querySelector<HTMLButtonElement>(
    `.status-tray button[aria-label="${label}"]`,
  );
  await act(async () => {
    button?.click();
    await Promise.resolve();
  });
  await settleUntil(() => document.querySelector("dialog[open]") !== null);
  const dialog = document.querySelector<HTMLDialogElement>("dialog[open]");
  if (dialog === null) {
    throw new Error(`no dialog opened by ${label}`);
  }
  return dialog;
}

// The owner's S23, 2026-10-30: Cast left the Watch Together page for a casting page of its own.
it("has Cast's sign open its dialog over the page, which stays where it is, offering the devices to cast its film to (D384 §2, D364 §1)", async () => {
  const server = fakeTogetherServer();
  const services = {
    ...servedBy({ "GET /me/devices": [200, { devices: [] }] }, [], "token"),
    openTogetherSocket: server.openSocket,
  };
  services.navigation.go({ screen: "play", fileId: 9 });
  mounted = await mount(
    withServices(
      <LiveProvider>
        <StatusTray isAdmin={false} canHostWatchTogether={false} />
      </LiveProvider>,
      services,
    ),
  );
  await server.accept();
  await server.say({ type: "sending" });
  const dialog = await openDialogOf("Cast to a device");
  await settleUntil(() => dialog.textContent.includes("None of your other devices"));
  expect(dialog.textContent).toContain("Cast to");
  expect(services.navigation.route()).toEqual({ screen: "play", fileId: 9 });
  await act(async () => {
    buttonNamed(dialog, "Close")?.click();
    await Promise.resolve();
  });
  expect(document.querySelector("dialog[open]")).toBeNull();
  expect(services.navigation.route()).toEqual({ screen: "play", fileId: 9 });
});

// The owner, 2026-11-04: the film fills the window over the tray (D418 §1), so Cast's sign shows over
// the film too, its dialog drawn there, the overlay held shown while it is open.
it("has Cast's sign over the film open its dialog in the player, telling the overlay it is open until it closes, the page staying where it is", async () => {
  const server = fakeTogetherServer();
  const services = {
    ...servedBy({ "GET /me/devices": [200, { devices: [] }] }, [], "token"),
    openTogetherSocket: server.openSocket,
  };
  services.navigation.go({ screen: "play", fileId: 9 });
  const opens: boolean[] = [];
  mounted = await mount(
    withServices(
      <LiveProvider>
        <div className="player-overlay">
          <CastSignOverTheFilm onOpenChange={(isOpen) => opens.push(isOpen)} />
        </div>
      </LiveProvider>,
      services,
    ),
  );
  await server.accept();
  await server.say({ type: "sending" });
  const button = mounted.container.querySelector<HTMLButtonElement>(
    '.player-overlay button[aria-label="Cast to a device"]',
  );
  expect(button?.classList.contains("icon-button--over-video")).toBe(true);
  await act(async () => {
    button?.click();
    await Promise.resolve();
  });
  await settleUntil(() => document.querySelector(".player-overlay dialog[open]") !== null);
  const dialog = document.querySelector<HTMLDialogElement>(".player-overlay dialog[open]");
  if (dialog === null) {
    throw new Error("no dialog opened over the film");
  }
  await settleUntil(() => dialog.textContent.includes("None of your other devices"));
  expect(opens).toEqual([true]);
  await act(async () => {
    buttonNamed(dialog, "Close")?.click();
    await Promise.resolve();
  });
  expect(document.querySelector("dialog[open]")).toBeNull();
  expect(opens).toEqual([true, false]);
  expect(services.navigation.route()).toEqual({ screen: "play", fileId: 9 });
});

/** The page's text but any dialog's: what shows in place of a sign while its dialog's code loads. */
function textOutsideDialogs(): string {
  const page = document.body.cloneNode(true);
  if (!(page instanceof HTMLElement)) {
    return "";
  }
  for (const dialog of page.querySelectorAll("dialog")) {
    dialog.remove();
  }
  return page.textContent;
}

// The owner, 2026-11-03 (plans/toolkit.md item 10): a first press of Cast or Watch Together showed
// "Loading…" in place of the sign, and the dialog some time later.
it("loads Cast's and Watch Together's dialogs' code ahead once the page has shown, so a first press opens each at once, never Loading… in the sign's place", async () => {
  const server = fakeTogetherServer();
  const services = {
    ...servedBy({ "GET /me/devices": [200, { devices: [] }] }, [], "token"),
    openTogetherSocket: server.openSocket,
  };
  mounted = await mount(
    withServices(
      <LiveProvider>
        <StatusTray isAdmin={false} canHostWatchTogether />
      </LiveProvider>,
      services,
    ),
  );
  expect(areTrayDialogsLoaded()).toBe(false); // not part of what the first page loads
  await server.accept();
  await settleUntil(areTrayDialogsLoaded); // no press: loaded once the page has shown
  for (const label of ["Cast to a device", "Watch Together"]) {
    act(() => {
      mounted?.container
        .querySelector<HTMLButtonElement>(`.status-tray button[aria-label="${label}"]`)
        ?.click();
    });
    expect(document.querySelector("dialog[open]")).not.toBeNull();
    expect(textOutsideDialogs()).not.toContain("Loading…");
    act(() => {
      closeTrayDialog();
    });
  }
});

it("turns Watch Together's sign blue for the host and for a signed-in guest, a button that opens its dialog over the page (D364 §13, §14; D384 §2)", async () => {
  window.sessionStorage.setItem(
    "orpheum.together.host",
    JSON.stringify({ sessionId: 7, credential: "host-credential" }),
  );
  const services = servedBy({ "GET /together/7": [200, hostedSession] });
  services.navigation.go({ screen: "play", fileId: 9 });
  mounted = await mount(
    withServices(<StatusTray isAdmin={false} canHostWatchTogether />, services),
  );
  expect(sign("Watch Together is on: its controls")).toEqual({ isOn: true, href: null });
  const dialog = await openDialogOf("Watch Together is on: its controls");
  expect(dialog.textContent).toContain("Your session is on.");
  expect(services.navigation.route()).toEqual({ screen: "play", fileId: 9 });
  // Its way back to the film moves the page, and the dialog closes with it.
  await act(async () => {
    services.navigation.go({ screen: "play", fileId: 4 });
    await Promise.resolve();
  });
  expect(document.querySelector("dialog[open]")).toBeNull();
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
  expect(sign("Watch Together is on: its controls")).toEqual({ isOn: true, href: null });
  window.localStorage.clear();
});

// The owner, 2026-10-30: the session ended from its dialog, Watch Together's dialog showed again; and
// the host's film played on alone (D389 §1).
it("closes Watch Together's dialog as the session is ended from it, nothing opening it again, and goes home with no word (D385's Also, D389 §1)", async () => {
  window.sessionStorage.setItem(
    "orpheum.together.host",
    JSON.stringify({ sessionId: 7, credential: "host-credential" }),
  );
  const heard: Heard[] = [];
  const services = servedBy(
    { "GET /together/7": [200, hostedSession], "DELETE /together/7": [204, undefined] },
    heard,
    "token",
  );
  services.navigation.go({ screen: "play", fileId: 9 });
  mounted = await mount(
    withServices(<StatusTray isAdmin={false} canHostWatchTogether />, services),
  );
  const dialog = await openDialogOf("Watch Together is on: its controls");
  await click(buttonWithText(dialog, "End the session"));
  await click(buttonWithText(dialog, "End it"));
  await settleUntil(() => heard.some((each) => each.methodAndPath === "DELETE /together/7"));
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  expect(document.querySelector("dialog[open]")).toBeNull();
  expect(window.sessionStorage.getItem("orpheum.together.host")).toBeNull();
  expect(sign("Watch Together")).toEqual({ isOn: false, href: null });
  expect(services.navigation.route()).toEqual({ screen: "home" });
  expect(currentNotice()).toBeUndefined();
});

// Principles §28: the answer to an end that ended it lost, the second try is answered 404; it showed
// a problem and said the session ended elsewhere.
it("takes a session the server no longer has as ended, as its end is tried again, going home with no word", async () => {
  window.sessionStorage.setItem(
    "orpheum.together.host",
    JSON.stringify({ sessionId: 7, credential: "host-credential" }),
  );
  const heard: Heard[] = [];
  const services = servedBy(
    { "GET /together/7": [200, hostedSession], "DELETE /together/7": refusal(404, "not_found") },
    heard,
    "token",
  );
  services.navigation.go({ screen: "play", fileId: 9 });
  mounted = await mount(
    withServices(<StatusTray isAdmin={false} canHostWatchTogether />, services),
  );
  const dialog = await openDialogOf("Watch Together is on: its controls");
  await click(buttonWithText(dialog, "End the session"));
  await click(buttonWithText(dialog, "End it"));
  await settleUntil(() => heard.some((each) => each.methodAndPath === "DELETE /together/7"));
  await act(async () => {
    await new Promise((resolve) => setTimeout(resolve, 0));
  });
  expect(document.querySelector("dialog[open]")).toBeNull();
  expect(services.navigation.route()).toEqual({ screen: "home" });
  expect(currentNotice()).toBeUndefined();
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

it("turns Cast's sign blue while this device casts, naming the receiver and what plays, its dialog saying so, with the way back to the film and Stop casting (D364 §7; D384 §2)", async () => {
  const server = fakeTogetherServer();
  const heard: Heard[] = [];
  const services = {
    ...servedBy(
      { "POST /me/casts/5/end": [200, { positionMs: 0, isPlaying: false }] },
      heard,
      "token",
    ),
    openTogetherSocket: server.openSocket,
  };
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
    itemName: "Metropolis",
    durationMs: 0,
    isAccepted: true,
  };
  await server.say({ type: "sending", sending: { ...casting, fileId: 0, itemName: "" } });
  expect(sign("Casting to Fire TV, ready")).toEqual({ isOn: true, href: null });
  await server.say({ type: "sending", sending: casting });
  expect(sign("Casting Metropolis to Fire TV")).toEqual({ isOn: true, href: null });
  const dialog = await openDialogOf("Casting Metropolis to Fire TV");
  expect(dialog.textContent).toContain("Playing Metropolis on Fire TV");
  await act(async () => {
    buttonWithText(dialog, "Back to the film")?.click();
    await Promise.resolve();
  });
  expect(services.navigation.route()).toEqual({ screen: "play", fileId: 3 });
  const again = await openDialogOf("Casting Metropolis to Fire TV");
  expect(buttonWithText(again, "Back to the film")).toBeUndefined(); // on its page already
  await act(async () => {
    buttonWithText(again, "Stop casting")?.click();
    await Promise.resolve();
  });
  await settleUntil(() => heard.some((each) => each.methodAndPath === "POST /me/casts/5/end"));
  expect(heard.find((each) => each.methodAndPath === "POST /me/casts/5/end")?.body).toEqual({
    reason: "stopped",
  });
  await server.say({ type: "sending" });
  expect(sign("Cast to a device")).toEqual({ isOn: false, href: null });
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
    withServices(
      <NeedsAttention />,
      servedBy({ "GET /needs-attention": [200, somethingWrong], ...noUpdate }),
    ),
  );
  const text = mounted.container.textContent;
  expect(text).toContain("1 library folders' scans wait for you");
  expect(text).toContain("2 files or folders could not be read.");
  expect(text).not.toContain("offline");
});

it("tells an admin of a series whose episodes were likely read wrongly, leading to it (D306 §6)", async () => {
  const readWrongly = {
    ...nothingWrong,
    seriesReadWrongly: [{ seriesId: 606, name: "Greywolf", unplacedCount: 24 }],
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
    withServices(
      <NeedsAttention />,
      servedBy({ "GET /needs-attention": [200, readWrongly], ...noUpdate }),
    ),
  );
  expect(mounted.container.querySelector('a[href="/series/606"]')?.textContent).toBe("Greywolf");
  expect(mounted.container.textContent).toContain(": 24 of its episodes have no number");
});

it("tells an admin of a film whose video has no sound, leading to it (the owner, 2026-11-02)", async () => {
  const silent = {
    ...nothingWrong,
    filmsWithoutSound: [{ titleId: 880, name: "Mister 517" }],
  };
  mounted = await mount(
    withServices(
      <StatusTray isAdmin canHostWatchTogether={false} />,
      servedBy({ "GET /needs-attention": [200, silent] }),
    ),
  );
  expect(
    mounted.container.querySelector('[aria-label="Something needs your attention"]'),
  ).not.toBeNull();
  mounted.unmount();
  mounted = await mount(
    withServices(
      <NeedsAttention />,
      servedBy({ "GET /needs-attention": [200, silent], ...noUpdate }),
    ),
  );
  expect(mounted.container.querySelector('a[href="/titles/880"]')?.textContent).toBe("Mister 517");
  expect(mounted.container.textContent).toContain(": its video has no sound track");
});

// ui-audit-2026-11-04.md §3, How to update: no menu leads to it; Needs attention does.
it("tells an admin of a newer version, leading to How to update, and says nothing needs them only without one", async () => {
  const newer = { version: "v0.3.1", latest: "v0.4.0", checkedAt: 1 };
  mounted = await mount(
    withServices(
      <NeedsAttention />,
      servedBy({
        "GET /needs-attention": [200, nothingWrong],
        "GET /system/update": [200, newer],
      }),
    ),
  );
  const howTo = mounted.container.querySelector('li a[href="/update"]');
  expect(howTo?.textContent).toBe("How to update");
  expect(howTo?.closest("li")?.textContent).toContain("v0.4.0 is available.");
  expect(mounted.container.textContent).not.toContain("Nothing needs your attention.");
  mounted.unmount();
  mounted = await mount(
    withServices(
      <NeedsAttention />,
      servedBy({ "GET /needs-attention": [200, nothingWrong], ...noUpdate }),
    ),
  );
  expect(mounted.container.querySelector('a[href="/update"]')).toBeNull();
  expect(mounted.container.textContent).toContain("Nothing needs your attention.");
});

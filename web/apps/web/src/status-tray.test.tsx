// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, expect, it } from "vitest";

import type { AttentionItem, AttentionPageKey } from "./attention-pages.ts";
import { hostedDetailsKey } from "./hosted-details.ts";
import { heldAnswer } from "./use-resource.ts";
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
  pressToStartTheCast,
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
  seriesReadWrongly: [],
  seriesReadWronglyCount: 0,
  filmsWithoutSound: [],
  filmsWithoutSoundCount: 0,
  unknownFailureAlertCount: 0,
  copyRefusedAt: 0,
  copyRefusedFrom: "",
  stoodAsideAt: 0,
};
const somethingWrong = { ...nothingWrong, waitingRootCount: 1, unreadCount: 2 };
/** A page's line of what needs the admin that says the text given. */
function lineSaying(container: Element, text: string): Element | undefined {
  return [...container.querySelectorAll("li")].find((each) => each.textContent.includes(text));
}

/** A link's words and where it leads. */
function linkOf(link: Element | null): readonly (string | null | undefined)[] {
  return [link?.textContent, link?.getAttribute("href")];
}

/**
 * The server's pages of what needs the admin (D538 §12): each page named with its items, as the
 * server lists them, the rest with none.
 */
function pagesOf(
  given: Readonly<Partial<Record<AttentionPageKey, readonly AttentionItem[]>>>,
): object {
  return Object.fromEntries(
    Object.entries(given).map(([page, items]) => [
      page,
      { isProblem: items.some((item) => item.isProblem), itemCount: items.length, items },
    ]),
  );
}
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
    buttonNamed(dialog, "Done")?.click();
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
    buttonNamed(dialog, "Done")?.click();
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
    { "GET /together/7": [200, hostedSession], "DELETE /together/7": refusal(404, "notFound") },
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
  expect(heldAnswer<{ sessionId: number }>(hostedDetailsKey(17))?.sessionId).toBe(17);
  window.sessionStorage.clear();
});

it("forgets, at each test's mount, the hosted details an earlier test's tray fetched, as a new tab would", async () => {
  window.sessionStorage.setItem(
    "orpheum.together.host",
    JSON.stringify({ sessionId: 17, credential: "host-credential" }),
  );
  mounted = await mount(
    withServices(
      <StatusTray isAdmin={false} canHostWatchTogether />,
      servedBy({ "GET /together/17": [200, { ...hostedSession, sessionId: 17 }] }),
    ),
  );
  expect(heldAnswer(hostedDetailsKey(17))).toBeDefined();
  mounted.unmount();
  window.sessionStorage.clear();
  mounted = await mount(withServices(<p>{"next test"}</p>));
  expect(heldAnswer(hostedDetailsKey(17))).toBeUndefined();
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

// The owner's 11-20 batch, finding 5: Firefox on Linux, cast to and out of full screen, showed
// nothing in the tray saying so.
it("turns Cast's sign blue on a device being cast to, naming the device that casts, or another of the person's where its name is not known", async () => {
  const server = fakeTogetherServer();
  const services = {
    ...servedBy({ "POST /me/casts/5/accept": [200, { castId: 5 }] }, [], "token"),
    openTogetherSocket: server.openSocket,
  };
  mounted = await mount(
    withServices(
      <LiveProvider>
        <StatusTray isAdmin={false} canHostWatchTogether={false} />
      </LiveProvider>,
      services,
    ),
  );
  await server.accept();
  const handed = {
    castId: 5,
    membershipCredential: "cast",
    fileId: 0,
    itemName: "",
    fromDeviceId: 2,
    fromDeviceName: "Mac",
  };
  await server.say({ type: "receive", receive: handed });
  expect(sign("Cast to a device")).toEqual({ isOn: false, href: null }); // asked, not yet receiving
  await pressToStartTheCast();
  expect(sign("Cast from Mac")).toEqual({ isOn: true, href: null });
  await server.say({ type: "receive", receive: { ...handed, fileId: 3, fromDeviceName: "" } });
  expect(sign("Cast from another of your devices")).toEqual({ isOn: true, href: null });
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

it("lists what needs the admin by the page it is fixed on, in the menu's order, each line leading to its place, the problems marked (D538 §12)", async () => {
  const counts = {
    ...nothingWrong,
    pages: pagesOf({
      system: [{ kind: "workFailing", isProblem: true, count: 4, windowHours: 24 }],
      librarySetup: [
        {
          kind: "folderWaiting",
          isProblem: true,
          count: 2,
          libraryId: 3,
          libraryName: "Films",
          rootPath: "/media/films",
        },
      ],
      titlesToCheck: [{ kind: "needsMatch", isProblem: false, count: 1 }],
    }),
  };
  mounted = await mount(
    withServices(<NeedsAttention />, servedBy({ "GET /needs-attention": [200, counts] })),
  );
  const { container } = mounted;
  expect(container.querySelector(".page__job")?.textContent).toBe(
    "What needs you, and where to fix it.",
  );
  const sections = [...container.querySelectorAll("section")];
  expect(sections.map((each) => each.querySelector("h2")?.textContent.trim())).toEqual([
    "Needs a match",
    "Libraries",
    "System",
  ]);
  expect(sections.every((each) => each.querySelector("h2 .warning-icon") !== null)).toBe(true);
  expect(sections.map((each) => linkOf(each.querySelector("a")))).toEqual([
    ["Go to Needs a match", "/identify"],
    ["Go to Libraries", "/libraries"],
    ["Go to System", "/system"],
  ]);
  const waiting = lineSaying(container, "waits for you");
  expect(linkOf(waiting?.querySelector("a") ?? null)).toEqual([
    "Films: the scan of /media/films waits for you before removing 2 files.",
    "/libraries/3",
  ]);
  expect(waiting?.querySelector(".warning-icon")).not.toBeNull();
  const match = lineSaying(container, "choose its match");
  expect(linkOf(match?.querySelector("a") ?? null)[1]).toBe("/identify");
  expect(match?.querySelector(".warning-icon")).toBeNull();
  const failing = lineSaying(container, "4 jobs failed in the last 24 hours.");
  expect(linkOf(failing?.querySelector("a") ?? null)[1]).toBe("/jobs");
});

it("says nothing needs the admin while no page has anything of a kind it knows (D432 §2)", async () => {
  // A kind a newer server sends, which this app's types do not name.
  const newer = {
    system: { isProblem: true, itemCount: 1, items: [{ kind: "newerKind", isProblem: true }] },
  };
  for (const pages of [pagesOf({}), newer]) {
    mounted = await mount(
      withServices(
        <NeedsAttention />,
        servedBy({ "GET /needs-attention": [200, { ...nothingWrong, pages }] }),
      ),
    );
    expect(mounted.container.textContent).toContain("Nothing needs your attention.");
    expect(mounted.container.querySelector("section")).toBeNull();
    mounted.unmount();
    mounted = undefined;
  }
});

it("lights the admin's sign from the pages where the server says them, whatever its counts say (D538 §12)", async () => {
  const signOf = async (counts: object): Promise<Element | null> => {
    mounted = await mount(
      withServices(
        <StatusTray isAdmin canHostWatchTogether={false} />,
        servedBy({ "GET /needs-attention": [200, counts] }),
      ),
    );
    const found = mounted.container.querySelector('[aria-label="Something needs your attention"]');
    mounted.unmount();
    mounted = undefined;
    return found;
  };
  expect(
    await signOf({
      ...nothingWrong,
      pages: pagesOf({
        update: [{ kind: "updateAvailable", isProblem: false, version: "v0.4.0" }],
      }),
    }),
  ).not.toBeNull();
  expect(await signOf({ ...somethingWrong, pages: pagesOf({}) })).toBeNull();
});

it("reads what needs the admin once for the tray's sign and the page of it together (D478)", async () => {
  const heard: Heard[] = [];
  const counts = {
    ...somethingWrong,
    pages: pagesOf({
      librarySetup: [
        { kind: "unread", isProblem: true, count: 2, libraryId: 3, libraryName: "Films" },
      ],
    }),
  };
  mounted = await mount(
    withServices(
      <>
        <StatusTray isAdmin canHostWatchTogether={false} />
        <NeedsAttention />
      </>,
      servedBy({ "GET /needs-attention": [200, counts] }, heard),
    ),
  );
  expect(mounted.container.textContent).toContain("Films: 2 files or folders could not be read.");
  expect(
    mounted.container.querySelector('[aria-label="Something needs your attention"]'),
  ).not.toBeNull();
  expect(heard.filter((each) => each.methodAndPath === "GET /needs-attention")).toHaveLength(1);
});

it("tells an admin of a series whose episodes were likely read wrongly, by name, leading to it (D306 §6)", async () => {
  const readWrongly = {
    ...nothingWrong,
    seriesReadWrongly: [{ seriesId: 606, name: "Greywolf", unplacedCount: 24 }],
    seriesReadWronglyCount: 1,
    pages: pagesOf({ titlesToCheck: [{ kind: "seriesReadWrongly", isProblem: true, count: 1 }] }),
  };
  mounted = await mount(
    withServices(<NeedsAttention />, servedBy({ "GET /needs-attention": [200, readWrongly] })),
  );
  expect(mounted.container.textContent).toContain(
    "1 series may have had its file names read wrongly.",
  );
  expect(mounted.container.querySelector('a[href="/series/606"]')?.textContent).toBe("Greywolf");
  expect(mounted.container.textContent).toContain(": 24 of its episodes have no number");
});

it("tells an admin of a film whose video has no sound, by name, leading to it (the owner, 2026-11-02)", async () => {
  const silent = {
    ...nothingWrong,
    filmsWithoutSound: [{ itemId: 880, name: "Mister 517" }],
    filmsWithoutSoundCount: 1,
    pages: pagesOf({ titlesToCheck: [{ kind: "filmsWithoutSound", isProblem: true, count: 1 }] }),
  };
  mounted = await mount(
    withServices(<NeedsAttention />, servedBy({ "GET /needs-attention": [200, silent] })),
  );
  expect(mounted.container.textContent).toContain(
    "1 film has no sound track, so it plays in silence.",
  );
  expect(mounted.container.querySelector('a[href="/titles/880"]')?.textContent).toBe("Mister 517");
  expect(mounted.container.textContent).toContain(": its video has no sound track");
});

it("shows an admin the yellow sign while failures of unknown cause pile up, with nothing else to say, leading to Needs attention (D517 §2)", async () => {
  const piling = { ...nothingWrong, unknownFailureAlertCount: 1 };
  mounted = await mount(
    withServices(
      <StatusTray isAdmin canHostWatchTogether={false} />,
      servedBy({ "GET /needs-attention": [200, piling] }),
    ),
  );
  const link = mounted.container.querySelector('[aria-label="Something needs your attention"]');
  expect(link?.getAttribute("href")).toBe("/needs-attention");
});

it("tells an admin of a library whose rescan gave up, leading to the library (the owner, 2026-11-13)", async () => {
  const gaveUp = {
    ...nothingWrong,
    rescansGivenUp: [{ libraryId: 3, name: "Films", givenUpAt: 9000 }],
    pages: pagesOf({
      librarySetup: [{ kind: "rescanGaveUp", isProblem: true, libraryId: 3, libraryName: "Films" }],
    }),
  };
  mounted = await mount(
    withServices(<NeedsAttention />, servedBy({ "GET /needs-attention": [200, gaveUp] })),
  );
  expect(mounted.container.querySelector('li a[href="/libraries/3"]')?.textContent).toBe(
    "Films: its rescan gave up, because the library changed every time it was read.",
  );
  expect(mounted.container.textContent).not.toContain("Nothing needs your attention.");
});

it("says how many more there are than Needs attention lists by name (D432 §4)", async () => {
  const many = {
    ...nothingWrong,
    seriesReadWrongly: [{ seriesId: 606, name: "Greywolf", unplacedCount: 24 }],
    seriesReadWronglyCount: 3,
    filmsWithoutSound: [{ itemId: 880, name: "Mister 517" }],
    filmsWithoutSoundCount: 101,
    pages: pagesOf({
      titlesToCheck: [
        { kind: "seriesReadWrongly", isProblem: true, count: 3 },
        { kind: "filmsWithoutSound", isProblem: true, count: 101 },
      ],
    }),
  };
  mounted = await mount(
    withServices(<NeedsAttention />, servedBy({ "GET /needs-attention": [200, many] })),
  );
  expect(mounted.container.textContent).toContain(
    "And 2 more series whose file names may have been read wrongly.",
  );
  expect(mounted.container.textContent).toContain("And 100 more films with no sound track.");
  mounted.unmount();
  // A server before the pages lights the sign from its counts.
  const countedOnly = { ...nothingWrong, filmsWithoutSoundCount: 1 };
  mounted = await mount(
    withServices(
      <StatusTray isAdmin canHostWatchTogether={false} />,
      servedBy({ "GET /needs-attention": [200, countedOnly] }),
    ),
  );
  expect(
    mounted.container.querySelector('[aria-label="Something needs your attention"]'),
  ).not.toBeNull();
});

it("tells an admin of a newer version, leading to Update", async () => {
  const newer = {
    ...nothingWrong,
    pages: pagesOf({ update: [{ kind: "updateAvailable", isProblem: false, version: "v0.4.0" }] }),
  };
  mounted = await mount(
    withServices(<NeedsAttention />, servedBy({ "GET /needs-attention": [200, newer] })),
  );
  const line = mounted.container.querySelector('li a[href="/update"]');
  expect(line?.textContent).toBe("v0.4.0 is available.");
  expect(line?.closest("li")?.querySelector(".warning-icon")).toBeNull();
  expect(mounted.container.textContent).not.toContain("Nothing needs your attention.");
});

it("tells the serving copy's admin that a copy of the server was refused, from where, leading to Network (D437 §4)", async () => {
  const refused = {
    ...nothingWrong,
    copyRefusedAt: 1_790_000_000_000,
    copyRefusedFrom: "203.0.113.9",
    pages: pagesOf({
      network: [
        { kind: "copyRefused", isProblem: true, name: "203.0.113.9", since: 1_790_000_000_000 },
      ],
    }),
  };
  mounted = await mount(
    withServices(<NeedsAttention />, servedBy({ "GET /needs-attention": [200, refused] })),
  );
  const line = mounted.container.querySelector('li a[href="/network"]');
  expect(line?.textContent).toContain("A copy of this server tried to start from 203.0.113.9 at");
  expect(line?.textContent).toContain("and was refused.");
});

it("tells a refused copy's admin that another copy serves, leading to Network to ask again (D437 §3, §4)", async () => {
  const stoodAside = {
    ...nothingWrong,
    stoodAsideAt: 1_790_000_000_000,
    pages: pagesOf({
      network: [{ kind: "stoodAside", isProblem: true, since: 1_790_000_000_000 }],
    }),
  };
  mounted = await mount(
    withServices(<NeedsAttention />, servedBy({ "GET /needs-attention": [200, stoodAside] })),
  );
  const line = mounted.container.querySelector('li a[href="/network"]');
  expect(line?.textContent).toContain("Another copy of this server serves.");
  expect(line?.textContent).toContain(
    "Asking again takes over only once the other copy has stopped.",
  );
});

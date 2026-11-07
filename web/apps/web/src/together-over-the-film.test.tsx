// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";
import { act, useState, type JSX } from "react";
import { afterEach, expect, it } from "vitest";

import type { Services } from "./services.tsx";
import {
  buttonNamed,
  buttonWithText,
  click,
  mount,
  servedBy,
  settleUntil,
  type Heard,
  type Mounted,
} from "./test-dom.tsx";
import { tabStorage } from "./browser-storage.ts";
import { LiveProvider } from "./live.tsx";
import { withServices } from "./test-services.tsx";
import { liveAndSession } from "./test-together.ts";
import { CanStartTogetherContext } from "./together-can-start.ts";
import { useMembership } from "./together-membership.ts";
import { TogetherPanel, TogetherSignOverTheFilm } from "./together-over-the-film.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  window.sessionStorage.clear();
  window.localStorage.clear();
});

const signLabel = "Watch Together is on: its controls";

/** The session this tab hosts, as the server tells it. */
const hostedSession = {
  sessionId: 3,
  nextRights: { canPlayPause: true, canSeek: true },
  links: [],
  members: [{ id: 1, name: "Karl", isHost: true, devices: [] }],
};

/** This tab hosting session 3, its details and its end served (heard into heard). */
function hosting(heard: Heard[] = []): Services {
  window.sessionStorage.setItem(
    "orpheum.together.host",
    JSON.stringify({ sessionId: 3, credential: "host-credential" }),
  );
  const services = servedBy(
    { "GET /together/3": [200, hostedSession], "DELETE /together/3": [204, undefined] },
    heard,
    "token",
  );
  services.navigation.go({ screen: "play", fileId: 9 });
  return services;
}

/** This browser a guest in session 8, by its membership. */
function guesting(): Services {
  window.localStorage.setItem(
    "orpheum.together.viewer",
    JSON.stringify({ sessionId: 8, credential: "guest-credential" }),
  );
  return servedBy({}, [], "token");
}

/**
 * A panel holding state of its own, as the voice controls do: a count its button raises; and a way
 * to leave, which takes the panel off its page, as a guest's Leave does.
 */
function Counting(props: { readonly onLeave: () => void }): JSX.Element {
  const [count, setCount] = useState(0);
  return (
    <>
      <button
        type="button"
        className="counting"
        onClick={() => {
          setCount(count + 1);
        }}
      >
        {`Count ${String(count)}`}
      </button>
      <button type="button" onClick={props.onLeave}>
        Leave
      </button>
    </>
  );
}

function Player(props: {
  readonly hasPanel: boolean;
  readonly onOpenChange: (isOpen: boolean) => void;
}): JSX.Element {
  const [hasPanel, setHasPanel] = useState(props.hasPanel);
  return (
    <>
      <div className="page">
        {hasPanel ? (
          <TogetherPanel>
            <Counting
              onLeave={() => {
                setHasPanel(false);
              }}
            />
          </TogetherPanel>
        ) : null}
      </div>
      <div className="player-overlay">
        <TogetherSignOverTheFilm onOpenChange={props.onOpenChange} />
      </div>
    </>
  );
}

function sign(): HTMLButtonElement | null {
  return document.querySelector<HTMLButtonElement>(
    `.player-overlay button[aria-label="${signLabel}"]`,
  );
}

/** Presses the sign over the film and waits for the dialog it opens there. */
async function openOverTheFilm(): Promise<HTMLDialogElement> {
  await act(async () => {
    sign()?.click();
    await Promise.resolve();
  });
  await settleUntil(
    () => document.querySelector(".player-overlay dialog[open] .counting") !== null,
  );
  const dialog = document.querySelector<HTMLDialogElement>(".player-overlay dialog[open]");
  if (dialog === null) {
    throw new Error("no dialog over the film");
  }
  return dialog;
}

it("shows Watch Together's sign over the film only while a session's panel is drawn, blue", async () => {
  mounted = await mount(withServices(<Player hasPanel={false} onOpenChange={() => undefined} />));
  expect(sign()).toBeNull();
  mounted.unmount();
  mounted = await mount(withServices(<Player hasPanel onOpenChange={() => undefined} />));
  await settleUntil(() => sign() !== null);
  expect(sign()?.classList.contains("icon-button--over-video")).toBe(true);
  expect(sign()?.classList.contains("icon-button--running")).toBe(true);
});

it("moves the session's panel, its state kept, into the dialog the sign opens over the film, and back to its page as it closes", async () => {
  const opens: boolean[] = [];
  mounted = await mount(
    withServices(<Player hasPanel onOpenChange={(isOpen) => opens.push(isOpen)} />, hosting()),
  );
  await settleUntil(() => sign() !== null);
  const counting = document.querySelector<HTMLButtonElement>(".page .counting");
  if (counting === null) {
    throw new Error("the panel is not drawn on its page");
  }
  await click(counting);
  const dialog = await openOverTheFilm();
  const moved = dialog.querySelector(".counting");
  // The same element, never drawn again: what it held goes on.
  expect(moved).toBe(counting);
  expect(moved?.textContent).toBe("Count 1");
  expect(document.querySelector(".page .counting")).toBeNull();
  expect(opens).toEqual([true]);
  await act(async () => {
    buttonNamed(dialog, "Close")?.click();
    await Promise.resolve();
  });
  await settleUntil(() => document.querySelector(".page .counting") !== null);
  expect(document.querySelector(".page .counting")).toBe(counting);
  expect(counting.textContent).toBe("Count 1");
  expect(opens).toEqual([true, false]);
});

it("opens the tray's Watch Together dialog over the film, the host's controls with End, the panel under what plays, which holds Talk and the volumes (D433 §1, §2)", async () => {
  mounted = await mount(
    withServices(<Player hasPanel onOpenChange={() => undefined} />, hosting()),
  );
  await settleUntil(() => sign() !== null);
  const dialog = await openOverTheFilm();
  await settleUntil(() => buttonWithText(dialog, "End the session") !== undefined);
  expect(dialog.getAttribute("aria-label")).toBe("Watch Together");
  expect(dialog.querySelector("h2#watch-together-heading")?.textContent).toBe("Watch Together");
  const controls = dialog.querySelector(".watch-together__controls");
  const panel = dialog.querySelector(".counting");
  if (controls === null || panel === null) {
    throw new Error("the dialog lacks the host's controls or the panel");
  }
  // The panel under what plays, above the people, End and the debug fold: it stood after them all,
  // a second dialog under the first (the owner, staging, 2026-11-07).
  const plays = controls.querySelector(".together-plays");
  const people = controls.querySelector(".watch-together__people-head");
  if (plays === null || people === null) {
    throw new Error("the controls lack what plays or the people");
  }
  expect(plays.compareDocumentPosition(panel) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
  expect(people.compareDocumentPosition(panel) & Node.DOCUMENT_POSITION_PRECEDING).not.toBe(0);
  // The panel's are Talk and the volumes: the controls draw no second film volume.
  const volumes = [...controls.querySelectorAll("label")].filter((label) =>
    label.textContent.startsWith("Film volume"),
  );
  expect(volumes).toEqual([]);
});

it("ends the session from the dialog over the film, closing it, as the tray's does (D433 §3)", async () => {
  const heard: Heard[] = [];
  const opens: boolean[] = [];
  const services = hosting(heard);
  mounted = await mount(
    withServices(<Player hasPanel onOpenChange={(isOpen) => opens.push(isOpen)} />, services),
  );
  await settleUntil(() => sign() !== null);
  const dialog = await openOverTheFilm();
  await settleUntil(() => buttonWithText(dialog, "End the session") !== undefined);
  await click(buttonWithText(dialog, "End the session"));
  await click(buttonWithText(dialog, "End it"));
  await settleUntil(() => heard.some((each) => each.methodAndPath === "DELETE /together/3"));
  await settleUntil(() => document.querySelector("dialog[open]") === null);
  expect(window.sessionStorage.getItem("orpheum.together.host")).toBeNull();
  expect(services.navigation.route()).toEqual({ screen: "home" });
  expect(opens).toEqual([true, false]);
});

it("opens the guest's dialog over the film with the panel and no way back to the film they watch, and closes it as they leave (D433)", async () => {
  const opens: boolean[] = [];
  mounted = await mount(
    withServices(<Player hasPanel onOpenChange={(isOpen) => opens.push(isOpen)} />, guesting()),
  );
  await settleUntil(() => sign() !== null);
  const dialog = await openOverTheFilm();
  expect(dialog.getAttribute("aria-label")).toBe("Watch Together");
  expect(dialog.textContent).not.toContain("Back to the film");
  expect(dialog.textContent).not.toContain("Loading");
  expect(buttonWithText(dialog, "End the session")).toBeUndefined();
  await click(buttonWithText(dialog, "Leave"));
  await settleUntil(() => document.querySelector("dialog[open]") === null);
  expect(opens).toEqual([true, false]);
  expect(sign()).toBeNull();
});

const startLabel = "Watch Together";

/**
 * A Play page whose panel is drawn while this tab hosts, as the host's player draws it
 * (HostPlaysTogether), its overlay told whether the person may start a session.
 */
function PlayerToStartFrom(props: { readonly canStart: boolean }): JSX.Element {
  const isHosting = useMembership(tabStorage, "host") !== undefined;
  return (
    <CanStartTogetherContext value={props.canStart}>
      <div className="page">
        {isHosting ? (
          <TogetherPanel>
            <p className="hosts-panel">The host's panel</p>
          </TogetherPanel>
        ) : null}
      </div>
      <div className="player-overlay">
        <TogetherSignOverTheFilm onOpenChange={() => undefined} />
      </div>
    </CanStartTogetherContext>
  );
}

function startSign(): HTMLButtonElement | null {
  return document.querySelector<HTMLButtonElement>(
    `.player-overlay button[aria-label="${startLabel}"]`,
  );
}

it("shows the sign over the film with no session, white, to one who may start a session, and to no one else (D440 §6)", async () => {
  mounted = await mount(withServices(<PlayerToStartFrom canStart />));
  await settleUntil(() => startSign() !== null);
  expect(startSign()?.classList.contains("icon-button--over-video")).toBe(true);
  expect(startSign()?.classList.contains("icon-button--running")).toBe(false);
  mounted.unmount();
  // A role that may not host, or a guest by link, whose page tells nothing: no sign.
  mounted = await mount(withServices(<PlayerToStartFrom canStart={false} />));
  expect(startSign()).toBeNull();
  expect(sign()).toBeNull();
  mounted.unmount();
  // A guest of a session whose panel is not drawn here is offered no second session, as before.
  mounted = await mount(withServices(<PlayerToStartFrom canStart />, guesting()));
  expect(startSign()).toBeNull();
  expect(sign()).toBeNull();
});

it("starts a session from the sign over the film, as the tray does, the dialog staying open over the film as the host's, the new panel moved in (D440 §6)", async () => {
  const heard: Heard[] = [];
  const services = servedBy(
    {
      "POST /together": [
        200,
        { sessionId: 3, membershipCredential: "host-credential", isNew: true },
      ],
      "GET /together/3": [200, hostedSession],
    },
    heard,
    "token",
  );
  services.navigation.go({ screen: "play", fileId: 9 });
  mounted = await mount(withServices(<PlayerToStartFrom canStart />, services));
  await settleUntil(() => startSign() !== null);
  await act(async () => {
    startSign()?.click();
    await Promise.resolve();
  });
  await settleUntil(() => buttonWithText(document.body, "Start a session") !== undefined);
  const dialog = document.querySelector<HTMLDialogElement>(".player-overlay dialog[open]");
  if (dialog === null) {
    throw new Error("no dialog over the film");
  }
  expect(dialog.getAttribute("aria-label")).toBe("Watch Together");
  await click(buttonWithText(dialog, "Start a session"));
  expect(heard.map((each) => each.methodAndPath)).toContain("POST /together");
  await settleUntil(() => buttonWithText(dialog, "End the session") !== undefined);
  // The dialog stayed open, and the session's panel is in it, under the host's controls.
  await settleUntil(() => dialog.querySelector(".hosts-panel") !== null);
  expect(dialog.open).toBe(true);
  expect(document.querySelector(".page .hosts-panel")).toBeNull();
  expect(sign()?.classList.contains("icon-button--running")).toBe(true);
  expect(startSign()).toBeNull();
});

it("offers no session to start over the film while this device receives a cast, whose session it plays (D440 §6)", async () => {
  const services = servedBy({ "POST /me/casts/5/accept": [200, { castId: 5 }] }, [], "token");
  const sockets = liveAndSession();
  mounted = await mount(
    withServices(
      <LiveProvider>
        <PlayerToStartFrom canStart />
      </LiveProvider>,
      { ...services, openTogetherSocket: sockets.openSocket },
    ),
  );
  await sockets.live.accept();
  await settleUntil(() => startSign() !== null);
  await sockets.live.say({
    type: "receive",
    receive: {
      castId: 5,
      membershipCredential: "cast",
      fileId: 0,
      fromDeviceId: 2,
      fromDeviceName: "Pixel",
    },
  });
  await click(buttonWithText(document.body, "Use"));
  await settleUntil(() => startSign() === null);
  expect(sign()).toBeNull();
});

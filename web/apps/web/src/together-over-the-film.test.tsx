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
import { withServices } from "./test-services.tsx";
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

it("opens the tray's Watch Together dialog over the film, the host's controls with End, the panel under them, which holds Talk and the volumes (D433 §1, §2)", async () => {
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
  // The panel under the controls, not among them.
  expect(controls.contains(panel)).toBe(false);
  expect(controls.compareDocumentPosition(panel) & Node.DOCUMENT_POSITION_FOLLOWING).not.toBe(0);
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

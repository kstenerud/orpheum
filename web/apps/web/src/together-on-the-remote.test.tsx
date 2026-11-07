// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { TogetherVoice, VoiceSnapshot } from "@orpheum/sdk";
import { act, type JSX } from "react";
import { afterEach, expect, it } from "vitest";

import {
  buttonNamed,
  buttonWithText,
  enter,
  mount,
  servedBy,
  settleUntil,
  type Mounted,
} from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";
import { CanStartTogetherContext } from "./together-can-start.ts";
import { TogetherOnTheRemote } from "./together-on-the-remote.tsx";
import { SessionVoiceContext, type VoiceInUse } from "./together-voice.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  window.sessionStorage.clear();
  window.localStorage.clear();
});

/** The session's voice as a test holds it: Dan and Kim heard, each gain set noted in set. */
function voiceNoting(set: [number, number][], isWaiting = false): VoiceInUse {
  const snapshot = {
    status: "off",
    isConnected: true,
    isHearing: true,
    partners: [
      { memberId: 2, name: "Dan", state: "connected" },
      { memberId: 3, name: "Kim", state: "connected" },
    ],
    partnerGains: {},
  } as unknown as VoiceSnapshot;
  const voice = {
    setPartnerGain: (memberId: number, gain: number) => {
      set.push([memberId, gain]);
    },
  } as unknown as TogetherVoice;
  return { voice, snapshot, tap: { isWaiting, play: () => undefined } };
}

/** The session this tab hosts, as the server tells it. */
const hostedSession = {
  sessionId: 3,
  nextRights: { canPlayPause: true, canSeek: true },
  links: [],
  members: [
    { id: 1, name: "Karl", isHost: true, devices: [] },
    { id: 2, name: "Dan", devices: [] },
  ],
};

/** This tab hosting session 3, its details served. */
function hosting(): ReturnType<typeof servedBy> {
  window.sessionStorage.setItem(
    "orpheum.together.host",
    JSON.stringify({ sessionId: 3, credential: "host-credential" }),
  );
  return servedBy({ "GET /together/3": [200, hostedSession] }, [], "token");
}

function Remote(props: {
  readonly voice: VoiceInUse | undefined;
  readonly canStart?: boolean;
}): JSX.Element {
  return (
    <CanStartTogetherContext value={props.canStart ?? false}>
      <section className="cast-remote">
        <SessionVoiceContext value={props.voice}>
          <TogetherOnTheRemote />
        </SessionVoiceContext>
      </section>
    </CanStartTogetherContext>
  );
}

const onLabel = "Watch Together is on: its controls";

it("shows each other person's slider on the remote, which sets how loud their voice plays here (D440 §2, §5)", async () => {
  const set: [number, number][] = [];
  mounted = await mount(withServices(<Remote voice={voiceNoting(set)} />, hosting()));
  const row = document.querySelector(".cast-remote .volume-row");
  if (row === null) {
    throw new Error("no slider row on the remote");
  }
  const [dan, kim, ...others] = row.querySelectorAll<HTMLInputElement>("input[type=range]");
  if (dan === undefined || kim === undefined) {
    throw new Error("no slider for Dan or Kim");
  }
  expect(others).toEqual([]);
  expect(dan.getAttribute("aria-label")).toBe("Dan's voice");
  expect(row.querySelector(".volume-row__letters")?.textContent).toBe("D");
  // Part of the page, not a pop-up's one control: each slider takes the browser's focus.
  expect(dan.tabIndex).not.toBe(-1);
  act(() => {
    dan.focus();
  });
  expect(row.querySelector(".volume-row__name")?.textContent).toBe("Dan");
  await enter(kim, "2");
  expect(set.at(-1)?.[0]).toBe(3);
});

it("opens the host's Watch Together dialog from the remote, the people, Invite and End, and no volume the row carries (D440 §6, §8)", async () => {
  mounted = await mount(withServices(<Remote voice={voiceNoting([])} />, hosting()));
  const button = document.querySelector<HTMLButtonElement>(
    `.cast-remote button[aria-label="${onLabel}"]`,
  );
  if (button === null) {
    throw new Error("no Watch Together button on the remote");
  }
  // The host's panel is drawn for the dialog alone: nothing of it shows on the remote's page.
  expect(document.querySelector(".cast-remote .together-panel")?.hasAttribute("hidden")).toBe(true);
  await act(async () => {
    button.click();
    await Promise.resolve();
  });
  await settleUntil(() => document.querySelector("dialog[open] .together-voice") !== null);
  const dialog = document.querySelector<HTMLDialogElement>("dialog[open]");
  if (dialog === null) {
    throw new Error("no dialog");
  }
  await settleUntil(() => buttonWithText(dialog, "End the session") !== undefined);
  expect(dialog.textContent).toContain("People");
  // The row carries each person's volume and the film's: the dialog repeats neither.
  expect(dialog.querySelector("input[type=range]")).toBeNull();
  expect(dialog.textContent).not.toContain("Dan's voice");
});

it("offers the voices' tap on the remote's page where they wait for one, not in the hidden panel", async () => {
  mounted = await mount(withServices(<Remote voice={voiceNoting([], true)} />, hosting()));
  const tap = buttonWithText(document.body, "Tap to hear everyone's voices");
  expect(tap).toBeDefined();
  expect(tap?.closest(".together-panel")).toBeNull();
});

it("offers Watch Together's button with no session to one who may start one, and nothing else, on the remote (D440 §6)", async () => {
  mounted = await mount(withServices(<Remote voice={undefined} canStart />));
  expect(buttonNamed(document.body, "Watch Together")).toBeDefined();
  expect(document.querySelector(".cast-remote .volume-row")).toBeNull();
  expect(document.querySelector(".cast-remote .together-panel")).toBeNull();
  mounted.unmount();
  mounted = await mount(withServices(<Remote voice={undefined} />));
  expect(buttonNamed(document.body, "Watch Together")).toBeUndefined();
});

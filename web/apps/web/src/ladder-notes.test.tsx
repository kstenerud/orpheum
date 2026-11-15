// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import type { CannotKeepUp } from "@orpheum/sdk";
import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { cannotKeepUpSaidForMs, LadderNotes } from "./film-player.tsx";
import { politeNotices } from "./test-dom.tsx";

// The owner, 2026-11-15: "Actually let's make it 10 seconds".
const ownersSeconds = 10;

const offer = {
  url: "/api/v1/bytes/media/chunks/t/master.m3u8",
  startAtBps: 1_896_000,
  qualityKbps: 1200,
  height: 480,
  isBelowTheLowest: false,
  soundCarriedGainDb: 0,
  isReady: true,
  filmPictureKbps: 4000,
  linkSharePercent: 70,
  steadiness: 0,
};

const stalled: CannotKeepUp = { linkKbps: 400, lowestKbps: 1200 };

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
  vi.useFakeTimers();
  Reflect.set(globalThis, "IS_REACT_ACT_ENVIRONMENT", true);
  container = document.createElement("div");
  document.body.append(container);
  root = createRoot(container);
});

afterEach(() => {
  act(() => {
    root.unmount();
  });
  container.remove();
  vi.useRealTimers();
});

function draw(props: { readonly url?: string; readonly cannotKeepUp?: CannotKeepUp }): void {
  act(() => {
    root.render(
      <LadderNotes
        offer={{ ...offer, url: props.url ?? offer.url }}
        playing={undefined}
        cannotKeepUp={props.cannotKeepUp}
      />,
    );
  });
}

function wait(ms: number): void {
  act(() => {
    vi.advanceTimersByTime(ms);
  });
}

const said = (): string => politeNotices(container).join(" ");

describe("word that even the lowest quality cannot keep up", () => {
  it("waits the owner's ten seconds", () => {
    expect(cannotKeepUpSaidForMs).toBe(ownersSeconds * 1000);
  });

  it("shows for ten seconds at the first stall and never again in that playback", () => {
    draw({});
    expect(said()).toBe("");
    draw({ cannotKeepUp: stalled });
    expect(said()).toContain("it will keep pausing");
    wait(cannotKeepUpSaidForMs - 1);
    expect(said()).toContain("it will keep pausing");
    wait(1);
    expect(said()).toBe("");
    // A later stall in the same playback, with other numbers, says nothing.
    draw({ cannotKeepUp: { linkKbps: 300, lowestKbps: 1200 } });
    wait(cannotKeepUpSaidForMs);
    expect(said()).toBe("");
  });

  it("goes after ten seconds when the server said so at the start, and shows again in a new playback", () => {
    act(() => {
      root.render(
        <LadderNotes
          offer={{ ...offer, isBelowTheLowest: true }}
          playing={undefined}
          cannotKeepUp={undefined}
        />,
      );
    });
    expect(said()).toContain("it will keep pausing");
    wait(cannotKeepUpSaidForMs);
    expect(said()).toBe("");
    draw({ url: "/api/v1/bytes/media/chunks/u/master.m3u8", cannotKeepUp: stalled });
    expect(said()).toContain("it will keep pausing");
  });
});

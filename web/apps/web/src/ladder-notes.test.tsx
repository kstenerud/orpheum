// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { createRoot, type Root } from "react-dom/client";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { LadderNotes } from "./film-player.tsx";
import { politeNotices } from "./test-dom.tsx";

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

let container: HTMLDivElement;
let root: Root;

beforeEach(() => {
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
});

function draw(): void {
  act(() => {
    root.render(<LadderNotes offer={{ ...offer, isBelowTheLowest: true }} playing={undefined} />);
  });
}

const said = (): string => politeNotices(container).join(" ");

// The owner, 2026-11-17: the dialog at the lowest quality (D506 §3) replaces the note.
describe("word that even the lowest quality cannot keep up", () => {
  it("is no note under the film, the server saying the link is below the lowest: the dialog says it (D506 §5)", () => {
    draw();
    expect(said()).toBe("");
  });
});

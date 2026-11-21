// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act, type JSX } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { useFileWaits } from "./file-waits.ts";
import { InterestSaidEveryContext } from "./page-interest.ts";
import { mount, servedBy, settleUntil, unanswered, type Heard, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  vi.useRealTimers();
  delete (document as { visibilityState?: unknown }).visibilityState;
});

const everySeconds = 15;

function Waits(): JSX.Element {
  const waits = useFileWaits(3, true);
  return <p>{waits?.readThrough?.secondsLeft ?? "none"}</p>;
}

async function mountWaits(answers: Parameters<typeof servedBy>[0], heard: Heard[]): Promise<void> {
  mounted = await mount(
    withServices(
      <InterestSaidEveryContext value={everySeconds}>
        <Waits />
      </InterestSaidEveryContext>,
      servedBy(answers, heard),
    ),
  );
}

async function waitOn(ms: number): Promise<void> {
  await act(async () => {
    vi.advanceTimersByTime(ms);
    await Promise.resolve();
  });
}

const waitsReads = (heard: readonly Heard[]): number =>
  heard.filter((each) => each.methodAndPath === "GET /files/3/waits").length;

const answer = [200, { readThrough: { isRunning: false, secondsLeft: 720 } }] as const;

describe("a file's waits, asked again while its page shows (D324 §6, D237)", () => {
  it("asks at once and again each interestSaidEverySeconds, one ask at a time", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const heard: Heard[] = [];
    await mountWaits({ "GET /files/3/waits": [answer, unanswered] }, heard);
    await settleUntil(() => mounted?.container.textContent === "720");
    expect(waitsReads(heard)).toBe(1);
    await waitOn(everySeconds * 1000);
    await settleUntil(() => waitsReads(heard) === 2);
    // The second ask is still out: the next time due asks nothing beside it.
    await waitOn(everySeconds * 1000);
    await waitOn(everySeconds * 1000);
    expect(waitsReads(heard)).toBe(2);
    expect(mounted?.container.textContent).toBe("720");
  });

  it("asks nothing while the page is hidden", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    let visibility: DocumentVisibilityState = "visible";
    Object.defineProperty(document, "visibilityState", {
      configurable: true,
      get: () => visibility,
    });
    const heard: Heard[] = [];
    await mountWaits({ "GET /files/3/waits": answer }, heard);
    await settleUntil(() => waitsReads(heard) === 1);
    visibility = "hidden";
    await waitOn(everySeconds * 1000);
    await waitOn(everySeconds * 1000);
    expect(waitsReads(heard)).toBe(1);
    visibility = "visible";
    await act(async () => {
      document.dispatchEvent(new Event("visibilitychange"));
      await Promise.resolve();
    });
    await settleUntil(() => waitsReads(heard) === 2);
    expect(waitsReads(heard)).toBe(2);
  });

  it("keeps the estimate it has through a read that fails", async () => {
    vi.useFakeTimers({ toFake: ["setInterval", "clearInterval"] });
    const heard: Heard[] = [];
    const failed = [500, { error: { code: "internal", message: "the server failed" } }] as const;
    await mountWaits({ "GET /files/3/waits": [answer, failed] }, heard);
    await settleUntil(() => mounted?.container.textContent === "720");
    await waitOn(everySeconds * 1000);
    await settleUntil(() => waitsReads(heard) === 2);
    await act(async () => {
      await Promise.resolve();
    });
    expect(mounted?.container.textContent).toBe("720");
  });
});

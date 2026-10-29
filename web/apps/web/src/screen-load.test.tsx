// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { sendFailuresTo } from "@orpheum/sdk";
import type { JSX } from "react";
import { afterEach, expect, it, vi } from "vitest";

import {
  LaterScreen,
  lazyScreen,
  loadScreenCode,
  ScreenCodeNotLoaded,
  ScreenNotLoadedPage,
  screenNotLoadedText,
  workerAnswerOf,
} from "./screen-load.tsx";
import { buttonWithText, mount, settleUntil, type Mounted } from "./test-dom.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
  sendFailuresTo(() => undefined);
  vi.restoreAllMocks();
});

function AScreen(): JSX.Element {
  return <p>the screen</p>;
}

const notFetched = new TypeError(
  "Failed to fetch dynamically imported module: https://c.example/assets/play-BOYzvUh7.js",
);

it("shows a screen whose code failed once and loaded when asked again", async () => {
  const load = vi
    .fn<() => Promise<{ default: typeof AScreen }>>()
    .mockRejectedValueOnce(notFetched)
    .mockResolvedValueOnce({ default: AScreen });
  const Lazy = lazyScreen("play", load, 0);
  mounted = await mount(
    <LaterScreen resetKey="play">
      <Lazy />
    </LaterScreen>,
  );
  const { container } = mounted;
  await settleUntil(() => container.textContent.includes("the screen"));
  expect(container.textContent).toContain("the screen");
  expect(load).toHaveBeenCalledTimes(2);
});

it("says a page whose screen's code did not load twice could not load, with Reload, and tells the core which and why (the owner's Chrome, 2026-10-29)", async () => {
  const sent: string[] = [];
  sendFailuresTo((text) => sent.push(text));
  vi.spyOn(console, "error").mockImplementation(() => undefined);
  const load = vi.fn<() => Promise<{ default: typeof AScreen }>>().mockRejectedValue(notFetched);
  const Lazy = lazyScreen("play", load, 0);
  mounted = await mount(
    <LaterScreen resetKey="play">
      <Lazy />
    </LaterScreen>,
  );
  const { container } = mounted;
  await settleUntil(() => container.querySelector('[role="alert"]') !== null);
  expect(container.querySelector('[role="alert"]')?.textContent).toBe("This page could not load.");
  expect(buttonWithText(container, "Reload")).toBeDefined();
  expect(buttonWithText(container, "Copy what happened")).toBeDefined();
  expect(container.querySelector(".what-happened pre")?.textContent).toContain("Screen: play");
  expect(load).toHaveBeenCalledTimes(2);
  expect(sent).toHaveLength(1);
  expect(sent[0]).toContain("the code of the play screen did not load, asked twice");
  expect(sent[0]).toContain("/assets/play-BOYzvUh7.js");
  expect(sent[0]).toContain("a service worker controls the page: no");
});

it("reloads the page from its Reload", async () => {
  const reload = vi.fn();
  mounted = await mount(
    <ScreenNotLoadedPage
      text="what happened"
      worker={{ isUnderLoader: false, isControlled: true }}
      reload={reload}
    />,
  );
  buttonWithText(mounted.container, "Reload")?.click();
  expect(reload).toHaveBeenCalledTimes(1);
});

it("says no worker answers the page under a loader, as DevTools' bypass makes it, and only there", async () => {
  mounted = await mount(
    <ScreenNotLoadedPage
      text="what happened"
      worker={{ isUnderLoader: true, isControlled: false }}
      reload={() => undefined}
    />,
  );
  expect(mounted.container.textContent).toContain("bypass the service worker");
  mounted.unmount();

  mounted = await mount(
    <ScreenNotLoadedPage
      text="what happened"
      worker={{ isUnderLoader: true, isControlled: true }}
      reload={() => undefined}
    />,
  );
  expect(mounted.container.textContent).not.toContain("bypass the service worker");
});

it("tells the core's log where the time went when a screen's code is slow, though it loads (VQ137)", async () => {
  vi.useFakeTimers({ toFake: ["performance", "setTimeout", "clearTimeout"] });
  const sent: string[] = [];
  sendFailuresTo((text) => sent.push(text));
  const load = (): Promise<{ default: typeof AScreen }> =>
    new Promise((resolve) => {
      setTimeout(() => {
        resolve({ default: AScreen });
      }, 3500);
    });
  const resultPromise = loadScreenCode("play", load, 0);
  await vi.advanceTimersByTimeAsync(3500);
  const result = await resultPromise;
  expect(result.default).toBe(AScreen);
  expect(sent).toHaveLength(1);
  expect(sent[0]).toContain("the play screen's code took 3500 ms to load");
  expect(sent[0]).toContain("attempt 1: 3500 ms (ok)");
});

it("tells the core's log nothing for an ordinary, fast load", async () => {
  const sent: string[] = [];
  sendFailuresTo((text) => sent.push(text));
  const result = await loadScreenCode(
    "play",
    async () => await Promise.resolve({ default: AScreen }),
  );
  expect(result.default).toBe(AScreen);
  expect(sent).toEqual([]);
});

it("says how a loader's worker answered a screen's chunk, from the browser's own record of its load", () => {
  const kept = {
    name: "https://c.example/assets/title-ABC123.js",
    startTime: 100,
    workerStart: 50,
    transferSize: 0,
  } as unknown as PerformanceEntry;
  expect(workerAnswerOf("title", 0, [kept])).toBe("the service worker answered from what it kept");

  const fetched = {
    name: "https://c.example/assets/title-ABC123.js",
    startTime: 100,
    workerStart: 50,
    transferSize: 4096,
  } as unknown as PerformanceEntry;
  expect(workerAnswerOf("title", 0, [fetched])).toBe(
    "the service worker answered, 4096 bytes over the network",
  );

  const notIntercepted = {
    name: "https://c.example/assets/title-ABC123.js",
    startTime: 100,
    workerStart: 0,
    transferSize: 0,
  } as unknown as PerformanceEntry;
  expect(workerAnswerOf("title", 0, [notIntercepted])).toBeUndefined();

  // Loaded before the screen asked for it: not this load's.
  expect(workerAnswerOf("title", 200, [kept])).toBeUndefined();

  // Another screen's chunk, which this one's name does not match.
  const another = {
    name: "https://c.example/assets/series-XYZ789.js",
    startTime: 100,
    workerStart: 50,
    transferSize: 0,
  } as unknown as PerformanceEntry;
  expect(workerAnswerOf("title", 0, [another])).toBeUndefined();
});

it("says what happened: the screen, the page, the reason, the loader, the worker and the browser", () => {
  const text = screenNotLoadedText(
    {
      failed: new ScreenCodeNotLoaded("title", notFetched),
      worker: { isUnderLoader: true, isControlled: false },
      page: "/titles/5285",
    },
    "Mozilla/5.0 Chrome/156",
  );
  expect(text).toContain("Screen: title");
  expect(text).toContain("Page: /titles/5285");
  expect(text).toContain("What failed: Failed to fetch dynamically imported module");
  expect(text).toContain("Under the discovery's loader: yes");
  expect(text).toContain("Service worker controlling the page: no");
  expect(text).toContain("Browser: Mozilla/5.0 Chrome/156");
});

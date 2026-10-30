// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { sendFailuresTo } from "@orpheum/sdk";
import { afterEach, describe, expect, it, vi } from "vitest";

import { mount, settleAnswers, type Mounted } from "./test-dom.tsx";
import { probeWorker, WorkerBypassedNote } from "./worker-bypassed-note.tsx";

let mounted: Mounted | undefined;

afterEach(() => {
  mounted?.unmount();
  mounted = undefined;
});

// With DevTools' "Bypass for network" ticked, a client origin's posters and films 404 and nothing
// says so (idiosyncrasies.md, the Bypass entry; agent-failures A51).
describe("WorkerBypassedNote", () => {
  it("says developer tools bypass the worker when its probe says so", async () => {
    mounted = await mount(<WorkerBypassedNote probe={() => Promise.resolve(true)} />);
    await settleAnswers();
    expect(mounted.container.textContent).toBe(
      "Developer tools are bypassing the service worker; media won't load.",
    );
  });

  it("says nothing while the worker answers", async () => {
    mounted = await mount(<WorkerBypassedNote probe={() => Promise.resolve(false)} />);
    await settleAnswers();
    expect(mounted.container.textContent).toBe("");
  });
});

describe("probeWorker", () => {
  it("asks, under a loader of a page a worker controls, and tells the core's log of a bypass", async () => {
    const sent: string[] = [];
    sendFailuresTo((text) => {
      sent.push(text);
    });
    vi.spyOn(console, "error").mockImplementation(() => undefined);
    const isBypassed = await probeWorker({ isUnderLoader: true, isControlled: true }, () =>
      Promise.resolve(true),
    );
    expect(isBypassed).toBe(true);
    expect(sent.filter((each) => each.includes("bypass the page's service worker"))).toHaveLength(
      1,
    );
  });

  it("asks nothing at a core's own origin, nor of a page no worker controls", async () => {
    const probe = vi.fn(() => Promise.resolve(true));
    expect(await probeWorker({ isUnderLoader: false, isControlled: true }, probe)).toBe(false);
    expect(await probeWorker({ isUnderLoader: true, isControlled: false }, probe)).toBe(false);
    expect(probe).not.toHaveBeenCalled();
  });
});

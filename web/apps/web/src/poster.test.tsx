// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { BackgroundChangesContext, type Ask, type Poller } from "./polling.ts";
import { Poster, Still } from "./poster.tsx";
import { mount, type Mounted } from "./test-dom.tsx";

describe("a poster's or a still's image that does not load (the owner, Fire TV, 2026-10-25)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.unstubAllGlobals();
  });

  it("shows the poster's named blank, not a broken image", async () => {
    mounted = await mount(<Poster sourceSha256="ab12" />);
    act(() => {
      mounted?.container.querySelector("img")?.dispatchEvent(new Event("error"));
    });
    expect(mounted.container.querySelector("img")).toBeNull();
    expect(mounted.container.querySelector(".poster--blank")).not.toBeNull();
  });

  it("shows a still's blank of its size, not a broken image", async () => {
    mounted = await mount(<Still sourceSha256="cd34" className="series__still" />);
    act(() => {
      mounted?.container.querySelector("img")?.dispatchEvent(new Event("error"));
    });
    expect(mounted.container.querySelector("img")).toBeNull();
    expect(mounted.container.querySelector("span.series__still")).not.toBeNull();
  });

  it("asks for an image that did not load again, and shows it once it loads", async () => {
    // Art made after the page was drawn kept its blank until a reload (the owner, 2026-10-25).
    const probes = stubImages();
    const poll = handPoller();
    mounted = await mount(
      <BackgroundChangesContext value={poll.poller}>
        <Poster sourceSha256="ef56" />
      </BackgroundChangesContext>,
    );
    act(() => {
      mounted?.container.querySelector("img")?.dispatchEvent(new Event("error"));
    });
    expect(poll.started()).toBe(1);
    const outcomes: string[] = [];
    act(() => {
      poll.ask((outcome) => outcomes.push(outcome));
    });
    expect(probes.at(-1)?.src).toBe("/api/v1/bytes/art/ef56/portrait-m.webp?again=1");
    await act(async () => {
      probes.at(-1)?.answer(false);
      await Promise.resolve();
    });
    // Still not made: the blank stays, and the poll is told so, to back off.
    expect(mounted.container.querySelector("img")).toBeNull();
    expect(outcomes).toEqual(["unchanged"]);
    act(() => {
      poll.ask((outcome) => outcomes.push(outcome));
    });
    await act(async () => {
      probes.at(-1)?.answer(true);
      await Promise.resolve();
    });
    expect(mounted.container.querySelector("img")?.getAttribute("src")).toBe(
      "/api/v1/bytes/art/ef56/portrait-m.webp?again=2",
    );
    expect(outcomes).toEqual(["unchanged", "changed"]);
    expect(poll.stopped()).toBe(1);
  });

  it("asks for a still that did not load again, and stops asking when it leaves the page", async () => {
    const probes = stubImages();
    const poll = handPoller();
    mounted = await mount(
      <BackgroundChangesContext value={poll.poller}>
        <Still sourceSha256="ab90" className="series__still" />
      </BackgroundChangesContext>,
    );
    act(() => {
      mounted?.container.querySelector("img")?.dispatchEvent(new Event("error"));
    });
    act(() => {
      poll.ask(() => undefined);
    });
    expect(probes.at(-1)?.src).toBe("/api/v1/bytes/art/ab90/landscape-s.webp?again=1");
    mounted.unmount();
    mounted = undefined;
    expect(poll.stopped()).toBe(1);
  });
});

/** A stand-in for the browser's Image, each one made kept to be answered by the test. */
interface Probe {
  readonly src: string;
  readonly answer: (isLoaded: boolean) => void;
}

function stubImages(): Probe[] {
  const probes: Probe[] = [];
  class StandInImage {
    onload: (() => void) | null = null;
    onerror: (() => void) | null = null;
    set src(url: string) {
      if (url === "") {
        return;
      }
      probes.push({
        src: url,
        answer: (isLoaded) => {
          (isLoaded ? this.onload : this.onerror)?.();
        },
      });
    }
  }
  vi.stubGlobal("Image", StandInImage);
  return probes;
}

/** A poller the test asks by hand, counting its starts and stops. */
function handPoller(): {
  readonly poller: Poller;
  readonly ask: (answered: Parameters<Ask>[0]) => void;
  readonly started: () => number;
  readonly stopped: () => number;
} {
  let current: Ask | undefined;
  let starts = 0;
  let stops = 0;
  return {
    poller: {
      start: (ask) => {
        current = ask;
        starts += 1;
        return () => {
          stops += 1;
        };
      },
    },
    ask: (answered) => {
      current?.(answered);
    },
    started: () => starts,
    stopped: () => stops,
  };
}

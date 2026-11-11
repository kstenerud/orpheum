// @vitest-environment happy-dom
// SPDX-License-Identifier: MPL-2.0

import "./i18n.ts";

import { act } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { BackgroundChangesContext, type Ask, type Poller } from "./polling.ts";
import { PosterCard } from "./poster-card.tsx";
import { Poster, posterTileSizes, Still, WideArt } from "./poster.tsx";
import { mount, servedBy, type Mounted } from "./test-dom.tsx";
import { withServices } from "./test-services.tsx";

describe("a film or series with no poster but a still (the owner, 2026-10-27; D362 §2)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
  });

  it("shows the still in the top of the poster's blank, the blank's mark and its name below it", async () => {
    mounted = await mount(<Poster sourceSha256={undefined} stillSha256="5711" />);
    const blank = mounted.container.querySelector(".poster--blank.poster--with-still");
    const still = blank?.querySelector("img.poster__still");
    expect(still?.getAttribute("src")).toBe("/api/v1/bytes/art/5711/landscape-s.webp");
    expect(blank?.firstElementChild).toBe(still);
    expect(blank?.querySelector(".poster__clapperboard")).not.toBeNull();
    expect(blank?.querySelector(".poster__caption")?.textContent).toBe("No poster");
  });

  it("shows the poster alone once it has one", async () => {
    mounted = await mount(<Poster sourceSha256="ab12" stillSha256="5711" />);
    const images = mounted.container.querySelectorAll("img");
    expect(images).toHaveLength(1);
    expect(images[0]?.getAttribute("src")).toBe("/api/v1/bytes/art/ab12/portrait-m.webp");
    expect(mounted.container.querySelector(".poster--blank")).toBeNull();
  });

  it("shows the plain blank where it has no still either", async () => {
    mounted = await mount(<Poster sourceSha256={undefined} />);
    expect(
      mounted.container.querySelector(".poster--blank:not(.poster--with-still)"),
    ).not.toBeNull();
    expect(mounted.container.querySelector("img")).toBeNull();
  });
});

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

  it("shows nothing for a title page's backdrop not made yet, and shows it once made", async () => {
    // The title page's banner showed as a broken image (the owner, 2026-10-26).
    const probes = stubImages();
    const poll = handPoller();
    mounted = await mount(
      <BackgroundChangesContext value={poll.poller}>
        <WideArt sourceSha256="aa11" size="landscape-l" className="title__backdrop" />
      </BackgroundChangesContext>,
    );
    expect(mounted.container.querySelector("img")?.getAttribute("src")).toBe(
      "/api/v1/bytes/art/aa11/landscape-l.webp",
    );
    act(() => {
      mounted?.container.querySelector("img")?.dispatchEvent(new Event("error"));
    });
    expect(mounted.container.innerHTML).toBe("");
    act(() => {
      poll.ask(() => undefined);
    });
    await act(async () => {
      probes.at(-1)?.answer(true);
      await Promise.resolve();
    });
    expect(mounted.container.querySelector("img.title__backdrop")?.getAttribute("src")).toBe(
      "/api/v1/bytes/art/aa11/landscape-l.webp?again=1",
    );
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

describe("a poster tile's image, at the size it is drawn (web architecture audit, 2026-11-10)", () => {
  let mounted: Mounted | undefined;
  afterEach(() => {
    mounted?.unmount();
    mounted = undefined;
    vi.unstubAllGlobals();
  });

  it("offers the small and the middle size for the tile's width, decoded off the main thread", async () => {
    mounted = await mount(<Poster sourceSha256="ab12" isTile />);
    const image = mounted.container.querySelector("img.poster");
    expect(image?.getAttribute("srcset")).toBe(
      "/api/v1/bytes/art/ab12/portrait-s.webp 240w, /api/v1/bytes/art/ab12/portrait-m.webp 480w",
    );
    expect(image?.getAttribute("sizes")).toBe(posterTileSizes);
    expect(image?.getAttribute("src")).toBe("/api/v1/bytes/art/ab12/portrait-m.webp");
    expect(image?.getAttribute("decoding")).toBe("async");
  });

  it("is a card's: a poster card's tile is drawn at a tile's size", async () => {
    mounted = await mount(
      withServices(
        <ul>
          <PosterCard to={{ screen: "home" }} posterSha256="ab12" name="A film" />
        </ul>,
        servedBy({}, []),
      ),
    );
    expect(mounted.container.querySelector("img.poster")?.getAttribute("sizes")).toBe(
      posterTileSizes,
    );
  });

  it("offers a title page's poster in the middle size alone", async () => {
    mounted = await mount(<Poster sourceSha256="ab12" />);
    expect(mounted.container.querySelector("img.poster")?.hasAttribute("srcset")).toBe(false);
  });

  it("asks for each size again, by a new address, once a tile's image that did not load is made", async () => {
    const probes = stubImages();
    const poll = handPoller();
    mounted = await mount(
      <BackgroundChangesContext value={poll.poller}>
        <Poster sourceSha256="ef56" isTile />
      </BackgroundChangesContext>,
    );
    act(() => {
      mounted?.container.querySelector("img")?.dispatchEvent(new Event("error"));
    });
    act(() => {
      poll.ask(() => undefined);
    });
    await act(async () => {
      probes.at(-1)?.answer(true);
      await Promise.resolve();
    });
    expect(mounted.container.querySelector("img")?.getAttribute("srcset")).toBe(
      "/api/v1/bytes/art/ef56/portrait-s.webp?again=1 240w, " +
        "/api/v1/bytes/art/ef56/portrait-m.webp?again=1 480w",
    );
  });

  it("shows a clock until its image loads, then the image alone, the same image kept", async () => {
    mounted = await mount(<Poster sourceSha256="ab12" isTile />);
    const image = mounted.container.querySelector("img.poster");
    expect(mounted.container.querySelector(".poster-waiting .poster__clock")).not.toBeNull();
    act(() => {
      image?.dispatchEvent(new Event("load"));
    });
    // The faded clock is its own layer, painted over the image: it goes (the owner, 2026-11-11).
    expect(mounted.container.querySelector(".poster-waiting")).toBeNull();
    expect(mounted.container.querySelector("img.poster")).toBe(image);
  });

  it("decodes a still and wide art off the main thread", async () => {
    mounted = await mount(
      <>
        <Still sourceSha256="ab90" className="series__still" />
        <WideArt sourceSha256="aa11" size="landscape-l" className="title__backdrop" />
      </>,
    );
    const images = [...mounted.container.querySelectorAll("img")];
    expect(images.map((image) => image.getAttribute("decoding"))).toEqual(["async", "async"]);
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

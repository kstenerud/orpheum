// SPDX-License-Identifier: MPL-2.0

// What is drawn over the film, laid out and driven as each device drives it (D538 §1; player-spec's
// three places; brief-common's browser tests): the notes at the bottom-left stack there without
// covering a control or each other, on a cast's receiver too; the sound-on Pill stands at the film's
// top centre and takes every press; and the offers in the top-right corner, Skip, Up next and Still
// watching, are reached and taken by a mouse, a finger, the keyboard and a remote; none has a ✕.

import "./tokens.css";
import "./base.css";
import "./i18n.ts";

import { act, createRef, type ComponentProps, type JSX, type ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { page, userEvent } from "vitest/browser";

import type { UpNext } from "./around-file.ts";
import { tabStorage } from "./browser-storage.ts";
import { CastReady } from "./cast-ready.tsx";
import { forgetReceiving, wasReceiving } from "./cast-receiving-mark.ts";
import { ControlledMode } from "./cast-receiver.tsx";
import { useTellsOffered, type OfferPiece } from "./convenience-area.tsx";
import { CornerNotes, type CornerNote } from "./corner-notes.tsx";
import { commandEventName, startInput } from "./input-commands.ts";
import { LiveProvider } from "./live.tsx";
import { OfferCard } from "./offer-card.tsx";
import { PlayUpwardContext } from "./play-up.tsx";
import { PlayerOverlay } from "./player-overlay.tsx";
import { SoundOnPill } from "./sound-on-pill.tsx";
import { StillWatching } from "./still-watching.tsx";
import { mount, pressToStartTheCast, servedBy, type Mounted } from "./test-dom.tsx";
import { focusedName, isRingShown, tapOn } from "./test-parts.ts";
import { withServices } from "./test-services.tsx";
import { liveAndSession } from "./test-together.ts";
import { UpNextCard } from "./up-next.tsx";
import type { Upward } from "./upward.ts";

/** An episode's place, for the overlay's way up. */
const episodeUpward: Upward = {
  crumbs: [{ label: "A series", to: { screen: "home" }, isIt: () => false }],
  here: "An episode",
};

/** A phone's screen upright, a computer's and a television's (D538's widths). */
const screens = [
  { width: 360, height: 740 },
  { width: 1280, height: 800 },
  { width: 1920, height: 1080 },
] as const;

/** Three notes, the most the corner shows, the last long enough to wrap on a phone. */
const threeNotes: readonly CornerNote[] = [
  { id: 0, text: "Sam joined" },
  { id: 1, text: "Kim and Alex left" },
  { id: 2, text: "Controlled from the living room's television upstairs" },
];

/** The next episode, as the server tells it. */
const nextEpisode = {
  episode: { itemId: 3, name: "Cat", groupNumber: 1, numberInGroup: 2, fileId: 5 },
  ordering: "aired",
} as unknown as UpNext;

let mounted: Mounted[] = [];

afterEach(() => {
  for (const each of mounted) {
    each.unmount();
  }
  mounted = [];
  vi.restoreAllMocks();
  // A cast received marks the tab (D383 §2), and a tab's storage is every test file's frame's: a
  // mark left would hold each later file's Play page in doubt, drawing nothing.
  forgetReceiving(tabStorage());
});

/** What was pressed, by the words of each press, in order. */
type Pressed = string[];

/**
 * Draws the overlay over a film filling a screen of the size given, the offer given in its corner and
 * over drawn on the film's stage with it, as the player portals its notes and prompts there.
 */
async function drawnOn(
  screen: { readonly width: number; readonly height: number },
  parts: { readonly offer?: OfferPiece; readonly over?: ReactNode } = {},
): Promise<HTMLVideoElement> {
  await page.viewport(screen.width, screen.height);
  const video = document.createElement("video");
  Object.defineProperty(video, "paused", { get: () => true });
  Object.defineProperty(video, "duration", { get: () => 600 });
  Object.defineProperty(video, "currentTime", { get: () => 100, set: () => undefined });
  video.style.display = "block";
  video.style.width = "100%";
  video.style.height = "100%";
  const videoRef = createRef<HTMLVideoElement>();
  Reflect.set(videoRef, "current", video);
  mounted.push(
    await mount(
      withServices(
        <PlayUpwardContext value={episodeUpward}>
          <div
            className="film-player__stage"
            style={{ position: "relative", width: "100vw", height: "100vh" }}
          >
            <PlayerOverlay
              videoRef={videoRef}
              readLinkKbps={() => undefined}
              failure={undefined}
              offer={parts.offer}
              moves={{ previous: () => undefined, next: () => undefined }}
            />
            {parts.over}
          </div>
        </PlayUpwardContext>,
      ),
    ),
  );
  overlay().parentElement?.prepend(video);
  return video;
}

function overlay(): HTMLElement {
  const found = document.querySelector<HTMLElement>(".player-overlay");
  if (found === null) {
    throw new Error("no overlay drawn");
  }
  return found;
}

function stage(): HTMLElement {
  const found = document.querySelector<HTMLElement>(".film-player__stage");
  if (found === null) {
    throw new Error("no stage drawn");
  }
  return found;
}

/** Shows all of the overlay as a mouse moved over the page does, then waits out its shield. */
async function shown(): Promise<void> {
  for (const screenX of [5, 50]) {
    act(() => {
      document.body.dispatchEvent(
        new PointerEvent("pointermove", {
          bubbles: true,
          pointerType: "mouse",
          screenX,
          screenY: 5,
        }),
      );
    });
  }
  await expect.poll(() => overlay().getAttribute("data-shown")).toBe("full");
  await new Promise((done) => setTimeout(done, 600));
}

/** Hides the overlay as Back does (D407 §5). */
async function hidden(): Promise<void> {
  act(() => {
    document.body.dispatchEvent(
      new CustomEvent(commandEventName, {
        detail: { command: "back", source: "key" },
        bubbles: true,
        cancelable: true,
      }),
    );
  });
  await expect.poll(() => overlay().getAttribute("data-shown")).not.toBe("full");
}

/** Makes the device's pointer a finger, as a phone's is, for what asks (pointer: coarse). */
function heldByAFinger(): void {
  const own = window.matchMedia.bind(window);
  vi.spyOn(window, "matchMedia").mockImplementation((query) => {
    const list = own(query);
    if (query === "(pointer: coarse)") {
      Object.defineProperty(list, "matches", { get: () => true });
    }
    return list;
  });
}

/** Whether two boxes share any of the screen, more than a rounding's half pixel. */
function isOverlapping(one: DOMRect, other: DOMRect): boolean {
  const across = Math.min(one.right, other.right) - Math.max(one.left, other.left);
  const down = Math.min(one.bottom, other.bottom) - Math.max(one.top, other.top);
  return across > 0.5 && down > 0.5;
}

/** Each control drawn and seen in container, outside the notes, by its name and box. */
function controlsIn(container: Element): { readonly name: string; readonly box: DOMRect }[] {
  return [...container.querySelectorAll<HTMLElement>("button, a[href], input")]
    .filter((each) => each.closest(".corner-notes") === null)
    .filter((each) => getComputedStyle(each).visibility === "visible")
    .map((each) => ({
      name: each.getAttribute("aria-label") ?? each.textContent,
      box: rectOf(each),
    }))
    .filter(({ box }) => box.width > 0 && box.height > 0);
}

/** An element's box as drawn. */
function rectOf(element: Element): DOMRect {
  return element.getBoundingClientRect();
}

/** The corner's notes as drawn, top first. */
function noteBoxes(): DOMRect[] {
  return [...document.querySelectorAll(".corner-notes .pill")].map((each) =>
    each.getBoundingClientRect(),
  );
}

/** The controls a note covers, by name, and whether any two notes cover each other. */
function coveredBy(notes: readonly DOMRect[], container: Element): string[] {
  const covered = controlsIn(container)
    .filter(({ box }) => notes.some((note) => isOverlapping(note, box)))
    .map(({ name }) => name);
  const isStacked = notes.every((note, at) => at === 0 || note.top >= notes[at - 1]!.bottom - 0.5);
  return isStacked ? covered : [...covered, "another note"];
}

/** Expects the notes in the corner, each at the screen's left edge's room, newest lowest. */
function expectsTheCorner(notes: readonly DOMRect[]): void {
  expect(notes).toHaveLength(3);
  for (const note of notes) {
    expect(Math.round(note.left)).toBe(8); // --screen-edge-room
    expect(note.right).toBeLessThanOrEqual(window.innerWidth);
  }
  expect(document.querySelector(".corner-notes")?.lastElementChild?.textContent).toBe(
    threeNotes[2]!.text,
  );
}

describe("the notes at the film's bottom-left (D538 §1, player-spec's three places)", () => {
  for (const screen of screens) {
    const at = `${String(screen.width)}×${String(screen.height)}`;

    it(`stack above the overlay's rows as they show and at the film's foot as they hide, covering nothing, at ${at}`, async () => {
      if (screen.width < 600) {
        heldByAFinger();
      }
      await drawnOn(screen, { over: <CornerNotes notes={threeNotes} onGone={() => undefined} /> });
      await shown();
      await expect.poll(() => stage().style.getPropertyValue("--player-overlay-lift")).not.toBe("");
      await expect.poll(() => coveredBy(noteBoxes(), overlay())).toEqual([]);
      expectsTheCorner(noteBoxes());
      const rows = controlsIn(overlay()).map(({ box }) => box.top);
      expect(Math.max(...noteBoxes().map((note) => note.bottom))).toBeLessThanOrEqual(
        Math.min(...rows.filter((top) => top > window.innerHeight / 2)),
      );
      await hidden();
      await expect.poll(() => Math.round(window.innerHeight - noteBoxes().at(-1)!.bottom)).toBe(8); // the film's foot, past the safe area
      expect(coveredBy(noteBoxes(), overlay())).toEqual([]);
      expectsTheCorner(noteBoxes());
    });

    it(`stack on a cast's receiver as its film plays, over the overlay's rows, covering nothing, at ${at}`, async () => {
      const video = await drawnOn(screen);
      mounted.push(
        await mount(withServices(<ControlledMode fromName="the living room's TV" video={video} />)),
      );
      // Back stays the receiver's own (useBackStaysHere), so the overlay is looked at hidden first,
      // as it opens, then shown.
      await expect
        .poll(() => stage().querySelector(".corner-notes .pill")?.textContent)
        .toBe("Controlled from the living room's TV");
      expect(overlay().getAttribute("data-shown")).not.toBe("full");
      expect(coveredBy(noteBoxes(), overlay())).toEqual([]);
      expect(Math.round(window.innerHeight - noteBoxes()[0]!.bottom)).toBe(8);
      await shown();
      await expect.poll(() => coveredBy(noteBoxes(), overlay())).toEqual([]);
      expect(Math.round(noteBoxes()[0]!.left)).toBe(8);
    });
  }
  // One mount for every width: the receiver's start press is asked once a page (pressToStartTheCast).
  it("stand clear of a cast's receiver's ready screen and its Full screen, at 360, 1280 and 1920 wide", async () => {
    await page.viewport(screens[0].width, screens[0].height);
    const sockets = liveAndSession();
    const services = servedBy({ "POST /me/casts/5/accept": [200, { castId: 5 }] }, [], "token");
    mounted.push(
      await mount(
        withServices(
          <LiveProvider>
            <CastReady />
          </LiveProvider>,
          { ...services, openTogetherSocket: sockets.openSocket },
        ),
      ),
    );
    await sockets.live.accept();
    await sockets.live.say({
      type: "receive",
      receive: {
        castId: 5,
        membershipCredential: "cast",
        fileId: 0,
        fromDeviceId: 2,
        fromDeviceName: "the living room's television upstairs",
      },
    });
    await pressToStartTheCast();
    for (const screen of screens) {
      await page.viewport(screen.width, screen.height);
      await expect.poll(() => noteBoxes()).toHaveLength(1);
      const ready = document.querySelector(".cast-ready");
      expect(controlsIn(ready!).map(({ name }) => name)).toEqual(
        expect.arrayContaining(["Stop casting here", "Full screen"]),
      );
      expect(coveredBy(noteBoxes(), ready!)).toEqual([]);
      expect(Math.round(noteBoxes()[0]!.left)).toBe(8);
    }
  });
  // The test before this one received a cast, marking the tab; the mark is gone with it.
  it("leave no cast marked as received for the test files after them", () => {
    expect(wasReceiving(tabStorage())).toBe(false);
  });
});

/** A film's stage drawn with the sound-on Pill on it, each of its presses written down. */
async function soundOnDrawn(screen: (typeof screens)[number]): Promise<Pressed> {
  const pressed: Pressed = [];
  await drawnOn(screen, {
    over: (
      <SoundOnPill
        onPress={() => {
          pressed.push("sound on");
        }}
      />
    ),
  });
  return pressed;
}

function soundOn(): HTMLButtonElement {
  const found = document.querySelector<HTMLButtonElement>(".sound-on-pill button");
  if (found === null) {
    throw new Error("no sound-on Pill drawn");
  }
  return found;
}

describe("the sound-on Pill (D538 §1, player-spec's Turning the sound on)", () => {
  for (const screen of screens) {
    const at = `${String(screen.width)}×${String(screen.height)}`;

    it(`stands at the film's top centre, a fingertip tall, over the overlay's rows, at ${at}`, async () => {
      await soundOnDrawn(screen);
      await shown();
      const box = soundOn().getBoundingClientRect();
      expect(Math.abs((box.left + box.right) / 2 - window.innerWidth / 2)).toBeLessThan(2);
      // On a phone under the overlay's top row, whose way up it covered (the owner, 2026-11-24); else
      // at --space-4.
      expect(Math.round(box.top)).toBe(isPhoneLaidOut() ? topRowDropPx() + 8 : 16);
      expect(Math.round(box.height)).toBeGreaterThanOrEqual(44);
      const pill = document.elementFromPoint(
        (box.left + box.right) / 2,
        (box.top + box.bottom) / 2,
      );
      expect(pill?.closest(".sound-on-pill button")).toBe(soundOn());
    });

    it(`takes a mouse's press, a finger's tap, the keyboard's Enter and a remote's OK, at ${at}`, async () => {
      const pressed = await soundOnDrawn(screen);
      await userEvent.click(soundOn());
      tapOn(soundOn());
      expect(pressed).toEqual(["sound on", "sound on"]);
      soundOn().blur();
      await shown();
      let tabs = 0;
      while (document.activeElement !== soundOn() && tabs < 24) {
        await userEvent.keyboard("{Tab}");
        tabs += 1;
      }
      expect(isRingShown()).toBe(true);
      await userEvent.keyboard("{Enter}");
      expect(pressed).toHaveLength(3);
      // On a phone, under the top row, the Pill is a finger's and Tab's alone: arrows search the
      // overlay's own buttons first, and a phone's layout is touch's (the owner, 2026-11-24).
      if (isPhoneLaidOut()) {
        return;
      }
      const input = startInput({ goBack: () => false, isNavigating: () => true });
      try {
        overlay().querySelector<HTMLElement>(".player-overlay__up")?.focus();
        await reachedBy(["{ArrowRight}", "{ArrowUp}"], () => document.activeElement === soundOn());
        expect(isRingShown()).toBe(true);
        await userEvent.keyboard("{Enter}");
        expect(pressed).toHaveLength(4);
      } finally {
        input.stop();
      }
    });
  }
});

/** Whether the overlay drawn lays itself out as a phone's (overlay-measure.ts). */
function isPhoneLaidOut(): boolean {
  return overlay().getAttribute("data-layout") === "phone";
}

/** How far the overlay's top row reaches down the stage, as overlay-measure.ts sets it, in px. */
function topRowDropPx(): number {
  const stage = overlay().parentElement;
  const dropPx = Number.parseFloat(stage?.style.getPropertyValue("--player-overlay-drop") ?? "");
  return Number.isNaN(dropPx) ? 0 : dropPx;
}

/** Presses each key in turn, at most six times each, until isThere holds; expects it does. */
async function reachedBy(keys: readonly string[], isThere: () => boolean): Promise<void> {
  const walked = [focusedName()];
  for (const key of keys) {
    for (let step = 0; step < 6 && !isThere(); step += 1) {
      await userEvent.keyboard(key);
      expect(document.activeElement).not.toBe(document.body);
      walked.push(`${key} ${String(focusedName())}`);
    }
  }
  expect(isThere(), walked.join(", ")).toBe(true);
}

/** An offer the overlay draws in its corner, telling it so, as the player's own do. */
function offerOf(card: (onOffered: OfferPieceProps["onOffered"]) => JSX.Element): OfferPiece {
  return function Offered(props: OfferPieceProps): JSX.Element {
    useTellsOffered(true, props.onOffered);
    return card(props.onOffered);
  };
}

type OfferPieceProps = ComponentProps<OfferPiece>;

/** Each offer, by the button that takes it. */
interface Offered {
  readonly name: string;
  readonly piece: (pressed: Pressed) => OfferPiece;
  readonly take: string;
}

const offers: readonly Offered[] = [
  {
    name: "Skip intro",
    take: "Skip intro",
    piece: (pressed) =>
      offerOf(() => (
        <OfferCard
          words="Skip intro"
          onTake={() => {
            pressed.push("Skip intro");
          }}
        />
      )),
  },
  {
    name: "Up next, counting",
    take: "Up next",
    piece: (pressed) =>
      offerOf(() => <UpNextCard upNext={nextEpisode} secondsLeft={8} play={playing(pressed)} />),
  },
  {
    name: "the remote's Up next, with no count",
    take: "Up next",
    piece: (pressed) => offerOf(() => <UpNextCard upNext={nextEpisode} play={playing(pressed)} />),
  },
  {
    name: "Still watching",
    take: "Continue",
    piece: (pressed) =>
      offerOf(() => (
        <StillWatching
          onContinue={() => {
            pressed.push("Continue");
          }}
          onStop={() => {
            pressed.push("Stop");
          }}
        />
      )),
  },
];

/** Up next's press, written down. */
function playing(pressed: Pressed): Parameters<typeof UpNextCard>[0]["play"] {
  return {
    onPlay: () => {
      pressed.push("Up next");
    },
  };
}

/** The button of the offer in the corner whose name or words are name. */
function offerButton(name: string): HTMLElement {
  const found = [
    ...document.querySelectorAll<HTMLElement>(".convenience-area .offer-card button"),
  ].find((each) =>
    // A take button's words follow its still.
    (each.querySelector(".offer-card__offer")?.textContent ?? each.textContent).startsWith(name),
  );
  if (found === undefined) {
    throw new Error(`no ${name} in the corner`);
  }
  return found;
}

describe("the offers in the film's top-right corner (D538 §1, player-spec's OfferCard)", () => {
  for (const screen of screens) {
    const at = `${String(screen.width)}×${String(screen.height)}`;
    for (const offer of offers) {
      const named = [offer.take];

      it(`draws ${offer.name} in the corner, taken by a mouse's press and a finger's tap, at ${at}`, async () => {
        const pressed: Pressed = [];
        await drawnOn(screen, { offer: offer.piece(pressed) });
        await shown();
        const card = document
          .querySelector(".convenience-area .offer-card")!
          .getBoundingClientRect();
        // At the end of the overlay's top row, laid over it (the owner's look 1, 2026-11-23): inside
        // the overlay's own padding, a phone's --space-2 --space-3, else --space-5 --space-6.
        const isPhone = isPhoneLaidOut();
        expect(Math.round(window.innerWidth - card.right)).toBe(isPhone ? 12 : 32);
        expect(Math.round(card.top)).toBe(isPhone ? 8 : 24);
        expect(card.width).toBeLessThanOrEqual(Math.min(26 * 16, window.innerWidth - 48) + 0.5);
        for (const name of named) {
          await userEvent.click(offerButton(name));
          tapOn(offerButton(name));
        }
        expect(pressed).toEqual(named.flatMap((name) => [name, name]));
      });

      it(`reaches ${offer.name} by the keyboard's Tab and a remote's arrows, Enter and OK taking it, at ${at}`, async () => {
        const pressed: Pressed = [];
        await drawnOn(screen, { offer: offer.piece(pressed) });
        await shown();
        for (const name of named) {
          overlay().querySelector<HTMLElement>(".player-overlay__up")?.focus();
          let tabs = 0;
          while (document.activeElement !== offerButton(name) && tabs < 24) {
            await userEvent.keyboard("{Tab}");
            tabs += 1;
          }
          expect(isRingShown()).toBe(true);
          await userEvent.keyboard("{Enter}");
        }
        const input = startInput({ goBack: () => false, isNavigating: () => true });
        try {
          for (const name of named) {
            await shown();
            overlay().querySelector<HTMLElement>('button[aria-label="Play"]')?.focus();
            // Up alone reaches the card's first button, the one that takes it: Up next's, with no
            // ✕ before it (the owner, 2026-11-23).
            await reachedBy(["{ArrowUp}"], () => document.activeElement === offerButton(name));
            expect(isRingShown()).toBe(true);
            await userEvent.keyboard("{Enter}");
          }
        } finally {
          input.stop();
        }
        expect(pressed).toEqual([...named, ...named]);
      });
    }
  }
});

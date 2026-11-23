// SPDX-License-Identifier: MPL-2.0

import "./play-words.ts";

import type { components } from "@orpheum/sdk";
import {
  createContext,
  useEffect,
  useMemo,
  useRef,
  useState,
  type Context,
  type JSX,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { useTellsOffered, type OfferPlace } from "./convenience-area.tsx";
import { askedByOf, seekAheadOf } from "./seek-ahead.ts";
import { isOnTelevision } from "./input-mode.ts";
import { OfferCard } from "./offer-card.tsx";
import { hearStanding, useVideoStanding, type StandingTaken } from "./video-standing.ts";

/** A typed range of the file playing (plans/media-segments.md), as POST /playback gives it. */
export type MediaSegment = components["schemas"]["SegmentBody"];

/** No segments: a file with none known, and a page that plays none. */
const noSegments: readonly MediaSegment[] = [];

/**
 * The segments of the file the Play page plays (StartPlaybackOutputBody's segments), which its skip,
 * its commercials and its Up next read; none outside a Play page.
 */
export const SegmentsContext: Context<readonly MediaSegment[]> =
  createContext<readonly MediaSegment[]>(noSegments);

/** The segments a decision gives, none where it gives null. */
export function segmentsOf(given: readonly MediaSegment[] | null): readonly MediaSegment[] {
  return given ?? noSegments;
}

/** A segment the player offers to skip, and its button's words (D82: intro, recap, preview, credits). */
const skipWords = {
  intro: "playerControls.skipIntro",
  recap: "playerControls.skipRecap",
  preview: "playerControls.skipPreview",
  credits: "playerControls.skipCredits",
} as const;

/** The type of a segment the player offers to skip. */
type SkipType = keyof typeof skipWords;

/** A segment the player offers to skip. */
type SkippedSegment = MediaSegment & { readonly type: SkipType };

function isSkipOffered(segment: MediaSegment): segment is SkippedSegment {
  return segment.type in skipWords;
}

/**
 * How long a skip shows on its own as its segment starts, in ms, the rest of the controls hidden:
 * long enough to read and press, as Netflix's does; after it, it shows while the controls do. A
 * justified constant, the length of Up next's countdown (upNextCountdownMs) for the same reason.
 */
export const skipShownAloneMs = 10_000;

/**
 * Where the file's credits start, the last credits segment's start where it has one: where the Up
 * next offer and its countdown start in place of the fixed lead (D139, D339; plans/up-next.md §9).
 */
export function creditsStartOf(segments: readonly MediaSegment[]): number | undefined {
  let startMs: number | undefined;
  for (const segment of segments) {
    if (segment.type === "credits" && (startMs === undefined || segment.startMs > startMs)) {
      startMs = segment.startMs;
    }
  }
  return startMs;
}

/** Whether atMs is inside segment: from its start, to just before its end. */
function isInside(segment: MediaSegment, atMs: number): boolean {
  return atMs >= segment.startMs && atMs < segment.endMs;
}

/** The video's events at which a skip reads where the film stands. */
const skipEvents = ["loadedmetadata", "timeupdate", "seeked"];

/** The skip's taking of the video's reading among skipped: the segment it is inside, by its place. */
function skipTakingOf(skipped: readonly SkippedSegment[]): StandingTaken<number | undefined> {
  return {
    take: (standing) => {
      const index = skipped.findIndex((segment) => isInside(segment, standing.atMs));
      return index === -1 ? undefined : index;
    },
    isSame: (one, other) => one === other,
    initial: undefined,
    isReadAtOnce: true,
  };
}

/**
 * The skip over the film (D82; b1.md Phase C): while an intro, a recap, a preview or credits plays,
 * "Skip intro" (and its kind), an OfferCard pressed itself, with no ✕ (D538 §1), whose press seeks
 * to the segment's end. It shows on its own for
 * skipShownAloneMs as the segment starts, told the overlay as an offer (onOffered), which then shows
 * it with the rest hidden; after that it shows while the controls do, and in the bar under the film
 * while the segment plays. On a television it takes focus as it shows where nothing else has it, so
 * OK presses it, never from another control. A seek out of the segment, past its end or before its
 * start, hides it, as its press does. Credits are left to Up next where isCreditsLeftToUpNext.
 */
export function SkipOffer(
  props: OfferPlace & {
    readonly videoRef: RefObject<HTMLVideoElement | null>;
    readonly segments: readonly MediaSegment[];
    /** Whether Up next takes Skip credits' place: the episode has a next one. */
    readonly isCreditsLeftToUpNext: boolean;
  },
): JSX.Element | null {
  const { t } = useTranslation();
  const { videoRef, segments, isCreditsLeftToUpNext } = props;
  const skipped = useMemo(
    () =>
      segments.filter(
        (segment): segment is SkippedSegment =>
          isSkipOffered(segment) && !(isCreditsLeftToUpNext && segment.type === "credits"),
      ),
    [segments, isCreditsLeftToUpNext],
  );
  const taking = useMemo(() => skipTakingOf(skipped), [skipped]);
  const at = useVideoStanding(videoRef, skipEvents, taking);
  const { isPressed, isAlone, press } = useSkipShowing(at);
  const segment = at === undefined || isPressed ? undefined : skipped[at];
  const isShown = segment !== undefined;
  useTellsOffered(isShown && isAlone, props.onOffered);
  const buttonRef = useRef<HTMLButtonElement>(null);
  useFocusAsItIsDrawn(isShown && isOnTelevision(), buttonRef);
  if (segment === undefined) {
    return null;
  }
  const skip = (): void => {
    press();
    const video = videoRef.current;
    if (video !== null) {
      seekAheadOf(video).aim(segment.endMs, 0, askedByOf("skip", segment.type));
    }
  };
  return <OfferCard words={t(skipWords[segment.type])} onTake={skip} takeRef={buttonRef} />;
}

/** The frames a skip waits for its convenience area to be drawn before it gives up its focus. */
const focusTriesFrames = 10;

/**
 * Focuses the skip's button in buttonRef as it shows, where nothing but the video or the page has
 * focus, so a remote's OK presses it; never from another control, so a press already on its way
 * goes where it was aimed. The overlay draws its convenience area a render after the skip tells it
 * it shows (onOffered), and a button not yet drawn takes no focus (segments.browser.test.tsx), so it
 * tries at each frame until it has focus, focus goes elsewhere, or focusTriesFrames pass.
 */
function useFocusAsItIsDrawn(
  isShown: boolean,
  buttonRef: RefObject<HTMLButtonElement | null>,
): void {
  useEffect(() => {
    if (!isShown) {
      return undefined;
    }
    let frame = 0;
    let tries = 0;
    const tryFocus = (): void => {
      const focused = document.activeElement;
      const button = buttonRef.current;
      const isFree =
        focused === null || focused === document.body || focused instanceof HTMLVideoElement;
      if (button === null || !isFree) {
        return;
      }
      button.focus();
      tries += 1;
      if (document.activeElement !== button && tries < focusTriesFrames) {
        frame = requestAnimationFrame(tryFocus);
      }
    };
    tryFocus();
    return () => {
      cancelAnimationFrame(frame);
    };
  }, [isShown, buttonRef]);
}

/**
 * For the segment at (by its place), whether its skip was pressed, and whether it is still in its
 * first skipShownAloneMs; each segment come to starts both again.
 */
function useSkipShowing(at: number | undefined): {
  readonly isPressed: boolean;
  readonly isAlone: boolean;
  readonly press: () => void;
} {
  const [shownAt, setShownAt] = useState(at);
  const [isPressed, setIsPressed] = useState(false);
  const [isAlone, setIsAlone] = useState(true);
  // Adjusted as it renders.
  if (shownAt !== at) {
    setShownAt(at);
    setIsPressed(false);
    setIsAlone(true);
  }
  useEffect(() => {
    if (at === undefined) {
      return undefined;
    }
    const timer = setTimeout(() => {
      setIsAlone(false);
    }, skipShownAloneMs);
    return () => {
      clearTimeout(timer);
    };
  }, [at]);
  return {
    isPressed,
    isAlone,
    press: () => {
      setIsPressed(true);
    },
  };
}

/**
 * Skips each commercial of the file as the film comes to it, once in the playback (D82;
 * plans/media-segments.md: a commercial skips itself): seeking back into one plays it. The Play
 * page's, which leads its film; a Watch Together guest follows the host's skip.
 */
export function SkipsCommercials(props: {
  readonly video: HTMLVideoElement | null;
  readonly segments: readonly MediaSegment[];
}): null {
  const { video, segments } = props;
  // Each commercial skipped, by where it starts: kept for the playback, whatever renders between.
  const skippedRef = useRef(new Set<number>());
  useEffect(() => {
    const commercials = segments.filter((segment) => segment.type === "commercial");
    if (video === null || commercials.length === 0) {
      return undefined;
    }
    const skipped = skippedRef.current;
    return hearStanding(video, skipEvents, (standing) => {
      const coming = commercials.find(
        (segment) => !skipped.has(segment.startMs) && isInside(segment, standing.atMs),
      );
      if (coming !== undefined) {
        skipped.add(coming.startMs);
        seekAheadOf(video).aim(coming.endMs, 0, askedByOf("skip", "commercial"));
      }
    });
  }, [video, segments]);
  return null;
}

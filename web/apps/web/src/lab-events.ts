// SPDX-License-Identifier: MPL-2.0

/**
 * The button test's events on their way to the server's log as they happen (plans/tv-device-tests.md
 * H2; the owner, 2026-10-25: "the page should just send events to the server rather than waiting for
 * me to press send"): one request on its way at most, what comes meanwhile sent together once it is
 * answered, so no timer is needed, and what waits sent kept alive as the page leaves. Each run is
 * named as the page loads, for the log to be read by its name.
 */

import type { Api, components } from "@orpheum/sdk";

import { useCallback, useEffect, useRef, useState } from "react";

import { throwIfRefused } from "./problem.ts";

/** One event as the server logs it. */
export type LabInputEvent = components["schemas"]["LabInputEvent"];

/** The page as it opened, sent with a run's first events. */
export type LabInputRunStart = components["schemas"]["LabInputRunStart"];

/** Who moves focus between the tiles: the browser by itself, or our engine. */
export type FocusMode = NonNullable<LabInputEvent["mode"]>;

/** An event as the page notes it: its place in the run is the sender's to give. */
export type NotedEvent = Omit<LabInputEvent, "seq">;

/** What one request carries: the run's name, its events and, with the first, the page's start. */
export type LabEventsBody = components["schemas"]["LabInputEventsInputBody"];

/**
 * How many events one request carries at most: the server's bound (lab.go), which the page meets by
 * sending the rest in the next. A justified constant, the API's.
 */
export const eventsPerRequestAtMost = 200;

/** How many characters name a run: as many as name a lab result in the log (lab.go). */
const runNameLength = 8;

/** The characters a run's name is made of: base32's, as a lab result's name is. */
const runNameAlphabet = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567";

/** A new run's name, random: short enough to read off a TV and find in the log. */
export function newRunName(): string {
  const bytes = crypto.getRandomValues(new Uint8Array(runNameLength));
  return [...bytes].map((byte) => runNameAlphabet[byte % runNameAlphabet.length]).join("");
}

/** Where a run's sending stands: how many events the server has taken, and whether the last failed. */
export interface EventSending {
  readonly run: string;
  readonly sentCount: number;
  readonly hasFailed: boolean;
}

/** Sends one request of events; resolves once the server took them, rejects otherwise. */
export type PostEvents = (body: LabEventsBody, isLeaving: boolean) => Promise<void>;

/** Posts a request of events through the SDK, kept alive past the page as it leaves. */
export function postEventsBy(api: Api): PostEvents {
  return async (body, isLeaving) => {
    throwIfRefused(await api.POST("/lab-input-events", { body, keepalive: isLeaving }));
  };
}

/** A run's sender: note each event, and send what waits as the page leaves. */
export interface EventSender {
  readonly note: (event: NotedEvent) => void;
  readonly sendAsLeaving: () => void;
}

/**
 * The sender of a run's events: each noted event is numbered and sent at once when no request is
 * on its way, else waits for that one's answer and goes with the others that waited, in order; a
 * failed request's events wait for the next event to be tried again. The page's start goes with
 * the first request the server takes. onSending is told each change of where it stands.
 */
export function eventSender({
  run,
  start,
  post,
  onSending,
}: {
  readonly run: string;
  readonly start: LabInputRunStart;
  readonly post: PostEvents;
  readonly onSending: (sending: EventSending) => void;
}): EventSender {
  let nextSeq = 0;
  let waiting: LabInputEvent[] = [];
  let requestsOnTheirWay = 0;
  let isStartTaken = false;
  let isStartOnItsWay = false;
  let sentCount = 0;
  const send = (isLeaving: boolean): void => {
    if (waiting.length === 0 || (requestsOnTheirWay > 0 && !isLeaving)) {
      return;
    }
    const events = waiting.slice(0, eventsPerRequestAtMost);
    waiting = waiting.slice(eventsPerRequestAtMost);
    const isCarryingStart = !isStartTaken && !isStartOnItsWay;
    isStartOnItsWay ||= isCarryingStart;
    requestsOnTheirWay += 1;
    post({ run, events, ...(isCarryingStart ? { start } : {}) }, isLeaving).then(
      () => {
        requestsOnTheirWay -= 1;
        isStartTaken ||= isCarryingStart;
        sentCount += events.length;
        onSending({ run, sentCount, hasFailed: false });
        send(false);
      },
      () => {
        requestsOnTheirWay -= 1;
        isStartOnItsWay &&= !isCarryingStart;
        waiting = [...events, ...waiting].toSorted((one, other) => one.seq - other.seq);
        onSending({ run, sentCount, hasFailed: true });
      },
    );
  };
  return {
    note: (event) => {
      waiting.push({ ...event, seq: nextSeq });
      nextSeq += 1;
      send(false);
    },
    sendAsLeaving: () => {
      while (waiting.length > 0) {
        send(true);
      }
    },
  };
}

/**
 * The run's events on their way to the server's log as they happen, each with who moved focus when
 * it came: its name made as the page loads, the page's start sent with its first; what waits sent
 * as the page hides, leaves or this test closes, kept alive past the page.
 */
export function useInputEvents(
  api: Api,
  mode: FocusMode,
): {
  readonly sending: EventSending;
  readonly noteEvent: (event: NotedEvent) => void;
} {
  const [run] = useState(newRunName);
  const [sending, setSending] = useState<EventSending>({ run, sentCount: 0, hasFailed: false });
  const [sender] = useState(() =>
    eventSender({
      run,
      start: {
        viewportWidth: window.innerWidth,
        viewportHeight: window.innerHeight,
        devicePixelRatio: window.devicePixelRatio,
        mode,
        hasMediaSession: "mediaSession" in navigator,
        hasFullscreen: "requestFullscreen" in Element.prototype,
        hasCaptureStream: "captureStream" in HTMLCanvasElement.prototype,
      },
      post: postEventsBy(api),
      onSending: setSending,
    }),
  );
  const modeRef = useRef(mode);
  useEffect(() => {
    modeRef.current = mode;
  }, [mode]);
  useEffect(() => {
    const onVisibility = (): void => {
      if (document.visibilityState === "hidden") {
        sender.sendAsLeaving();
      }
    };
    document.addEventListener("visibilitychange", onVisibility);
    window.addEventListener("pagehide", sender.sendAsLeaving);
    return () => {
      document.removeEventListener("visibilitychange", onVisibility);
      window.removeEventListener("pagehide", sender.sendAsLeaving);
      sender.sendAsLeaving();
    };
  }, [sender]);
  const noteEvent = useCallback(
    (event: NotedEvent) => {
      sender.note({ mode: modeRef.current, ...event });
    },
    [sender],
  );
  return { sending, noteEvent };
}

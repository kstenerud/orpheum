// SPDX-License-Identifier: MPL-2.0

import { createApi } from "@orpheum/sdk";
import type { JSX, ReactNode } from "react";

import { InputLayer } from "./input-layer.tsx";
import { createNavigation, type Place } from "./navigation.ts";
import { createSession } from "./session.ts";
import { ServicesContext, type Services } from "./services.tsx";

/** The origin a test's place is at. */
export const testOrigin = "http://orpheum.test";

/** A browser's location and history for a test, which keeps every address in its history. */
export interface FakePlace extends Place {
  /** Each address in the history, oldest first, as its path, query and fragment. */
  readonly addresses: readonly string[];
  /** The browser moving back to an address, as its back button does. */
  back(to: string): void;
}

/** A place at start, the first address in its history. */
export function fakePlace(start: string): FakePlace {
  const location = { origin: testOrigin, pathname: "/", search: "", hash: "" };
  const addresses: string[] = [];
  const states: unknown[] = [];
  // The entry shown, which moves back by the history's own back.
  let at = 0;
  let popstate = (): void => undefined;
  const show = (to: string): string => {
    const next = new URL(to, testOrigin + location.pathname);
    location.pathname = next.pathname;
    location.search = next.search;
    location.hash = next.hash;
    return next.pathname + next.search + next.hash;
  };
  addresses.push(show(start));
  states.push(null);
  return {
    location,
    addresses,
    history: {
      get state() {
        return states[at] ?? null;
      },
      back: () => {
        if (at > 0) {
          at -= 1;
          show(addresses[at] ?? "/");
          popstate();
        }
      },
      go: (delta) => {
        const to = at + delta;
        if (delta !== 0 && to >= 0 && to < addresses.length) {
          at = to;
          show(addresses[at] ?? "/");
          popstate();
        }
      },
      pushState: (data, _unused, to) => {
        addresses.length = at + 1;
        states.length = at + 1;
        addresses.push(show(to));
        states.push(data);
        at += 1;
      },
      replaceState: (data, _unused, to) => {
        addresses[at] = show(to);
        states[at] = data;
      },
    },
    addEventListener: (_type, listener) => {
      popstate = listener;
    },
    back: (to) => {
      show(to);
      popstate();
    },
  };
}

/**
 * A place whose history steps come later, as a browser's do: each back or go waits for settle, and
 * its popstate with it.
 */
export function steppingLater(place: FakePlace): {
  readonly place: Place;
  readonly settle: () => void;
} {
  const queued: (() => void)[] = [];
  return {
    place: {
      location: place.location,
      history: {
        get state() {
          return place.history.state;
        },
        back: () => {
          queued.push(() => {
            place.history.back();
          });
        },
        go: (delta) => {
          queued.push(() => {
            place.history.go(delta);
          });
        },
        pushState: (data, unused, url) => {
          place.history.pushState(data, unused, url);
        },
        replaceState: (data, unused, url) => {
          place.history.replaceState(data, unused, url);
        },
      },
      addEventListener: (type, listener) => {
        place.addEventListener(type, listener);
      },
    },
    settle: () => {
      for (let step = queued.shift(); step !== undefined; step = queued.shift()) {
        step();
      }
    },
  };
}

/**
 * Services for a test render: an API that sends through send (by default nowhere: each request fails
 * at once, as with no network, and never reaches the real one; AGENTS.md rule 16), a session with the
 * token given, and a place at path.
 */
/** A fetch with no network under it: it fails as a browser's does, without looking anything up. */
function noNetwork(): Promise<Response> {
  return Promise.reject(new TypeError("Failed to fetch: a test has no network"));
}

export function testServices(
  token?: string,
  path = "/films",
  send?: (request: Request) => Promise<Response>,
): Services {
  const session = createSession(undefined);
  if (token !== undefined) {
    session.signIn(token);
  }
  const navigation = createNavigation(fakePlace(path));
  return {
    api: createApi({
      origin: testOrigin,
      getToken: session.token,
      fetch: send ?? noNetwork,
    }),
    session,
    navigation,
    playableFormats: () => ({
      videoCodecs: ["hevc-sdr", "h264-sdr"],
      audioCodecs: ["aac-stereo", "aac-multi", "opus", "flac"],
      subtitleFormats: ["ass", "webvtt"],
      containers: ["mp4"],
    }),
    // A socket that never opens: a test of Watch Together passes its own stand-in server.
    openTogetherSocket: () => ({ send: () => undefined, close: () => undefined }),
    canFollowAGroup: () => true,
    // No microphone: a test of voice passes its own stand-in devices.
    voiceDevices: () => ({
      openMicrophone: () => Promise.reject(new DOMException("no microphone", "NotFoundError")),
      startCall: () => {
        throw new Error("a test of voice passes its own stand-in devices");
      },
      readMicrophoneLevel: () => 0,
      prepareToHear: () => undefined,
      isWaitingForATap: () => false,
      subscribeToTapWaiting: () => () => undefined,
      playAtTap: () => undefined,
      microphoneClosed: () => undefined,
      close: () => undefined,
    }),
    fetchMedia: async (url, init) => {
      if (send === undefined) {
        throw new TypeError("Failed to fetch: this test serves no media");
      }
      return await send(new Request(new URL(url, testOrigin), init));
    },
  };
}

/** Wraps a tree in test services, with the input layer the app runs every screen under. */
export function withServices(tree: ReactNode, services: Services = testServices()): JSX.Element {
  return (
    <ServicesContext value={services}>
      <InputLayer>{tree}</InputLayer>
    </ServicesContext>
  );
}

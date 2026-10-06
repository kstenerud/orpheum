// SPDX-License-Identifier: MPL-2.0

import { createApi } from "@orpheum/sdk";
import type { JSX, ReactNode } from "react";

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
  let popstate = (): void => undefined;
  const show = (to: string): string => {
    const next = new URL(to, testOrigin + location.pathname);
    location.pathname = next.pathname;
    location.search = next.search;
    location.hash = next.hash;
    return next.pathname + next.search + next.hash;
  };
  addresses.push(show(start));
  return {
    location,
    addresses,
    history: {
      pushState: (_data, _unused, to) => {
        addresses.push(show(to));
      },
      replaceState: (_data, _unused, to) => {
        addresses[addresses.length - 1] = show(to);
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
 * Services for a test render: an API that sends through send (by default nowhere a test reaches),
 * a session with the token given, and a place at path.
 */
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
      ...(send === undefined ? {} : { fetch: send }),
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
  };
}

/** Wraps a tree in test services. */
export function withServices(tree: ReactNode, services: Services = testServices()): JSX.Element {
  return <ServicesContext value={services}>{tree}</ServicesContext>;
}

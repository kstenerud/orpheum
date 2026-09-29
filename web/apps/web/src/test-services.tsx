// SPDX-License-Identifier: MPL-2.0

import { createApi } from "@orpheum/sdk";
import type { JSX, ReactNode } from "react";

import { createNavigation } from "./navigation.ts";
import { createSession } from "./session.ts";
import { ServicesContext, type Services } from "./services.tsx";

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
  const location = { pathname: path, search: "" };
  const navigation = createNavigation({
    location,
    history: { pushState: () => undefined },
    addEventListener: () => undefined,
  });
  return {
    api: createApi({
      origin: "http://orpheum.test",
      getToken: session.token,
      ...(send === undefined ? {} : { fetch: send }),
    }),
    session,
    navigation,
    menu: () => ({
      videoCodecs: ["hevc-sdr", "h264-sdr"],
      audioCodecs: ["aac-stereo", "aac-multi", "opus", "flac"],
      subtitleFormats: ["ass", "webvtt"],
      containers: ["mp4"],
    }),
  };
}

/** Wraps a tree in test services. */
export function withServices(tree: ReactNode, services: Services = testServices()): JSX.Element {
  return <ServicesContext value={services}>{tree}</ServicesContext>;
}

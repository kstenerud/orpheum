// SPDX-License-Identifier: MPL-2.0

import { createApi } from "@orpheum/sdk";
import type { JSX, ReactNode } from "react";

import { createNavigation } from "./navigation.ts";
import { createSession } from "./session.ts";
import { ServicesContext, type Services } from "./services.tsx";

/** Services for a test render: an API nothing calls, a session with the token given, and a place at path. */
export function testServices(token?: string, path = "/films"): Services {
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
    api: createApi({ origin: "http://orpheum.test", token: session.token }),
    session,
    navigation,
  };
}

/** Wraps a tree in test services. */
export function withServices(tree: ReactNode, services: Services = testServices()): JSX.Element {
  return <ServicesContext value={services}>{tree}</ServicesContext>;
}

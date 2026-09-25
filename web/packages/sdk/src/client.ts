// SPDX-License-Identifier: MPL-2.0

import createClient, { type Client, type Middleware } from "openapi-fetch";

import type { paths } from "./gen/api.ts";

/** Where the API lives on the server that served the page (plans/cut-a.md §5). */
export const apiPrefix = "/api/v1";

/** The typed client for every endpoint of the API, generated from api/openapi.json. */
export type Api = Client<paths>;

/** What the client needs: where the server is, and the session token, when there is one. */
export interface ApiOptions {
  /** The server's origin; the page's own when omitted. */
  readonly origin?: string;
  /** The session token, sent in the Authorization header and never in a URL (D154). */
  readonly token: () => string | undefined;
}

/** Creates the API client. The token is read on every request, so signing in or out needs no new client. */
export function createApi(options: ApiOptions): Api {
  const api = createClient<paths>({
    baseUrl: (options.origin ?? "") + apiPrefix,
    credentials: "same-origin",
  });
  const authorise: Middleware = {
    onRequest({ request }) {
      const token = options.token();
      if (token !== undefined) {
        request.headers.set("Authorization", `Bearer ${token}`);
      }
      return request;
    },
  };
  api.use(authorise);
  return api;
}

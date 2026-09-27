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
  /** What sends a request: the browser's fetch when omitted, or a test's stand-in server. */
  readonly fetch?: (request: Request) => Promise<Response>;
}

/** Creates the API client. The token is read on every request, so signing in or out needs no new client. */
export function createApi(options: ApiOptions): Api {
  const api = createClient<paths>({
    baseUrl: (options.origin ?? "") + apiPrefix,
    credentials: "same-origin",
    ...(options.fetch === undefined ? {} : { fetch: options.fetch }),
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

/** The sizes an image is served at (D212): a closed set, as the server names them. */
export type ArtSize =
  | "portrait-s"
  | "portrait-m"
  | "portrait-l"
  | "landscape-s"
  | "landscape-m"
  | "landscape-l"
  | "landscape-xl"
  | "logo-s"
  | "logo-m";

/**
 * The address of one size of an image, by the hash a title's artwork names. An <img> sends the
 * media cookie sign-in set, never a token in the address (D154).
 */
export function artUrl(hash: string, size: ArtSize, format: "webp" | "jpeg" = "webp"): string {
  return `${apiPrefix}/bytes/art/${encodeURIComponent(hash)}/${size}.${format}`;
}

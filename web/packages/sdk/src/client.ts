// SPDX-License-Identifier: MPL-2.0

import createClient, {
  type Client,
  type ClientPathsWithMethod,
  type FetchResponse,
  type MaybeOptionalInit,
  type Middleware,
} from "openapi-fetch";

import type { paths } from "./gen/api.ts";

/** Where the API lives on the server that served the page (plans/cut-a.md §5). */
export const apiPrefix = "/api/v1";

/** The HTTP methods the client calls the API by. */
type ApiMethod = "get" | "put" | "post" | "delete";

/** The options one call of a path takes: its parameters and body, as the API document names them. */
type CallOptions<
  Path extends ClientPathsWithMethod<Client<paths>, Method>,
  Method extends ApiMethod,
> = MaybeOptionalInit<paths[Path], Method>;

/**
 * One method of the client, whose options are checked as they are written. openapi-fetch infers
 * them as a type parameter, and an inferred argument escapes TypeScript's check for a key the
 * type does not have, so a body could name a field the API renamed and still compile (agent
 * failures, A12). Here they are the path's own type, which the check applies to.
 */
type CheckedMethod<Method extends ApiMethod> = <
  Path extends ClientPathsWithMethod<Client<paths>, Method>,
>(
  url: Path,
  ...init: undefined extends CallOptions<Path, Method>
    ? [CallOptions<Path, Method>?]
    : [CallOptions<Path, Method>]
) => Promise<
  FetchResponse<NonNullable<paths[Path][Method]>, CallOptions<Path, Method>, "application/json">
>;

/** The typed client for every endpoint of the API, generated from api/openapi.json. */
export type Api = Omit<Client<paths>, Uppercase<ApiMethod>> & {
  readonly [Method in ApiMethod as Uppercase<Method>]: CheckedMethod<Method>;
};

/** What the client needs: where the server is, and the session token, when there is one. */
export interface ApiOptions {
  /** The server's origin; the page's own when omitted. */
  readonly origin?: string;
  /** Reads the session token, sent in the Authorization header and never in a URL (D154). */
  readonly getToken: () => string | undefined;
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
      const token = options.getToken();
      if (token !== undefined) {
        request.headers.set("Authorization", `Bearer ${token}`);
      }
      return request;
    },
  };
  api.use(authorise);
  // eslint-disable-next-line @typescript-eslint/consistent-type-assertions -- the same client: Api narrows only how a call's options are checked
  return api as Api;
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
 * The address of one size of an image, by the source SHA-256 a title's artwork names. An <img> sends the
 * media cookie sign-in set, never a token in the address (D154).
 */
export function artUrl(
  sourceSha256: string,
  size: ArtSize,
  format: "webp" | "jpeg" = "webp",
): string {
  return `${apiPrefix}/bytes/art/${encodeURIComponent(sourceSha256)}/${size}.${format}`;
}

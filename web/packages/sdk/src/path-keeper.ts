// SPDX-License-Identifier: MPL-2.0

/**
 * The path a page's requests take to its core, kept working (webrtc-transport.md W2c items 5 and 6):
 * a request that fails on the path in use for the network's sake moves the page to another path and
 * is sent again there, once; requests already under way finish on their own path; and a better path
 * found later is moved to between requests. Firefox fails every request to a LAN name until its
 * local-network prompt is answered, and again after a grant that was not remembered: a page that kept
 * the path it chose first sat there looking hung (the owner, 2026-10-15).
 */

import type { FetchRequest } from "./rtc-transport.ts";

/** How a keeper sends on a path, finds another, and tells the page which it moved to. */
export interface PathWays<Path> {
  /** Sends a request on path. */
  readonly sendOn: (path: Path) => FetchRequest;
  /** Another path than failed, the best that answers now; it rejects when none does. */
  readonly another: (failed: Path) => Promise<Path>;
  /** Told each path the keeper moves to, the first included, before any request goes on it. */
  readonly onUse: (path: Path) => void;
}

/**
 * Whether a request's failure is the network's, so another path may carry it: fetch's TypeError, as
 * a refused connection, a name that does not resolve or a blocked local-network request all are; an
 * abort is the page's own and is never sent again.
 */
export function isNetworkFailure(problem: unknown): boolean {
  return problem instanceof TypeError;
}

/**
 * Sends on send, a failure saying which request failed on which path, so the page's report of it
 * names them: a bare "NetworkError when attempting to fetch resource." said neither (the owner's
 * the production NAS log, 2026-10-15). A network failure stays a TypeError, so isNetworkFailure still knows it.
 */
export function namingFailures(send: FetchRequest, pathName: string): FetchRequest {
  return async (request) => {
    try {
      return await send(request);
    } catch (problem: unknown) {
      if (!(problem instanceof Error) || request.signal.aborted) {
        throw problem;
      }
      const where = `${request.method} ${new URL(request.url).pathname} over ${pathName}`;
      const Named = problem instanceof TypeError ? TypeError : Error;
      throw new Named(`${problem.message} (${where})`, { cause: problem });
    }
  };
}

/** Keeps the page's requests on a path that works, starting on first. */
export class PathKeeper<Path> {
  private current: Path;
  private moving: Promise<Path> | undefined;

  constructor(
    first: Path,
    private readonly ways: PathWays<Path>,
  ) {
    this.current = first;
    ways.onUse(first);
  }

  /** The path requests go on now. */
  get path(): Path {
    return this.current;
  }

  /**
   * Sends request on the path in use; one that fails for the network's sake moves the page to
   * another path, once for every request failing together, and is sent again there.
   */
  readonly fetch: FetchRequest = async (request) => {
    const sentOn = this.current;
    const again = request.clone();
    try {
      return await this.ways.sendOn(sentOn)(request);
    } catch (problem: unknown) {
      if (!isNetworkFailure(problem) || request.signal.aborted) {
        throw problem;
      }
      return await this.ways.sendOn(await this.awayFrom(sentOn))(again);
    }
  };

  /** Moves the requests sent from now on to path, a better one found later. */
  moveTo(path: Path): void {
    if (path !== this.current) {
      this.current = path;
      this.ways.onUse(path);
    }
  }

  /** The path to use instead of failed: the one in use when another request moved already. */
  private async awayFrom(failed: Path): Promise<Path> {
    if (failed !== this.current) {
      return this.current;
    }
    this.moving ??= this.ways
      .another(failed)
      .then((next) => {
        this.moveTo(next);
        return next;
      })
      .finally(() => {
        this.moving = undefined;
      });
    return await this.moving;
  }
}

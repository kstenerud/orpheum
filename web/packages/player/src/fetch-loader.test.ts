// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it, vi } from "vitest";

import Hls, {
  type LoaderCallbacks,
  type LoaderConfiguration,
  type LoaderContext,
  type LoaderResponse,
  type LoaderStats,
} from "hls.js";

import { fetchLoader, type RequestSetup } from "./fetch-loader.ts";
import type { ProgressingRequest } from "./first-piece.ts";

const noSetup: RequestSetup = {
  headersOf: () => [],
  answered: () => undefined,
  follow: () => undefined,
};

function contextOf(url: string, rest: Partial<LoaderContext> = {}): LoaderContext {
  const context: Partial<LoaderContext> = { url, responseType: "arraybuffer", ...rest };
  return context as LoaderContext;
}

function configOf(maxTimeToFirstByteMs: number, maxLoadTimeMs: number): LoaderConfiguration {
  return {
    loadPolicy: {
      maxTimeToFirstByteMs,
      maxLoadTimeMs,
      timeoutRetry: null,
      errorRetry: null,
    },
  } as unknown as LoaderConfiguration;
}

/** What a load told its callbacks. */
interface Said {
  readonly what: string;
  readonly response?: LoaderResponse;
  readonly stats?: LoaderStats;
  readonly code?: number;
}

/** The callbacks a load tells, each heard as what it was told. */
function heard(): {
  readonly callbacks: LoaderCallbacks<LoaderContext>;
  readonly told: Promise<Said>;
} {
  let tell: (said: Said) => void = () => undefined;
  const told = new Promise<Said>((resolve) => {
    tell = resolve;
  });
  return {
    told,
    callbacks: {
      onSuccess: (response, stats) => {
        tell({ what: "success", response, stats });
      },
      onError: (error) => {
        tell({ what: "error", code: error.code });
      },
      onTimeout: (stats) => {
        tell({ what: "timeout", stats });
      },
      onAbort: (stats) => {
        tell({ what: "abort", stats });
      },
    },
  };
}

function loaderOf(send: (request: Request) => Promise<Response>, setup: RequestSetup = noSetup) {
  const Loader = fetchLoader(send, setup);
  return new Loader(Hls.DefaultConfig);
}

describe("hls.js's loads made by a given fetch (the cast receiver over WebRTC)", () => {
  it("makes each load by the given fetch, with the setup's headers, and gives hls.js its bytes", async () => {
    const asked: Request[] = [];
    const loader = loaderOf(
      (request) => {
        asked.push(request);
        return Promise.resolve(new Response(new Uint8Array([1, 2, 3]), { status: 200 }));
      },
      { ...noSetup, headersOf: () => [["Authorization", "Bearer lab-token"]] },
    );
    const { callbacks, told } = heard();
    loader.load(contextOf("https://core.example/c/00001.m4s"), configOf(0, 0), callbacks);
    const said = await told;
    expect(said.what).toBe("success");
    expect(new Uint8Array(said.response?.data as ArrayBuffer)).toEqual(new Uint8Array([1, 2, 3]));
    expect(said.stats?.loaded).toBe(3);
    expect(said.stats?.total).toBe(3);
    expect(said.stats?.loading.end).toBeGreaterThanOrEqual(said.stats?.loading.first ?? Infinity);
    expect(asked[0]?.url).toBe("https://core.example/c/00001.m4s");
    expect(asked[0]?.headers.get("Authorization")).toBe("Bearer lab-token");
  });

  it("gives a playlist as text, and asks a byte range by Range", async () => {
    const asked: Request[] = [];
    const loader = loaderOf((request) => {
      asked.push(request);
      return Promise.resolve(new Response("#EXTM3U\n", { status: 200 }));
    });
    const { callbacks, told } = heard();
    loader.load(
      contextOf("https://core.example/c/master.m3u8", {
        responseType: "text",
        rangeStart: 100,
        rangeEnd: 200,
      }),
      configOf(0, 0),
      callbacks,
    );
    expect((await told).response?.data).toBe("#EXTM3U\n");
    expect(asked[0]?.headers.get("Range")).toBe("bytes=100-199");
  });

  it("tells the answer's status and headers, and its bytes to a follower as they arrive", async () => {
    const answers: [number, string | null][] = [];
    const arrivals: number[] = [];
    const loader = loaderOf(
      () =>
        Promise.resolve(
          new Response(new Uint8Array(5), { status: 200, headers: { "X-Ceiling": "800" } }),
        ),
      {
        headersOf: () => [],
        answered: (status, header) => {
          answers.push([status, header("X-Ceiling")]);
        },
        follow: (request: ProgressingRequest) => {
          request.addEventListener("progress", (event) => {
            arrivals.push(Reflect.get(event, "loaded") as number);
          });
        },
      },
    );
    const { callbacks, told } = heard();
    loader.load(contextOf("https://core.example/c/1.m4s"), configOf(0, 0), callbacks);
    await told;
    expect(answers).toEqual([[200, "800"]]);
    expect(arrivals).toEqual([5]);
  });

  it("fails a load the server refuses with its status, as hls.js's own loaders do", async () => {
    const loader = loaderOf(() => Promise.resolve(new Response("no", { status: 404 })));
    const { callbacks, told } = heard();
    loader.load(contextOf("https://core.example/c/1.m4s"), configOf(0, 0), callbacks);
    expect(await told).toMatchObject({ what: "error", code: 404 });
  });

  it("times out a load whose first byte is late, and stops its request", async () => {
    vi.useFakeTimers();
    try {
      let signal: AbortSignal | undefined;
      const loader = loaderOf((request) => {
        signal = request.signal;
        return new Promise(() => undefined);
      });
      const { callbacks, told } = heard();
      loader.load(contextOf("https://core.example/c/1.m4s"), configOf(5000, 20_000), callbacks);
      await vi.advanceTimersByTimeAsync(5000);
      expect((await told).what).toBe("timeout");
      expect(signal?.aborted).toBe(true);
    } finally {
      vi.useRealTimers();
    }
  });

  it("tells an abort once and nothing after, the answer that comes late unheard", async () => {
    let answer: (response: Response) => void = () => undefined;
    const loader = loaderOf(
      () =>
        new Promise((resolve) => {
          answer = resolve;
        }),
    );
    const said: string[] = [];
    const { callbacks } = heard();
    loader.load(contextOf("https://core.example/c/1.m4s"), configOf(0, 0), {
      ...callbacks,
      onSuccess: () => said.push("success"),
      onAbort: () => said.push("abort"),
    });
    loader.abort();
    answer(new Response(new Uint8Array(1), { status: 200 }));
    await new Promise((resolve) => setTimeout(resolve, 0));
    expect(said).toEqual(["abort"]);
  });
});

// SPDX-License-Identifier: MPL-2.0

import { describe, expect, it } from "vitest";

import {
  answerLoad,
  askPathOf,
  isPathMessage,
  isPathQueryMessage,
  responseFromPort,
  type FetchMessage,
  isFetchedByTheWorker,
} from "./rtc-media.ts";

describe("media over the WebRTC path", () => {
  it("carries a load from the worker to the page and its body back, umi no tobira per pull", async () => {
    const { port1, port2 } = new MessageChannel();
    const heard: Request[] = [];
    const pieces = ["orph", "eum"].map((text) => new TextEncoder().encode(text));
    const load: FetchMessage = {
      kind: "orpheum-fetch",
      url: "https://app.example/api/v1/bytes/media/x",
      method: "GET",
      headers: [["Range", "bytes=0-"]],
    };
    void answerLoad(load, port2, {
      send: (request) => {
        heard.push(request);
        return Promise.resolve(
          new Response(new Blob(pieces).stream(), {
            status: 206,
            headers: { "Content-Range": "bytes 0-6/7" },
          }),
        );
      },
      token: "t",
      viewer: "v",
    });
    const response = await responseFromPort(port1);
    expect(response.status).toBe(206);
    expect(response.headers.get("content-range")).toBe("bytes 0-6/7");
    expect(await response.text()).toBe("orpheum");
    expect(heard[0]?.headers.get("Range")).toBe("bytes=0-");
    expect(heard[0]?.headers.get("Authorization")).toBe("Bearer t");
    // A viewer's membership beside the page's own sign-in, which no cookie reaches across origins.
    expect(heard[0]?.headers.get("Orpheum-Viewer")).toBe("v");
    port1.close();
  });

  it("fails a load the page could not make", async () => {
    const { port1, port2 } = new MessageChannel();
    void answerLoad(
      { kind: "orpheum-fetch", url: "https://app.example/api/v1/x", method: "GET", headers: [] },
      port2,
      {
        send: () => Promise.reject(new Error("the channel closed")),
        token: undefined,
        viewer: undefined,
      },
    );
    await expect(responseFromPort(port1)).rejects.toThrow("the channel closed");
    port1.close();
  });

  it("knows a page's word on its path from anything else", () => {
    expect(isPathMessage({ kind: "orpheum-path", path: "webrtc" })).toBe(true);
    expect(isPathMessage({ kind: "orpheum-path", path: "pigeon" })).toBe(false);
    expect(isPathMessage("webrtc")).toBe(false);
  });

  it("asks a page its path, as a worker started again must: a page carrying its media says so, any other says nothing", async () => {
    const carrying = {
      postMessage: (message: unknown, transfer: Transferable[]) => {
        const [port] = transfer;
        if (isPathQueryMessage(message) && port instanceof MessagePort) {
          port.postMessage({ kind: "orpheum-path", path: "webrtc" });
        }
      },
    };
    expect(await askPathOf(carrying)).toBe("webrtc");
    const silent = { postMessage: () => undefined };
    expect(await askPathOf(silent, 20)).toBe("http");
  });
});

describe("the loads the worker fetches itself", () => {
  it("are those to another origin, as the core's own names are, never the page's own", () => {
    const worker = "https://app.orpheum-app.com";
    const lan = new URL("https://192-168-1-2.k7xq2.orpheum-core.com:8443/api/v1/me");
    expect(isFetchedByTheWorker(lan, worker)).toBe(true);
    expect(
      isFetchedByTheWorker(new URL("https://app.orpheum-app.com/api/v1/titles/7/poster"), worker),
    ).toBe(false);
    expect(isFetchedByTheWorker(new URL("http://192.168.1.2:8440/api/v1/me"), worker)).toBe(false);
  });
});

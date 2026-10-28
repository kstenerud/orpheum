// SPDX-License-Identifier: MPL-2.0

/**
 * The WebRTC media relay as released up to 106f0198, both its ends, frozen for the tests of a page
 * and a service worker of different releases: its worker sends "pull" for each piece of a body and
 * "cancel" to stop, and its page answers each "pull" with one piece. skipWaiting and clients.claim
 * hand a page still running this to a newer worker, and a reloaded page is controlled by this
 * worker until the newer one claims it; a newer relay that spoke only its own protocol to it left
 * every load hanging and staging's pages black (2026-10-28). Never change it: it is what is out
 * there. Not part of the app (sw.ts and the SDK never import it).
 */

import type { FetchMessage, PortLike } from "@orpheum/sdk";

/** The released page's side: answers load on port, one piece of its body for each "pull". */
export async function pullPageAnswer(
  message: FetchMessage,
  port: PortLike,
  send: (request: Request) => Promise<Response>,
): Promise<void> {
  const headers = new Headers();
  for (const [name, value] of message.headers) {
    headers.append(name, value);
  }
  let response: Response;
  try {
    response = await send(new Request(message.url, { method: message.method, headers }));
  } catch (error: unknown) {
    port.postMessage({ kind: "error", message: String(error) });
    return;
  }
  const replied: [string, string][] = [];
  response.headers.forEach((value, name) => {
    replied.push([name, value]);
  });
  port.postMessage({ kind: "head", status: response.status, headers: replied });
  const reader = response.body?.getReader();
  port.addEventListener("message", (event) => {
    if (event.data === "cancel") {
      void reader?.cancel();
      return;
    }
    if (event.data !== "pull") {
      return;
    }
    if (reader === undefined) {
      port.postMessage({ kind: "end" });
      return;
    }
    reader.read().then(
      ({ done: isDone, value }) => {
        if (isDone) {
          port.postMessage({ kind: "end" });
          return;
        }
        const data = value.slice().buffer;
        port.postMessage({ kind: "chunk", data }, [data]);
      },
      (error: unknown) => {
        port.postMessage({ kind: "error", message: String(error) });
      },
    );
  });
  port.start();
}

/** One reply of the released page's, as its worker read it. */
type PullReply =
  | { readonly kind: "head"; readonly status: number; readonly headers: [string, string][] }
  | { readonly kind: "chunk"; readonly data: ArrayBuffer }
  | { readonly kind: "end" }
  | { readonly kind: "error"; readonly message: string };

/** A reply as the released worker read it, or undefined for anything else. */
function asPullReply(data: unknown): PullReply | undefined {
  if (typeof data !== "object" || data === null || !("kind" in data)) {
    return undefined;
  }
  switch (data.kind) {
    case "head":
      return asPullHead(data);
    case "chunk":
      return asPullChunk(data);
    case "end":
      return { kind: "end" };
    case "error":
      return { kind: "error", message: "message" in data ? String(data.message) : "failed" };
    default:
      return undefined;
  }
}

/** A piece as the released worker read it, or undefined for one with no bytes. */
function asPullChunk(data: object): PullReply | undefined {
  return "data" in data && data.data instanceof ArrayBuffer
    ? { kind: "chunk", data: data.data }
    : undefined;
}

/** A head as the released worker read it, or undefined for one with no status. */
function asPullHead(data: object): PullReply | undefined {
  if (!("status" in data) || typeof data.status !== "number") {
    return undefined;
  }
  const headers = "headers" in data && Array.isArray(data.headers) ? data.headers : [];
  return {
    kind: "head",
    status: data.status,
    headers: headers.filter(
      (pair: unknown): pair is [string, string] =>
        Array.isArray(pair) && typeof pair[0] === "string" && typeof pair[1] === "string",
    ),
  };
}

/**
 * The released worker's side: the response port carries, resolved at its head, its body a stream
 * that sends "pull" for one piece at a time.
 */
export function pullWorkerResponse(port: PortLike): Promise<Response> {
  return new Promise<Response>((resolve, reject) => {
    let deliver: ((reply: PullReply) => void) | undefined;
    port.addEventListener("message", (event) => {
      const reply = asPullReply(event.data);
      if (reply === undefined) {
        return;
      }
      if (reply.kind === "head") {
        let waiting: ((next: PullReply) => void) | undefined;
        deliver = (next) => {
          const taker = waiting;
          waiting = undefined;
          taker?.(next);
        };
        const body = [101, 204, 205, 304].includes(reply.status)
          ? null
          : new ReadableStream<Uint8Array>(
              {
                pull: async (controller) => {
                  const next = await new Promise<PullReply>((taken) => {
                    waiting = taken;
                    port.postMessage("pull");
                  });
                  if (next.kind === "chunk") {
                    controller.enqueue(new Uint8Array(next.data));
                  } else if (next.kind === "error") {
                    controller.error(new Error(next.message));
                  } else {
                    controller.close();
                  }
                },
                cancel: () => {
                  port.postMessage("cancel");
                },
              },
              { highWaterMark: 0 },
            );
        resolve(new Response(body, { status: reply.status, headers: reply.headers }));
        return;
      }
      if (reply.kind === "error" && deliver === undefined) {
        reject(new Error(reply.message));
        return;
      }
      deliver?.(reply);
    });
    port.start();
  });
}

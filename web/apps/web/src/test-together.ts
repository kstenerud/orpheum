// SPDX-License-Identifier: MPL-2.0

import type { OpenTogetherSocket } from "@orpheum/sdk";
import { act } from "react";

/**
 * A stand-in for the server's end of the Watch Together socket: what a screen's sockets said, and
 * the server's messages to the last one opened. Tests never touch the network.
 */
export interface FakeTogetherServer {
  readonly openSocket: OpenTogetherSocket;
  /** Every message every socket sent, parsed, oldest first. */
  readonly heard: Record<string, unknown>[];
  /** How many sockets were opened. */
  readonly openedCount: () => number;
  /** The last socket opening, and its renders. */
  readonly accept: () => Promise<void>;
  /** The server sending message to the last socket, and its renders. */
  readonly say: (message: Readonly<Record<string, unknown>>) => Promise<void>;
}

/** A fake Watch Together server. */
export function fakeTogetherServer(): FakeTogetherServer {
  const heard: Record<string, unknown>[] = [];
  const opened: Parameters<OpenTogetherSocket>[0][] = [];
  const last = (): Parameters<OpenTogetherSocket>[0] => {
    const listeners = opened.at(-1);
    if (listeners === undefined) {
      throw new Error("no Watch Together socket was opened");
    }
    return listeners;
  };
  return {
    heard,
    openedCount: () => opened.length,
    openSocket: (listeners) => {
      opened.push(listeners);
      return {
        send: (text) => {
          const parsed: unknown = JSON.parse(text);
          if (typeof parsed === "object" && parsed !== null) {
            heard.push(Object.fromEntries(Object.entries(parsed)));
          }
        },
        close: () => undefined,
      };
    },
    accept: async () => {
      await act(async () => {
        last().onOpen();
        await Promise.resolve();
      });
    },
    say: async (message) => {
      await act(async () => {
        last().onMessage(JSON.stringify({ v: 2, ...message }));
        for (let turn = 0; turn < 5; turn += 1) {
          await new Promise((resolve) => setTimeout(resolve, 0));
        }
      });
    },
  };
}

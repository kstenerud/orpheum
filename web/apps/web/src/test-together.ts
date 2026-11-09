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
  /** Every socket opened opening, and their renders: a page with more than one at once. */
  readonly acceptEach: () => Promise<void>;
  /**
   * The server sending message to the page's own device, and its renders: the last socket that
   * said hello as anything but voice, which every page now attaches beside it (D281 §1); the last
   * socket opened where there is none.
   */
  readonly say: (message: Readonly<Record<string, unknown>>) => Promise<void>;
  /** The server refusing the last socket's hello (1008: no such session or member), and its renders. */
  readonly refuse: () => Promise<void>;
  /** The network losing the last socket (1006), which the page reconnects from, and its renders. */
  readonly drop: () => Promise<void>;
}

/** A few turns of the event loop: what a message sets off, renders and all. */
async function turns(): Promise<void> {
  for (let turn = 0; turn < 5; turn += 1) {
    await new Promise((resolve) => setTimeout(resolve, 0));
  }
}

/** Runs what the server does inside act, then lets what it set off render. */
function served(does: () => void): Promise<void> {
  return act(async () => {
    does();
    await turns();
  });
}

/** The network or the server closing a socket with code, and what that renders. */
function closeWith(socket: Parameters<OpenTogetherSocket>[0], code: number): Promise<void> {
  return served(() => {
    socket.onClose(code);
  });
}

/** A fake Watch Together server. */
export function fakeTogetherServer(): FakeTogetherServer {
  const heard: Record<string, unknown>[] = [];
  const opened: Parameters<OpenTogetherSocket>[0][] = [];
  // The role each socket said hello with, by its place in opened.
  const roles: (string | undefined)[] = [];
  const last = (): Parameters<OpenTogetherSocket>[0] => {
    const listeners = opened.at(-1);
    if (listeners === undefined) {
      throw new Error("no Watch Together socket was opened");
    }
    return listeners;
  };
  const device = (): Parameters<OpenTogetherSocket>[0] => {
    const index = roles.findLastIndex((role) => role !== undefined && role !== "panel");
    return index === -1 ? last() : (opened[index] ?? last());
  };
  return {
    heard,
    openedCount: () => opened.length,
    openSocket: (listeners) => {
      opened.push(listeners);
      roles.push(undefined);
      const index = opened.length - 1;
      return {
        send: (text) => {
          const parsed: unknown = JSON.parse(text);
          if (typeof parsed === "object" && parsed !== null) {
            const message = Object.fromEntries(Object.entries(parsed));
            heard.push(message);
            if (message["type"] === "hello") {
              roles[index] = typeof message["role"] === "string" ? message["role"] : "video";
            }
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
    acceptEach: async () => {
      await act(async () => {
        for (const listeners of opened) {
          listeners.onOpen();
        }
        await Promise.resolve();
      });
    },
    refuse: () => closeWith(last(), 1008),
    drop: () => closeWith(last(), 1006),
    say: (message) =>
      served(() => {
        device().onMessage(JSON.stringify({ v: 3, ...message }));
      }),
  };
}

/**
 * Two fake servers behind one opener: the page's live connection, the first socket it opens, or the
 * liveOpenedAs'th where a page's own session opens first, and a cast's or a session's, every other
 * (D337, D364).
 */
export function liveAndSession(liveOpenedAs = 1): {
  readonly live: FakeTogetherServer;
  readonly session: FakeTogetherServer;
  readonly openSocket: FakeTogetherServer["openSocket"];
} {
  const live = fakeTogetherServer();
  const session = fakeTogetherServer();
  let opened = 0;
  return {
    live,
    session,
    openSocket: (listeners) => {
      opened += 1;
      return opened === liveOpenedAs ? live.openSocket(listeners) : session.openSocket(listeners);
    },
  };
}

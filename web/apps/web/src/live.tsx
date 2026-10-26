// SPDX-License-Identifier: MPL-2.0

import { createLiveConnection, type CastToReceive, type LiveConnection } from "@orpheum/sdk";
import {
  createContext,
  use,
  useCallback,
  useEffect,
  useMemo,
  useState,
  useSyncExternalStore,
  type JSX,
  type ReactNode,
} from "react";

import { ownScriptOf } from "./app-update.ts";
import type { Navigation } from "./navigation.ts";
import { useServices, useToken } from "./services.tsx";
import { monotonicMs } from "./use-together.ts";

/** What every signed-in screen knows of the page's live connection (D337 §2). */
export interface Live {
  /** The person's devices connected now, this one among them, by id; empty until told. */
  readonly connectedDeviceIds: readonly number[];
  /** The cast this device receives now, until it ends here. */
  readonly receiving: CastToReceive | undefined;
  /** Ends receiving here: control taken back, or the cast ended. */
  readonly endReceiving: () => void;
}

const nothingLive: Live = {
  connectedDeviceIds: [],
  receiving: undefined,
  endReceiving: () => undefined,
};

const LiveContext = createContext<Live>(nothingLive);

/** The page's live connection as every signed-in screen reads it. */
export function useLive(): Live {
  return use(LiveContext);
}

const noDevices: readonly number[] = [];

/**
 * Holds the page's live connection while it is signed in, so the person's other devices see it is
 * open and can cast to it (D337 §2); a cast handed to it is received, the film's Play page opened.
 */
export function LiveProvider(props: { readonly children: ReactNode }): JSX.Element {
  const token = useToken();
  const { openTogetherSocket, navigation } = useServices();
  const connection = useMemo(
    (): LiveConnection | undefined =>
      token === undefined
        ? undefined
        : createLiveConnection({
            sessionToken: token,
            client: ownScriptOf(document),
            openSocket: openTogetherSocket,
            monotonicMs,
          }),
    [token, openTogetherSocket],
  );
  useEffect(() => {
    connection?.open();
    return () => {
      connection?.close();
    };
  }, [connection]);
  const snapshot = useSyncExternalStore(
    connection?.subscribe ?? subscribeToNothing,
    connection?.snapshot ?? noSnapshot,
    noSnapshot, // rendered on a server, as a test renders a page to markup, nothing is live
  );
  const { receiving, endReceiving } = useReceiving(connection, navigation);
  const live = useMemo(
    (): Live => ({
      connectedDeviceIds: snapshot?.deviceIds ?? noDevices,
      receiving,
      endReceiving,
    }),
    [snapshot?.deviceIds, receiving, endReceiving],
  );
  return <LiveContext value={live}>{props.children}</LiveContext>;
}

/**
 * The cast connection hands this device, while it receives one, and what ends receiving here: a
 * cast handed opens its film's Play page; handed again as the connection came back, it is received
 * already, unless the page left it; handed again with another film, the cast moved on to the next
 * episode (plans/up-next.md), which plays.
 */
function useReceiving(
  connection: LiveConnection | undefined,
  navigation: Navigation,
): { readonly receiving: CastToReceive | undefined; readonly endReceiving: () => void } {
  const [receiving, setReceiving] = useState<CastToReceive | undefined>(undefined);
  useEffect(() => {
    if (connection === undefined) {
      return undefined;
    }
    let received: CastToReceive | undefined;
    return connection.subscribe(() => {
      const handed = connection.snapshot().receive;
      if (handed === undefined) {
        return;
      }
      connection.takeReceive();
      const isSame = received?.castId === handed.castId && received.fileId === handed.fileId;
      if (!isSame || navigation.route().screen !== "play") {
        navigation.go({ screen: "play", fileId: handed.fileId });
      }
      received = handed;
      setReceiving((current) =>
        current?.castId === handed.castId && current.fileId === handed.fileId ? current : handed,
      );
    });
  }, [connection, navigation]);
  const endReceiving = useCallback(() => {
    setReceiving(undefined);
  }, []);
  return { receiving, endReceiving };
}

const subscribeToNothing = (): (() => void) => () => undefined;
const noSnapshot = (): undefined => undefined;

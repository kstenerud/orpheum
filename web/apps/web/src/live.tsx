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
  const [receiving, setReceiving] = useState<CastToReceive | undefined>(undefined);
  useEffect(() => {
    if (connection === undefined) {
      return undefined;
    }
    let receivingId: number | undefined;
    return connection.subscribe(() => {
      const handed = connection.snapshot().receive;
      if (handed === undefined) {
        return;
      }
      connection.takeReceive();
      // Handed again as its connection came back: received already, unless the page left it.
      if (receivingId !== handed.castId || navigation.route().screen !== "play") {
        navigation.go({ screen: "play", fileId: handed.fileId });
      }
      receivingId = handed.castId;
      setReceiving((current) => (current?.castId === handed.castId ? current : handed));
    });
  }, [connection, navigation]);
  const endReceiving = useCallback(() => {
    setReceiving(undefined);
  }, []);
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

const subscribeToNothing = (): (() => void) => () => undefined;
const noSnapshot = (): undefined => undefined;

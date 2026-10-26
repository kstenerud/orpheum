// SPDX-License-Identifier: MPL-2.0

import { createLiveConnection, type CastToReceive, type LiveConnection } from "@orpheum/sdk";
import {
  createContext,
  use,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  useSyncExternalStore,
  type JSX,
  type ReactNode,
} from "react";

import { ownScriptOf } from "./app-update.ts";
import { tabStorage } from "./browser-storage.ts";
import { forgetReturn, noteReturn, takeReturn } from "./cast-return.ts";
import type { Navigation } from "./navigation.ts";
import { useServices, useToken } from "./services.tsx";
import { monotonicMs } from "./use-together.ts";

/** What every signed-in screen knows of the page's live connection (D337 §2). */
export interface Live {
  /** The person's devices connected now, this one among them, by id; empty until told. */
  readonly connectedDeviceIds: readonly number[];
  /** Those of connectedDeviceIds with an Orpheum tab in front, which a cast goes to (D338 §2). */
  readonly inFrontDeviceIds: readonly number[];
  /** The cast this device receives now, until it ends here. */
  readonly receiving: CastToReceive | undefined;
  /** Ends receiving here, the film playing on as this device's own: control taken back here. */
  readonly endReceiving: () => void;
  /**
   * Ends receiving here and goes back to the page the cast took the tab over from: casting stopped,
   * or its film over with nothing after it (D338 §1).
   */
  readonly leaveReceiving: () => void;
}

const nothingLive: Live = {
  connectedDeviceIds: [],
  inFrontDeviceIds: [],
  receiving: undefined,
  endReceiving: () => undefined,
  leaveReceiving: () => undefined,
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
            isInFront: isTabInFront,
          }),
    [token, openTogetherSocket],
  );
  useEffect(() => {
    connection?.open();
    return () => {
      connection?.close();
    };
  }, [connection]);
  useTellsWhereTheTabIs(connection);
  const snapshot = useSyncExternalStore(
    connection?.subscribe ?? subscribeToNothing,
    connection?.snapshot ?? noSnapshot,
    noSnapshot, // rendered on a server, as a test renders a page to markup, nothing is live
  );
  const { receiving, endReceiving, leaveReceiving } = useReceiving(connection, navigation);
  const live = useMemo(
    (): Live => ({
      connectedDeviceIds: snapshot?.deviceIds ?? noDevices,
      inFrontDeviceIds: snapshot?.inFrontDeviceIds ?? noDevices,
      receiving,
      endReceiving,
      leaveReceiving,
    }),
    [snapshot?.deviceIds, snapshot?.inFrontDeviceIds, receiving, endReceiving, leaveReceiving],
  );
  return <LiveContext value={live}>{props.children}</LiveContext>;
}

/** Whether the page's tab is in front: shown, not behind another tab or a minimised window. */
function isTabInFront(): boolean {
  return document.visibilityState === "visible";
}

/** Tells the server each time the page's tab comes to the front or goes behind (D338 §2). */
function useTellsWhereTheTabIs(connection: LiveConnection | undefined): void {
  useEffect(() => {
    if (connection === undefined) {
      return undefined;
    }
    const moved = (): void => {
      connection.showFront(isTabInFront());
    };
    document.addEventListener("visibilitychange", moved);
    return () => {
      document.removeEventListener("visibilitychange", moved);
    };
  }, [connection]);
}

/**
 * The cast connection hands this device, while it receives one, and what ends receiving here: a
 * cast handed opens its film's Play page, noting the page it took the tab over from (D338 §1);
 * handed again as the connection came back, it is received already, unless the page left it;
 * handed again with another film, the cast moved on to the next episode (plans/up-next.md), which
 * plays.
 */
function useReceiving(
  connection: LiveConnection | undefined,
  navigation: Navigation,
): Pick<Live, "receiving" | "endReceiving" | "leaveReceiving"> {
  const [receiving, setReceiving] = useState<CastToReceive | undefined>(undefined);
  const isReceivingRef = useRef(false);
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
      noteReturn(tabStorage(), {
        castId: handed.castId,
        route: navigation.route(),
        isReceiving: isReceivingRef.current,
      });
      isReceivingRef.current = true;
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
    isReceivingRef.current = false;
    forgetReturn(tabStorage());
    setReceiving(undefined);
  }, []);
  const leaveReceiving = useCallback(() => {
    isReceivingRef.current = false;
    const back = takeReturn(tabStorage());
    setReceiving(undefined);
    navigation.go(back);
  }, [navigation]);
  return { receiving, endReceiving, leaveReceiving };
}

const subscribeToNothing = (): (() => void) => () => undefined;
const noSnapshot = (): undefined => undefined;

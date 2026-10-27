// SPDX-License-Identifier: MPL-2.0

import {
  createLiveConnection,
  type Api,
  type CastToReceive,
  type LiveConnection,
} from "@orpheum/sdk";
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
  type RefObject,
} from "react";

import { ownScriptOf } from "./app-update.ts";
import { tabStorage } from "./browser-storage.ts";
import { CastAsked, leaveFullScreen, type CastAnswer } from "./cast-asked.tsx";
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
  /** The press that answered Play to the cast received, and its full screen (D358 §2). */
  readonly answer: CastAnswer | undefined;
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
  answer: undefined,
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
  const { openTogetherSocket, navigation, api } = useServices();
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
  useChecksTheSessionWhenRefused(snapshot?.isRefused === true, api);
  const { receiving, answer, asked, onAnswered, endReceiving, leaveReceiving } = useReceiving(
    connection,
    navigation,
  );
  const live = useMemo(
    (): Live => ({
      connectedDeviceIds: snapshot?.deviceIds ?? noDevices,
      inFrontDeviceIds: snapshot?.inFrontDeviceIds ?? noDevices,
      receiving,
      answer,
      endReceiving,
      leaveReceiving,
    }),
    [
      snapshot?.deviceIds,
      snapshot?.inFrontDeviceIds,
      receiving,
      answer,
      endReceiving,
      leaveReceiving,
    ],
  );
  return (
    <LiveContext value={live}>
      {props.children}
      {asked === undefined ? null : (
        <CastAsked key={asked.castId} cast={asked} onAnswered={onAnswered} />
      )}
    </LiveContext>
  );
}

/**
 * Asks the server whether the session still works once the live connection is refused, or told the
 * device's sign-in ended as another device removed it: the answer's 401, which signed-out-watch.ts
 * hears, takes the page to sign in at once. A removed Fire TV kept showing its films until its
 * person pressed something (the owner, 2026-10-27).
 */
function useChecksTheSessionWhenRefused(isRefused: boolean, api: Api): void {
  useEffect(() => {
    if (isRefused) {
      api.GET("/me").catch(() => undefined); // a failure is the next check's, or the next press's
    }
  }, [isRefused, api]);
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

/** What useReceiving gives the provider: Live's part, and the cast it asks about and its answer. */
interface Received extends Pick<Live, "receiving" | "answer" | "endReceiving" | "leaveReceiving"> {
  /** The cast arrived and asked about, until it is answered (D358 §1). */
  readonly asked: CastToReceive | undefined;
  /** Told the answer to asked: Play's, which receives it, or undefined for none (Not now, silence). */
  readonly onAnswered: (answer: CastAnswer | undefined) => void;
}

/**
 * The cast connection hands this device, and what it does with it: one arriving while the device
 * receives none is asked about first, and received once its Play is pressed (D358 §1, §2); one
 * arriving while it receives one, the same cast handed again or another replacing it, is received
 * at once. A cast received opens its film's Play page, noting the page it took the tab over from
 * (D338 §1); handed again as the connection came back, it is received already, unless the page left
 * it; handed again with another film, the cast moved on to the next episode (plans/up-next.md),
 * which plays.
 */
function useReceiving(connection: LiveConnection | undefined, navigation: Navigation): Received {
  const [receiving, setReceiving] = useState<CastToReceive | undefined>(undefined);
  const isReceivingRef = useRef(false);
  const receivedRef = useRef<CastToReceive | undefined>(undefined);
  const receive = useCallback(
    (handed: CastToReceive) => {
      noteReturn(tabStorage(), {
        castId: handed.castId,
        route: navigation.route(),
        isReceiving: isReceivingRef.current,
      });
      isReceivingRef.current = true;
      const received = receivedRef.current;
      const isSame = received?.castId === handed.castId && received.fileId === handed.fileId;
      if (!isSame || navigation.route().screen !== "play") {
        navigation.go({ screen: "play", fileId: handed.fileId });
      }
      receivedRef.current = handed;
      setReceiving((current) =>
        current?.castId === handed.castId && current.fileId === handed.fileId ? current : handed,
      );
    },
    [navigation],
  );
  const { asked, answer, onAnswered } = useAsking(connection, isReceivingRef, receive);
  const endReceiving = useCallback(() => {
    isReceivingRef.current = false;
    receivedRef.current = undefined;
    forgetReturn(tabStorage());
    setReceiving(undefined);
  }, []);
  const leaveReceiving = useCallback(() => {
    isReceivingRef.current = false;
    receivedRef.current = undefined;
    const back = takeReturn(tabStorage());
    setReceiving(undefined);
    leaveFullScreen(document);
    navigation.go(back);
  }, [navigation]);
  return { receiving, answer, asked, onAnswered, endReceiving, leaveReceiving };
}

/**
 * Takes each cast the connection hands: received at once (receive) while the tab receives one
 * already, else asked about (D358 §1) and received once answered Play, with that answer.
 */
function useAsking(
  connection: LiveConnection | undefined,
  isReceivingRef: RefObject<boolean>,
  receive: (handed: CastToReceive) => void,
): Pick<Received, "asked" | "answer" | "onAnswered"> {
  const [asked, setAsked] = useState<CastToReceive | undefined>(undefined);
  const [answer, setAnswer] = useState<CastAnswer | undefined>(undefined);
  const askedRef = useRef<CastToReceive | undefined>(undefined);
  useEffect(() => {
    if (connection === undefined) {
      return undefined;
    }
    return connection.subscribe(() => {
      const handed = connection.snapshot().receive;
      if (handed === undefined) {
        return;
      }
      connection.takeReceive();
      if (isReceivingRef.current) {
        receive(handed);
        return;
      }
      askedRef.current = handed;
      setAsked(handed);
    });
  }, [connection, isReceivingRef, receive]);
  const onAnswered = useCallback(
    (given: CastAnswer | undefined) => {
      const answered = askedRef.current;
      askedRef.current = undefined;
      setAsked(undefined);
      if (given !== undefined && answered !== undefined) {
        setAnswer(given);
        receive(answered);
      }
    },
    [receive],
  );
  return { asked, answer, onAnswered };
}

const subscribeToNothing = (): (() => void) => () => undefined;
const noSnapshot = (): undefined => undefined;

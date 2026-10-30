// SPDX-License-Identifier: MPL-2.0

import {
  createLiveConnection,
  type Api,
  type CastSending,
  type CastToReceive,
  type LiveConnection,
  type LiveSnapshot,
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
import { forgetReceiving, markReceiving, wasReceiving } from "./cast-receiving-mark.ts";
import type { Navigation } from "./navigation.ts";
import type { Route } from "./router.ts";
import { useServices, useToken } from "./services.tsx";
import { monotonicMs } from "./use-together.ts";

/** What every signed-in screen knows of the page's live connection (D337 §2). */
export interface Live {
  /** The person's devices connected now, this one among them, by id; empty until told. */
  readonly connectedDeviceIds: readonly number[];
  /** Those of connectedDeviceIds with an Orpheum tab in front, which a cast goes to (D338 §2). */
  readonly inFrontDeviceIds: readonly number[];
  /** The person's devices receiving a cast now, which no device may cast to (D386 §1). */
  readonly receivingDeviceIds: readonly number[];
  /** The cast this device receives now, until it ends here. */
  readonly receiving: CastToReceive | undefined;
  /** The cast this device sends now, as the server last said (D364 §6); undefined for none. */
  readonly sending: CastSending | undefined;
  /**
   * Whether the server has said what this device sends since the page loaded, so a Play may go by
   * sending (PlayOrSendOn); true with no live connection, which sends nothing.
   */
  readonly isSendingKnown: boolean;
  /** The press that answered Play to the cast received, and its full screen (D358 §2). */
  readonly answer: CastAnswer | undefined;
  /**
   * Ends receiving here and sends the device to its home page: the cast ended, however it ended
   * (D384 §1), as by Stop casting, its session's end, its controller gone or control taken back here
   * (D385 §1).
   */
  readonly leaveReceiving: () => void;
  /**
   * Whether this page, loaded again while its tab received a cast (a reload, a deploy), does not know
   * yet whether that cast goes on, while which a Play page holds its film (PlayOrSendOn): one that does
   * not is left for home (D384 §1, useLeavesACastNoOneControls).
   */
  readonly isReceivingInDoubt: boolean;
}

const nothingLive: Live = {
  connectedDeviceIds: [],
  inFrontDeviceIds: [],
  receivingDeviceIds: [],
  receiving: undefined,
  sending: undefined,
  isSendingKnown: true,
  answer: undefined,
  leaveReceiving: () => undefined,
  isReceivingInDoubt: false,
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
  const { navigation, api } = useServices();
  const { connection, snapshot } = usePageLiveConnection();
  useChecksTheSessionWhenRefused(snapshot?.isRefused === true, api);
  const received = useReceiving(connection, navigation);
  const isInDoubt = useLeavesACastNoOneControls(snapshot?.isReceiveKnown === true, received);
  const live = useLiveOf(snapshot, received, isInDoubt);
  const { asked, onAnswered } = received;
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
 * The page's live connection while it is signed in, open while the provider shows, telling the
 * server where its tab is, and what it knows now; both undefined signed out.
 */
function usePageLiveConnection(): {
  readonly connection: LiveConnection | undefined;
  readonly snapshot: LiveSnapshot | undefined;
} {
  const token = useToken();
  const { openTogetherSocket, onPathBack } = useServices();
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
            // Heard as the session connection hears its page, one mechanism: shown again, or the way
            // to the core back, its socket is checked at once; left, it closes until the page returns.
            page: document,
            pathBack: onPathBack,
            leftPage: window,
          }),
    [token, openTogetherSocket, onPathBack],
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
  return { connection, snapshot };
}

/**
 * What every signed-in screen knows of the live connection, from its snapshot, what it received, and
 * whether a cast received before the page loaded again is in doubt.
 */
function useLiveOf(
  snapshot: LiveSnapshot | undefined,
  received: Received,
  isReceivingInDoubt: boolean,
): Live {
  const { receiving, answer, leaveReceiving } = received;
  return useMemo(
    (): Live => ({
      connectedDeviceIds: snapshot?.deviceIds ?? noDevices,
      inFrontDeviceIds: snapshot?.inFrontDeviceIds ?? noDevices,
      receivingDeviceIds: snapshot?.receivingDeviceIds ?? noDevices,
      receiving,
      sending: snapshot?.sending,
      isSendingKnown: snapshot?.isSendingKnown ?? true,
      answer,
      leaveReceiving,
      isReceivingInDoubt,
    }),
    [
      snapshot?.deviceIds,
      snapshot?.inFrontDeviceIds,
      snapshot?.receivingDeviceIds,
      snapshot?.sending,
      snapshot?.isSendingKnown,
      receiving,
      answer,
      leaveReceiving,
      isReceivingInDoubt,
    ],
  );
}

/**
 * Leaves a cast no one controls (D383 §2, D384 §1): this page loaded again while its tab received a
 * cast (a reload, a deploy), and once the server has said whether it hands the device a cast as it
 * connects (the snapshot's isReceiveKnown), it handed none, or one the person did not take (Not now),
 * so the cast is over unseen; a cast's page, its film's Play page or the ready page, goes home in its
 * place, and the mark that the tab received is forgotten. Whether the cast is still in doubt: until
 * then.
 */
function useLeavesACastNoOneControls(isReceiveKnown: boolean, received: Received): boolean {
  const { navigation } = useServices();
  const [doubt, setDoubt] = useState<"inDoubt" | "over" | "settled">(() =>
    wasReceiving(tabStorage()) ? "inDoubt" : "settled",
  );
  if (doubt === "inDoubt" && received.receiving !== undefined) {
    setDoubt("settled"); // handed again and taken: the cast goes on
  } else if (doubt === "inDoubt" && isReceiveKnown && received.asked === undefined) {
    setDoubt("over");
  }
  useEffect(() => {
    if (doubt !== "over") {
      return;
    }
    forgetReceiving(tabStorage());
    const { screen } = navigation.route();
    if (screen === "play" || screen === "cast-ready") {
      sendHome(navigation);
    }
  }, [doubt, navigation]);
  return doubt === "inDoubt";
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
interface Received extends Pick<Live, "receiving" | "answer" | "leaveReceiving"> {
  /** The cast arrived and asked about, until it is answered (D358 §1). */
  readonly asked: CastToReceive | undefined;
  /** Told the answer to asked: Play's, which receives it, or undefined for none (Not now, silence). */
  readonly onAnswered: (answer: CastAnswer | undefined) => void;
}

/**
 * The cast connection hands this device, and what it does with it: one arriving while the device
 * receives none is asked about first, and received once its Use is pressed (D364 §2, D358 §2); one
 * arriving while it receives one, the same cast handed again, is received at once (no other cast
 * takes a receiving device over, D386 §1). A cast received opens its film's Play page, or with nothing playing the full-screen ready
 * page (D364 §5), marking the tab as receiving (D383 §2); handed again as the
 * connection came back, it is received already, unless the page left it; handed again with another
 * film, or nothing, the person played another on the sender (D364 §3) or the cast moved on to the
 * next episode (plans/up-next.md), which plays.
 */
function useReceiving(connection: LiveConnection | undefined, navigation: Navigation): Received {
  const [receiving, setReceiving] = useState<CastToReceive | undefined>(undefined);
  const isReceivingRef = useRef(false);
  const receivedRef = useRef<CastToReceive | undefined>(undefined);
  const receive = useCallback(
    (handed: CastToReceive) => {
      markReceiving(tabStorage(), handed.castId);
      isReceivingRef.current = true;
      const received = receivedRef.current;
      const isSame = received?.castId === handed.castId && received.fileId === handed.fileId;
      const shown = receivedRoute(handed);
      if (!isSame || navigation.route().screen !== shown.screen) {
        navigation.go(shown);
      }
      receivedRef.current = handed;
      setReceiving((current) =>
        current?.castId === handed.castId &&
        current.fileId === handed.fileId &&
        isSamePart(current, handed)
          ? current
          : handed,
      );
    },
    [navigation],
  );
  const { asked, answer, onAnswered } = useAsking(connection, isReceivingRef, receive);
  const leaveReceiving = useCallback(() => {
    isReceivingRef.current = false;
    receivedRef.current = undefined;
    forgetReceiving(tabStorage());
    setReceiving(undefined);
    sendHome(navigation);
  }, [navigation]);
  return { receiving, answer, asked, onAnswered, leaveReceiving };
}

/**
 * Sends a receiver whose cast ended to its home page, in the current page's place, out of the full
 * screen its Use asked for (D384 §1, the owner: "it's probably best from a UX perspective for a castee
 * to go back to the home page whenever its cast ends"). A Play page of the cast's that Back reaches
 * later shows its title's page, never playing (D383 §1).
 */
function sendHome(navigation: Navigation): void {
  leaveFullScreen(document);
  navigation.replace({ screen: "home" });
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

/**
 * Whether a cast handed again takes the same part in Watch Together as the one received: in no
 * session, or in one as its host's player, or as a guest's. A host who starts Watch Together while
 * they cast has the cast handed again with their part in it, the same film playing on, and the
 * receiver takes it, leading that one session (D364 §11, D372 §8); a credential minted afresh for the
 * same part, as the connection comes back, changes nothing.
 */
function isSamePart(received: CastToReceive, handed: CastToReceive): boolean {
  return (
    (received.togetherCredential === undefined) === (handed.togetherCredential === undefined) &&
    (received.isTogetherFollower === true) === (handed.isTogetherFollower === true)
  );
}

/** Where a cast received shows: its film's Play page, or the ready page between videos (D364 §5). */
function receivedRoute(handed: CastToReceive): Route {
  return handed.fileId === 0 ? { screen: "cast-ready" } : { screen: "play", fileId: handed.fileId };
}

const subscribeToNothing = (): (() => void) => () => undefined;
const noSnapshot = (): undefined => undefined;

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
import { useTranslation } from "react-i18next";

import { ownScriptOf } from "./app-update.ts";
import { tabStorage } from "./browser-storage.ts";
import { forgetReceiving, markReceiving, wasReceiving } from "./cast-receiving-mark.ts";
import { CastStartPress, leaveFullScreen, type CastAnswer } from "./cast-start.tsx";
import { acceptCast } from "./cast.ts";
import type { Navigation } from "./navigation.ts";
import { showNotice } from "./notice.ts";
import { problemText } from "./problem.ts";
import type { Route } from "./router.ts";
import { useServices, useToken } from "./services.tsx";
import { hasPageBeenPressed } from "./start-refused.ts";
import { monotonicMs } from "./use-together.ts";

/** What every signed-in screen knows of the page's live connection (D337 §2). */
export interface Live {
  /**
   * Whether the live connection is up. While it is not, the lists below are as last told and may be
   * stale (a device gone, a cast over); true with no live connection, which lists nothing.
   */
  readonly isConnected: boolean;
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
  /**
   * The press that started the cast received, and its full screen, where the page waited for one
   * (D468 §2, D358 §2).
   */
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
  isConnected: true,
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
  useMarksTheLiveOpen(snapshot?.isConnected === true);
  const received = useReceiving(connection, navigation);
  const isInDoubt = useLeavesACastNoOneControls(snapshot?.isReceiveKnown === true, received);
  const live = useLiveOf(snapshot, received, isInDoubt);
  const { held, onPressed, onHeldGone } = received;
  return (
    <LiveContext value={live}>
      {props.children}
      {held === undefined ? null : (
        <CastStartPress key={held.castId} cast={held} onPressed={onPressed} onGone={onHeldGone} />
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
  const isConnected = snapshot?.isConnected ?? true;
  return useMemo(
    (): Live => ({
      isConnected,
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
      isConnected,
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
 * connects (the snapshot's isReceiveKnown), it handed none, or one the server would not let it take
 * (gone by then), so the cast is over unseen; a cast's page, its film's Play page or the ready page,
 * goes home in its place, and the mark that the tab received is forgotten. Whether the cast is still
 * in doubt: until then, a cast being taken or waiting for its press (D468 §2) among it.
 */
function useLeavesACastNoOneControls(isReceiveKnown: boolean, received: Received): boolean {
  const { navigation } = useServices();
  const [doubt, setDoubt] = useState<"inDoubt" | "over" | "settled">(() =>
    wasReceiving(tabStorage()) ? "inDoubt" : "settled",
  );
  if (doubt === "inDoubt" && received.receiving !== undefined) {
    setDoubt("settled"); // handed again and taken: the cast goes on
  } else if (doubt === "inDoubt" && isReceiveKnown && received.taking === undefined) {
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

/**
 * Marks whether the live connection is open (Services.liveOpen), so the session's minute check rests
 * while it is and asks again once it is not, or the provider is gone (signed-out-watch.ts).
 */
function useMarksTheLiveOpen(isConnected: boolean): void {
  const { liveOpen } = useServices();
  useEffect(() => {
    liveOpen?.mark(isConnected);
    return () => {
      liveOpen?.mark(false);
    };
  }, [isConnected, liveOpen]);
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
 * What useReceiving gives the provider: Live's part, the cast it is taking, and the one of those
 * that waits for the press that starts it.
 */
interface Received extends Pick<Live, "receiving" | "answer" | "leaveReceiving"> {
  /** The cast arrived and being taken, the server told, or waiting for its press (D468). */
  readonly taking: CastToReceive | undefined;
  /** The cast taken on a page never pressed since it loaded, until its press (D468 §2). */
  readonly held: CastToReceive | undefined;
  /** Told the press that starts held, which receives it. */
  readonly onPressed: (answer: CastAnswer) => void;
  /** Told that held ended before its press, its sender gone. */
  readonly onHeldGone: () => void;
}

/**
 * The cast connection hands this device, and what it does with it: one arriving while the device
 * receives none is taken, the server told at once so its sender hands its playback off (D468 §1,
 * §3), and received then where the page has been pressed since it loaded, or once a press starts it
 * where not (D468 §2); one arriving while it receives one, the same cast handed again, is received
 * at once (no other cast takes a receiving device over, D386 §1). A cast received opens its film's Play page, or with nothing playing the full-screen ready
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
  const taken = useTaking(connection, isReceivingRef, receive);
  const leaveReceiving = useCallback(() => {
    isReceivingRef.current = false;
    receivedRef.current = undefined;
    forgetReceiving(tabStorage());
    setReceiving(undefined);
    sendHome(navigation);
  }, [navigation]);
  return { receiving, leaveReceiving, ...taken };
}

/**
 * Sends a receiver whose cast ended to its home page, in the current page's place, out of the full
 * screen the press that started it asked for (D384 §1, the owner: "it's probably best from a UX
 * perspective for a castee to go back to the home page whenever its cast ends"). A Play page of the cast's that Back reaches
 * later shows its title's page, never playing (D383 §1).
 */
function sendHome(navigation: Navigation): void {
  leaveFullScreen(document);
  navigation.replace({ screen: "home" });
}

/** A cast being taken: the server being told (isHeld false), or taken and waiting for its press. */
interface Taking {
  readonly cast: CastToReceive;
  readonly isHeld: boolean;
}

/**
 * Takes each cast the connection hands: received at once (receive) while the tab receives one
 * already; else taken, the server told at once with nothing asked (D468 §1, §3), and received then
 * where the page has been pressed since it loaded (hasPageBeenPressed), or held for the press that
 * starts it (D468 §2) and received at that press, with it. One the server will not let it take, its
 * sender gone by then (stopped, or the server restarted and lost it), is received not at all, a
 * notice saying so.
 */
function useTaking(
  connection: LiveConnection | undefined,
  isReceivingRef: RefObject<boolean>,
  receive: (handed: CastToReceive) => void,
): Pick<Received, "taking" | "held" | "answer" | "onPressed" | "onHeldGone"> {
  const [answer, setAnswer] = useState<CastAnswer | undefined>(undefined);
  const { taking, takingRef, settle, take } = useTake(receive);
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
      take(handed);
    });
  }, [connection, isReceivingRef, receive, take]);
  const onPressed = useCallback(
    (given: CastAnswer) => {
      const held = takingRef.current;
      settle();
      if (held !== undefined) {
        setAnswer(given);
        receive(held);
      }
    },
    [receive, settle, takingRef],
  );
  return {
    taking: taking?.cast,
    held: taking?.isHeld === true ? taking.cast : undefined,
    answer,
    onPressed,
    onHeldGone: settle,
  };
}

/**
 * useTaking's cast being taken (taking, and takingRef its cast meanwhile), its end (settle), and its
 * take of a cast handed: the server told at once, then received where the page has been pressed,
 * else held; one the server refuses settled, a notice saying so. An answer for a cast no longer
 * takingRef's (handed again since, or over) is dropped.
 */
function useTake(receive: (handed: CastToReceive) => void): {
  readonly taking: Taking | undefined;
  readonly takingRef: RefObject<CastToReceive | undefined>;
  readonly settle: () => void;
  readonly take: (handed: CastToReceive) => void;
} {
  const { t } = useTranslation();
  const { api } = useServices();
  const [taking, setTaking] = useState<Taking | undefined>(undefined);
  const takingRef = useRef<CastToReceive | undefined>(undefined);
  const settle = useCallback(() => {
    takingRef.current = undefined;
    setTaking(undefined);
  }, []);
  const take = useCallback(
    (handed: CastToReceive) => {
      takingRef.current = handed;
      setTaking({ cast: handed, isHeld: false });
      acceptCast(api, handed.castId).then(
        () => {
          if (takingRef.current !== handed) {
            return; // handed again since, or over
          }
          if (hasPageBeenPressed()) {
            settle();
            receive(handed);
            return;
          }
          setTaking({ cast: handed, isHeld: true });
        },
        (cause: unknown) => {
          if (takingRef.current !== handed) {
            return;
          }
          settle();
          showNotice(
            problemText(t, cause, {
              notFound: t("cast.stoppedBeforeTaken", { device: handed.fromDeviceName }),
            }),
          );
        },
      );
    },
    [api, receive, settle, t],
  );
  return { taking, takingRef, settle, take };
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

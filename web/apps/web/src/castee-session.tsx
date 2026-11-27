// SPDX-License-Identifier: MPL-2.0

/**
 * The cast this device sends as the Play page works it, apart from what draws it: the cast held
 * through its end, its session joined and how it ended, where its castee last stood, the session
 * this tab hosts, what the cast plays and its moves (D555 §5, §8). The Play page over the castee
 * (play-castee.tsx) reads it.
 */

import {
  expectedGroupPositionMs,
  leaderOf,
  pingEveryMs,
  type components,
  type CastSending,
  type TogetherSnapshot,
} from "@orpheum/sdk";
import {
  useEffect,
  useMemo,
  useRef,
  useState,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";

import { tabStorage } from "./browser-storage.ts";
import { useRemotePrevious } from "./cast-remote-choices.tsx";
import { listMyCasts, moveCastOn, myCastsKey, type Cast } from "./cast.ts";
import type { WorkedSession } from "./castee-target.ts";
import { readTitle, titleKey } from "./item-detail.ts";
import { useLatest } from "./latest.ts";
import { useLive } from "./live.tsx";
import { usePlayUpward } from "./play-up.tsx";
import type { SequenceMoves } from "./player-controls.tsx";
import type { ReopenAt } from "./reopen.ts";
import { useServices, useToken } from "./services.tsx";
import { usePositionNow } from "./session-remote.tsx";
import { useMembership } from "./together-membership.ts";
import type { AroundFile } from "./around-file.ts";
import { useLeavesForThePageAbove } from "./up-next.tsx";
import { titleCrumb } from "./upward.ts";
import { useResource, type Resource } from "./use-resource.ts";
import { monotonicMs, useTogether } from "./use-together.ts";

/** A title's pictures, as its detail names them. */
type ArtBody = components["schemas"]["ArtBody"];
type TitleBody = components["schemas"]["ItemBody"];
type PlaysBody = components["schemas"]["PlaysBody"];

/**
 * Plays the page's film here again, from atMs, or where the person left it where undefined, paused
 * there where isPaused.
 */
export type PlaysHereFrom = (atMs: number | undefined, isPaused?: boolean) => void;

/**
 * The cast castId as the live connection says it, and after it ends, as it was last said, so its
 * end is said; undefined until it is said.
 */
export function useHeldCast(castId: number): CastSending | undefined {
  const { sending } = useLive();
  const now = sending?.castId === castId ? sending : undefined;
  const [held, setHeld] = useState<CastSending | undefined>(now);
  if (now !== undefined && now !== held) {
    setHeld(now); // kept as it renders, so it is there once the cast ends
  }
  return now ?? held;
}

/** A cast as its remote shows it: as sent, or on the film its session moved on to (filmOn). */
export type CastOn = CastSending & {
  /** The title its film plays as, where the session names it or the remote read it; else undefined. */
  readonly itemId?: number | undefined;
};

/**
 * Whether the cast's session, as snapshot says it, is a Watch Together session the cast is a guest's
 * in: its welcome heard (memberId) and its caster not the session's host. A cast alone is in its own
 * session of one, its caster that session's host, and a host's cast is in their Watch Together
 * session as its host (the core's together/cast.go, openAlone and moveIntoPart); a guest's cast takes
 * the guest's part there.
 */
function isAGuestsCast(snapshot: TogetherSnapshot | undefined): boolean {
  return snapshot?.memberId !== undefined && !snapshot.isHost;
}

/**
 * The cast sent as its remote shows it: as sent, but for a guest's cast in a Watch Together session
 * (isAGuestsCast) whose session plays another film than the one sent, on the session's film, its
 * title the session's or the episode the remote read for it (episodeOf), its length unknown. A
 * guest's cast is handed the film it started on, and the server leaves it so as the session moves
 * on, its castee following the session itself (cast-ready.tsx; the rework plan's next batch, item 2:
 * the guest's remote named the film it was handed). Every other cast is handed again as its session
 * moves on, so its remote shows the film sent: its session can say the next film before the server
 * hands the cast it, a cast's second film among them (cast.spec.ts).
 */
export function filmOn(
  sent: CastSending,
  snapshot: TogetherSnapshot | undefined,
  episodeOf: (fileId: number) => number | undefined,
): CastOn {
  const group = snapshot?.group;
  if (
    group === undefined ||
    !isAGuestsCast(snapshot) ||
    sent.fileId === 0 ||
    group.fileId === 0 ||
    group.fileId === sent.fileId
  ) {
    return sent;
  }
  return {
    ...sent,
    fileId: group.fileId,
    itemName: "",
    durationMs: 0,
    itemId: group.itemId ?? episodeOf(group.fileId),
  };
}

/**
 * Plays the film of the cast's last word here again, from atMs, paused there where isPaused, or from
 * its own start where atMs is undefined, no place being known (D549 §3): on the
 * page shown where it is the page's film (onPlayHere); else on that film's Play page, in this one's
 * place. Nothing played on the receiver, the page goes home (D446 §2).
 */
export function usePlaysHere(
  sent: CastSending,
  pageFileId: number,
  onPlayHere: PlaysHereFrom,
): (atMs: number | undefined, isPaused?: boolean) => void {
  const { navigation } = useServices();
  return (atMs, isPaused = false) => {
    if (sent.fileId === 0) {
      navigation.replace({ screen: "home" }); // nothing was on: nothing to take on (D446 §2)
    } else if (sent.fileId === pageFileId) {
      onPlayHere(atMs, isPaused);
    } else {
      navigation.replace({
        screen: "play",
        fileId: sent.fileId,
        ...(atMs === undefined ? {} : { atMs }), // none known: the film's own start, never 0 (D549 §3)
        shouldPlayHere: true,
        ...(isPaused ? { isPaused: true } : {}),
      });
    }
  };
}

/**
 * Leaves the remote's Play page for the page above it, as a local play leaves at its film's end with
 * nothing next (LeavesAtTheEnd; the owner, 2026-11-17), once the cast that played a film, fileId,
 * plays nothing: its receiver, a browser's or a Chromecast's, has it play nothing at its film's end
 * with nothing after it, waiting ready (D364 §5). The cast goes on, connected, for the next video
 * played from that page; one that played nothing from the start leaves nothing. Where the cast had
 * moved on from the page's film, pageFileId, to another episode, the page leaves for that episode's
 * page, as a local play moved on by Up next does, where episodeOf knows it.
 */
export function useLeavesAtTheCastsEnd(
  fileId: number,
  pageFileId: number,
  episodeOf: (fileId: number) => number | undefined,
): void {
  const { navigation } = useServices();
  const leaveRef = useLatest(useLeavesForThePageAbove());
  const episodeOfRef = useLatest(episodeOf);
  const playedRef = useRef(fileId);
  useEffect(() => {
    const played = playedRef.current;
    playedRef.current = fileId;
    if (played === 0 || fileId !== 0) {
      return;
    }
    const episodeId = played === pageFileId ? undefined : episodeOfRef.current(played);
    if (episodeId === undefined) {
      leaveRef.current?.();
      return;
    }
    const page = titleCrumb(episodeId, "");
    navigation.up(page.to, page.isIt);
  }, [fileId, pageFileId, leaveRef, episodeOfRef, navigation]);
}

/**
 * Has the remote's Play page's address follow its cast from the page's film, pageFileId, to the one
 * it moves on to, fileId, by Next, Previous or Up next, in its page's place, as a local play's does
 * (useSequenceMoves), as the episode episodeOf knows it for where it does. A page left at the film
 * the cast had moved on from sent that film on again when loaded again: a phone's page loaded again
 * after its Next put the episode before back on the television (the lead's walk, staging,
 * 2026-11-26). Only a move from the page's own film is followed: a page opened over a cast of
 * another film sends its own on, and the cast's word of the old one is no move.
 */
export function useAddressFollowsTheCast(
  fileId: number,
  pageFileId: number,
  episodeOf: (fileId: number) => number | undefined,
): void {
  const { navigation } = useServices();
  const episodeOfRef = useLatest(episodeOf);
  const playedRef = useRef(fileId);
  useEffect(() => {
    const played = playedRef.current;
    playedRef.current = fileId;
    if (played !== pageFileId || fileId === pageFileId || fileId === 0) {
      return;
    }
    const itemId = episodeOfRef.current(fileId);
    navigation.replace({ screen: "play", fileId, ...(itemId === undefined ? {} : { itemId }) });
  }, [fileId, pageFileId, episodeOfRef, navigation]);
}

/**
 * The episodes the remote has read the cast's moves to and back to, by their files: what plays after
 * and before each film the cast plays names its episode (useAroundFile), noted as it is read.
 */
export function useEpisodesMovedTo(): {
  readonly note: (around: AroundFile) => void;
  readonly of: (fileId: number) => number | undefined;
} {
  const [episodes] = useState(() => new Map<number, number>());
  return useMemo(
    () => ({
      note: (around) => {
        for (const episode of [around.upNext?.episode, around.previous]) {
          if (episode !== undefined) {
            episodes.set(episode.fileId, episode.itemId);
          }
        }
      },
      of: (fileId) => episodes.get(fileId),
    }),
    [episodes],
  );
}

/**
 * How far before the roster lost a receiver its last word came, plus how far it likely got past it
 * (D552 §2). The server takes a device for gone once it has sent nothing for three of its pings
 * (silenceLimit, internal/core/api/together_socket.go, held to the client's pingEveryMs by
 * TestTogether_TheSilenceLimitIsThreeOfTheClientsPings), so the receiver's last word came three gaps
 * before the roster lost it; a television switched off played on from that word for at most the one
 * gap before the ping it never sent. Two gaps back from the loss, then: a little behind what the
 * person last saw, never ahead of it (the owner, 2026-11-25: "It's fine to be off by an amount; we
 * just want to remain close to where it's likely to be"). A receiver whose page closed in order,
 * heard at once, is placed two gaps behind by the same rule, and scanned ahead from.
 */
const lostBeforeGoneMs = 2 * pingEveryMs;

/**
 * Where the receiver last stood while a device led the session the remote works, the cast's or,
 * while this tab hosts Watch Together, that session (D364 §11): its film's position, moved on by the
 * clock, and whether it was paused, kept at each render while a device leads. Control taken back,
 * the cast ends as it stands. The receiver gone, it led no more once the server found its connection
 * lost, and where it stood then is where it likely got to (D552 §2): its position lostBeforeGoneMs
 * before the roster lost it, never short of where its last group report placed it, and never where
 * the clock has moved a cast's session on to since, which is not held for want of a player (D385
 * §3: "The caster doesn't need to get its position exactly right").
 */
export function useWhereItStood(
  snapshot: TogetherSnapshot | undefined,
): RefObject<ReopenAt | undefined> {
  const positionMs = usePositionNow(snapshot);
  const stoodRef = useRef<ReopenAt | undefined>(undefined);
  const wasLedRef = useRef(false);
  const isLed =
    snapshot !== undefined && snapshot.status !== "ended" && leaderOf(snapshot) !== undefined;
  const isPaused = snapshot?.group?.state === "paused";
  useEffect(() => {
    if (isLed && positionMs !== undefined) {
      stoodRef.current = { positionMs, isPaused };
    } else if (wasLedRef.current && snapshot !== undefined && snapshot.status !== "ended") {
      // The leader lost while the session goes on: the receiver gone, where it likely got to.
      const lostAtMs = Math.max(monotonicMs() - lostBeforeGoneMs, snapshot.groupReceivedAtMs);
      const likelyMs = expectedGroupPositionMs(snapshot, lostAtMs);
      if (likelyMs !== undefined) {
        stoodRef.current = { positionMs: likelyMs, isPaused };
      }
    }
    wasLedRef.current = isLed;
  }, [isLed, positionMs, isPaused, snapshot]);
  return stoodRef;
}

/**
 * How a cast ended, as its remote says it: why, and where its video stood, and whether paused, where
 * its session said (stood).
 */
export interface CastEnd {
  readonly reason: string;
  readonly stood: ReopenAt | undefined;
}

/**
 * The cast castId's session, joined as this device's remote by a credential read from the server for
 * it: what it says, and how it ended once it has (ended, as by Stop casting in Cast's dialog, or no
 * longer the server's); and this device's casts as read for it, which name its title (useCastTitle).
 * The cast moved into its person's part in a Watch Together session, its own session let go
 * ("castMoved", D555 §2, §7), is no end: its casts are read again, what they showed kept meanwhile,
 * and the remote joins the session it moved to by the credential read for it (the walk on staging,
 * 2026-11-25: it said "Casting ended" while the receiver played on).
 */
export function useCastSession(castId: number): {
  readonly connection: ReturnType<typeof useTogether>["connection"];
  readonly snapshot: ReturnType<typeof useTogether>["snapshot"];
  readonly over: CastEnd | undefined;
  readonly casts: Resource<readonly Cast[]>;
} {
  const { api } = useServices();
  const token = useToken();
  // The cast started here handed its answer; only a page loaded anew asks (D478).
  const [casts, readCastsAgain] = useResource(
    myCastsKey(castId),
    async () => await listMyCasts(api),
    {
      isKeptOnReload: true,
    },
  );
  const own =
    casts.state === "loaded"
      ? casts.value.find((each) => each.castId === castId && each.isSender)
      : undefined;
  const { connection, snapshot } = useTogether({
    credential: own?.membershipCredential,
    sessionToken: token,
    role: "panel",
  });
  const positionMs = usePositionNow(snapshot);
  const told = snapshot?.status === "ended" ? snapshot.endReason : undefined;
  const isMoved = told === "castMoved";
  useEffect(() => {
    if (isMoved) {
      readCastsAgain();
    }
  }, [isMoved, readCastsAgain]);
  const endReason = isMoved ? undefined : told;
  const isGone = casts.state === "loaded" && own === undefined;
  return {
    connection,
    snapshot,
    casts,
    over: endOf({
      endReason,
      isGone,
      stood:
        positionMs === undefined
          ? undefined
          : { positionMs, isPaused: snapshot?.group?.state === "paused" },
    }),
  };
}

/**
 * How a cast ended, from its session's end or its absence, and where it stood (stood, as its session
 * says it now); undefined while it lasts.
 */
function endOf(cast: {
  readonly endReason: string | undefined;
  readonly isGone: boolean;
  readonly stood: ReopenAt | undefined;
}): CastEnd | undefined {
  const { endReason, isGone, stood } = cast;
  if (endReason === undefined && !isGone) {
    return undefined;
  }
  return { reason: endReason ?? "gone", stood };
}

/** Whether the remote's own connection to the server is lost: any of its sessions reconnecting (D553 §4). */
export function isServerLostIn(sessions: readonly (WorkedSession | undefined)[]): boolean {
  return sessions.some((each) => each?.snapshot?.status === "reconnecting");
}

/**
 * The remote's row of menus (D548 §1), aria-disabled while the receiver is not reached (§9), a
 * press on any opener in it then doing nothing.
 */
export function RemoteButtons(props: {
  readonly isDisabled: boolean;
  readonly children: ReactNode;
}): JSX.Element {
  const { isDisabled } = props;
  return (
    <div
      className="cast-remote__buttons"
      data-focus-container=""
      aria-disabled={isDisabled ? true : undefined}
      onClickCapture={
        isDisabled
          ? (event) => {
              event.preventDefault();
              event.stopPropagation();
            }
          : undefined
      }
    >
      {props.children}
    </div>
  );
}

/**
 * The remote's ⏮ and ⏭: ⏮ plays the previous episode in the cast (useRemotePrevious), ⏭ moves the
 * cast on to the next, as its Up next does, where one can play next.
 */
export function useRemoteMoves(sent: CastSending, around: AroundFile | undefined): SequenceMoves {
  const { api } = useServices();
  const previous = useRemotePrevious(sent, around?.previous);
  const upNext = around?.upNext;
  if (upNext === undefined || upNext.episode.unavailable !== undefined) {
    return previous;
  }
  return {
    ...previous,
    next: () => {
      moveCastOn(api, sent.castId).catch(() => undefined); // the cast goes on as it was
    },
  };
}

/**
 * The cast's title as its remote shows it: the player's way up to it (usePlayUpward), and its art;
 * neither while it is read.
 */
export function useRemoteTitle(
  casts: Resource<readonly Cast[]>,
  sent: CastOn,
): {
  readonly upward: ReturnType<typeof usePlayUpward> | undefined;
  readonly art: ArtBody | undefined;
} {
  const title = useCastTitle(casts, sent);
  const upward = usePlayUpward(title === undefined ? undefined : playsOf(title), title?.name);
  return { upward: title === undefined ? undefined : upward, art: title?.art };
}

/**
 * The title the cast sent plays as its film: the cast's item, as the server's list of this device's
 * casts names it (casts, the cast's session's read), read again under its file where the cast has
 * moved on since that read, else the cast's own itemId (a guest's cast the session moved on, filmOn), then
 * the title's detail, as its Title page reads it (readTitle). Undefined while read, or where the
 * cast names no title.
 */
function useCastTitle(casts: Resource<readonly Cast[]>, sent: CastOn): TitleBody | undefined {
  const { api } = useServices();
  const itemId = useCastItemId(casts, sent.castId, sent.fileId) ?? sent.itemId;
  const [title] = useResource(
    itemId === undefined ? "cast-art/none" : titleKey(itemId),
    async (signal) => (itemId === undefined ? undefined : await readTitle(api, itemId, signal)),
  );
  return title.state === "loaded" ? title.value : undefined;
}

/**
 * What a cast plays, title, as a Play page's playback says what it plays (PlaysBody), so the remote's
 * way up is the player's over its film (usePlayUpward): "The caster device shows the back links just
 * like the local video player does" (the owner, 2026-11-23). Its library is the title's first.
 */
function playsOf(title: TitleBody): PlaysBody {
  const library = title.libraries?.[0];
  return {
    itemId: title.id,
    kind: title.kind,
    ...(title.episode === undefined ? {} : { episode: title.episode }),
    ...(title.filmOfExtra === undefined ? {} : { filmOfExtra: title.filmOfExtra }),
    ...(library === undefined ? {} : { libraryId: library.id, libraryName: library.name }),
  };
}

/** The item the cast castId plays as fileId (useCastTitle); undefined while read, or where it names none. */
export function useCastItemId(
  casts: Resource<readonly Cast[]>,
  castId: number,
  fileId: number,
): number | undefined {
  const { api } = useServices();
  // The cast's session's read, which the cast started here handed its answer (D478): a reader of its
  // own would ask the server again, the handed answer taken by the first. Read again only for
  // another film.
  const firstCast = sentCastIn(casts, castId);
  const isFirstFilm = firstCast === undefined || firstCast.fileId === fileId;
  const [again] = useResource(
    isFirstFilm ? "cast-item/first" : `${myCastsKey(castId)}/file/${String(fileId)}`,
    async () => (isFirstFilm ? [] : await listMyCasts(api)),
  );
  const cast = sentCastIn(isFirstFilm ? casts : again, castId);
  return cast?.fileId === fileId && cast.itemId !== 0 ? cast.itemId : undefined;
}

/** The cast castId this device sends, as the list read says it; undefined while read, or without it. */
function sentCastIn(casts: Resource<readonly Cast[]>, castId: number): Cast | undefined {
  return casts.state === "loaded"
    ? casts.value.find((each) => each.castId === castId && each.isSender)
    : undefined;
}

/**
 * The Watch Together session this tab hosts, joined as a panel, whose controls the cast's remote
 * works while it is on (D364 §11); undefined while this tab hosts none.
 */
export function useHostedPanel(): WorkedSession | undefined {
  const token = useToken();
  const hosting = useMembership(tabStorage, "host");
  const hosted = useTogether({
    credential: hosting?.credential,
    sessionToken: token,
    role: "panel",
  });
  return hosting === undefined ? undefined : hosted;
}

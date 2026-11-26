// SPDX-License-Identifier: MPL-2.0

import type { TogetherConnection, TogetherEndReason, TogetherSnapshot } from "@orpheum/sdk";
import { createContext, use, useMemo, type JSX, type ReactNode } from "react";
import { useTranslation } from "react-i18next";

import type { UpNext } from "./around-file.ts";
import logo from "./assets/orpheum-logo.svg";
import type { OfferPlace } from "./convenience-area.tsx";
import type { FilmParts } from "./film-player.tsx";
import { useIsFullScreen } from "./full-screen-button.tsx";
import { CommandRightsContext, useVideoTarget, type SequenceMoves } from "./player-controls.tsx";
import { usePlayerCommands } from "./player-commands.ts";
import { productName } from "./product-name.ts";
import { useServices } from "./services.tsx";
import { GuestsCastEntry, type GuestsCast } from "./together-guest-cast.tsx";
import { LeaveButton } from "./together-leave.tsx";
import { requestTogetherAround } from "./together-playback.ts";
import type { Membership } from "./together-storage.ts";
import { TargetUpNext, useMovesCommands } from "./up-next.tsx";
import { useResource } from "./use-resource.ts";

/** What a guest's Cast and Leave are given. */
interface EntriesProps {
  /** Whether the guest joined by its link, signed in to nothing (D246 §3). */
  readonly isByLink: boolean;
  readonly cast: GuestsCast;
  readonly snapshot: TogetherSnapshot | undefined;
  readonly membership: Membership;
  readonly onLeft: (reason: TogetherEndReason) => void;
}

/**
 * A guest's page heading. A guest by link's is its top bar (D555 §9), who sees nothing of the app
 * (D246 §3): the product's mark and the session's name, then its Cast and Leave (GuestsEntries).
 * A signed-in guest's is the heading alone: the shell's banner is its top bar.
 */
export function GuestTopBar(props: EntriesProps): JSX.Element {
  const { t } = useTranslation();
  const heading = <h1 className="together-viewer__heading">{t("together.viewerHeading")}</h1>;
  if (!props.isByLink) {
    return heading;
  }
  return (
    <header className="together-viewer__bar">
      <img className="together-viewer__logo" src={logo} alt={productName} />
      {heading}
      <div className="together-viewer__bar-actions">
        <GuestsEntries place="bar" {...props} />
      </div>
    </header>
  );
}

/**
 * A guest's Cast and Leave where they belong (D555 §9): a guest by link's in its top bar while it
 * shows, out of full screen; in full screen, hiding the bar, in the session's panel, which the sign
 * over the film opens. A signed-in guest's are always the panel's.
 */
export function GuestsEntries(
  props: EntriesProps & { readonly place: "bar" | "panel" },
): JSX.Element | null {
  const isFullScreen = useIsFullScreen();
  const isBarShown = props.isByLink && !isFullScreen;
  if ((props.place === "bar") !== isBarShown) {
    return null;
  }
  return (
    <>
      <GuestsCastEntry cast={props.cast} snapshot={props.snapshot} />
      <LeaveButton membership={props.membership} onLeft={props.onLeft} />
    </>
  );
}

/** What a guest may put on from their film: the episode before or after it, and Up next's. */
interface GuestsMoves {
  readonly moves: SequenceMoves;
  readonly upNext: UpNext | undefined;
  /** Where the film's credits start, which Up next is offered from (D139). */
  readonly creditsStartMs: number | undefined;
}

const GuestMovesContext = createContext<GuestsMoves | undefined>(undefined);

/** What the guest's player may do, by the membership's rights. */
interface GuestsCommandsProps {
  readonly connection: TogetherConnection;
  readonly fileId: number;
  /** The membership's authorization, by which Up next is read. */
  readonly headers: Readonly<Record<string, string>>;
  readonly creditsStartMs: number | undefined;
  readonly children: ReactNode;
}

/**
 * The Play page's controls given only what the guest's link allows (D271 §4, D555 §10): play and
 * pause, and seeking, by its rights; and for a guest who may put on the episode after or before
 * (D471 §2), ⏮ and ⏭ each where there is one, as the host's own bar draws them (D407 §2), each
 * putting it on for everyone. Any other guest asks nothing of Up next.
 */
export function GuestsCommands(props: GuestsCommandsProps): JSX.Element {
  const { canPlayPause, canSeek, canNext } = props.connection.snapshot().rights;
  const rights = useMemo(() => ({ canPlayPause, canSeek }), [canPlayPause, canSeek]);
  return (
    <CommandRightsContext value={rights}>
      {canNext ? <ReadsGuestsMoves {...props} /> : props.children}
    </CommandRightsContext>
  );
}

/** The guest's moves, read once from GET /together/up-next for its film (GuestsCommands). */
function ReadsGuestsMoves(props: GuestsCommandsProps): JSX.Element {
  const { api } = useServices();
  const { connection, headers, creditsStartMs } = props;
  const [read] = useResource(`together-up-next/${String(props.fileId)}`, () =>
    requestTogetherAround(api, headers),
  );
  const around = read.state === "loaded" ? read.value : undefined;
  const hasPrevious = around?.previous !== undefined;
  const upNext = around?.upNext;
  const moves = useMemo(
    (): GuestsMoves => ({
      moves: {
        previous: hasPrevious
          ? () => {
              connection.command({ command: "previous" });
            }
          : undefined,
        next:
          upNext === undefined
            ? undefined
            : () => {
                connection.command({ command: "next" });
              },
      },
      upNext,
      creditsStartMs,
    }),
    [connection, hasPrevious, upNext, creditsStartMs],
  );
  return <GuestMovesContext value={moves}>{props.children}</GuestMovesContext>;
}

/** The guest's ⏮ and ⏭, none where its link puts on nothing (GuestsCommands). */
export function useGuestsMoves(): SequenceMoves | undefined {
  return use(GuestMovesContext)?.moves;
}

/**
 * The episode after offered near the film's end as the host's own player offers it, where the Play
 * page draws its own; it puts it on for everyone. It does not count down: only the host's player
 * does (D471, the owner's answers).
 */
export function GuestsUpNext(props: FilmParts & OfferPlace): JSX.Element | null {
  const guestsMoves = use(GuestMovesContext);
  const target = useVideoTarget(props.videoRef, true);
  if (guestsMoves?.upNext === undefined || guestsMoves.moves.next === undefined) {
    return null;
  }
  return (
    <TargetUpNext
      key={guestsMoves.upNext.episode.fileId}
      target={target}
      upNext={guestsMoves.upNext}
      isAutoplayOn={false}
      creditsStartMs={guestsMoves.creditsStartMs}
      isOverlayFull={props.isOverlayFull}
      onOffered={props.onOffered}
      onPlay={guestsMoves.moves.next}
    />
  );
}

/** A guest's moves where its link puts on no episode before or after: none. */
const noMoves: SequenceMoves = {};

/**
 * The guest's keys, the Play page's own (usePlayerCommands, useMovesCommands), each doing what its
 * button does for the guest (D557 §6): play and pause, and the steps, only where the link gives them,
 * as only then are their buttons drawn; ⏮ and ⏭ where it puts on the episode before or after; the
 * volume, mute and full screen always, as the guest's 🔈 and full screen are always drawn.
 */
export function useGuestsKeys(video: HTMLVideoElement, moves: SequenceMoves | undefined): void {
  const rights = use(CommandRightsContext);
  const tracks = useMovesCommands(moves ?? noMoves);
  usePlayerCommands(video, tracks, rights);
}

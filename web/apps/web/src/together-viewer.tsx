// SPDX-License-Identifier: MPL-2.0

import "./together-viewer.css";
import "./cast-words.ts";

import { isOnWirelessTarget } from "@orpheum/player";
import {
  followGroup,
  playedMutedEvent,
  soundOnAtTap,
  tellsNoFilmHere,
  type CannotKeepUp,
  type Standing,
  type BrowserVoiceDevices,
  type TellsCannotKeepUp,
  type TogetherConnection,
  type TogetherEndReason,
  type TogetherSnapshot,
} from "@orpheum/sdk";
import {
  createContext,
  use,
  useCallback,
  useEffect,
  useMemo,
  useRef,
  useState,
  type JSX,
  type ReactNode,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { secretFromHash } from "./accounts.ts";
import { AirPlayGrantsContext, sessionAirPlayGrants } from "./airplay.ts";
import { browserStorage, tabStorage } from "./browser-storage.ts";
import { GuestsMenusContext, IsGuestContext } from "./guest-page.ts";
import { pageThatPausesWhenHidden } from "./handheld.ts";
import { JoinForm } from "./together-join.tsx";
import { useStageFillingFromTheVideo } from "./film-fullscreen.ts";
import { isOnTelevision } from "./input-mode.ts";
import { EndsUnderTheHeading } from "./ends-place.tsx";
import { OverTheFilm, PageControls } from "./page-controls.tsx";
import {
  EmptyStage,
  FilmPlayer,
  type FilmComposition,
  type FilmParts,
  type MovedDown,
} from "./film-player.tsx";
import type { PlaybackDecision } from "./watching.ts";
import { ownTrackPlaying } from "./ladder-sound.ts";
import type { LadderOffer } from "./moving-down.ts";
import { CommandRightsContext } from "./player-controls.tsx";
import { useStallReports } from "./play-state.ts";
import { GuestTracksProvider } from "./together-guest-tracks.tsx";
import { useDiagnosisNotes } from "./together-diagnosis.ts";
import { useShowsAFilm } from "./film-showing.ts";
import { useMembership } from "./together-membership.ts";
import { SoundOnPill } from "./sound-on-pill.tsx";
import { useGoneFromUnder, useSessionEnd } from "./together-session-end.ts";
import { useServices, useToken } from "./services.tsx";
import { NoSessionHere, SessionOver, usePastedLinks } from "./together-viewer-outside.tsx";
import { Loaded } from "./loaded.tsx";
import { TogetherDebug } from "./together-debug.tsx";
import { TogetherNews } from "./together-news.tsx";
import { useNotesTheGroupsFilmShown } from "./playing-here.ts";
import { useGroupFilmDecision } from "./together-playback.ts";
import { creditsStartOf } from "./segments.tsx";
import { AsksWhatPlays, type GroupPlays } from "./together-what-plays.tsx";
import {
  hasBeenPressed,
  TapToContinue,
  useKeptVideo,
  useViewerVoiceDevices,
  ViewerPage,
  type KeptVideo,
} from "./together-viewer-tap.tsx";
import { ComingAndGoing, TogetherPanel } from "./together-over-the-film.tsx";
import {
  CastingStage,
  useCastingTheGroupsFilm,
  useIsCastingTheGroupsFilm,
} from "./together-on-the-remote.tsx";
import { useGuestsCast, type GuestsCast } from "./together-guest-cast.tsx";
import {
  GuestsCommands,
  GuestsEntries,
  GuestsUpNext,
  GuestTopBar,
  useGuestsMoves,
} from "./together-guest-controls.tsx";
import { SessionVoiceContext, useVoice, VoiceControls } from "./together-voice.tsx";
import {
  forgetEndedHere,
  keepMembership,
  membershipHeaders,
  type Membership,
} from "./together-storage.ts";
import { monotonicMs, useTogether, useToldCannotKeepUp, useToldStanding } from "./use-together.ts";

/**
 * A Watch Together viewer's page (D246 §3, §5, §6), for someone who need not be signed in: they
 * join by their link with a name everyone sees, asked nothing of how their device makes sound
 * (D336 §4), and then see only the host's playback, black while nothing plays, with their own
 * volume and Leave. The link's secret is after #, and leaves the address once used.
 */
export function TogetherViewer(): JSX.Element {
  const { navigation } = useServices();
  const token = useToken();
  const [secret, setSecret] = useState(() => secretFromHash(navigation.fragment()));
  const kept = useMembership(browserStorage, "viewer");
  // The membership this browser keeps, unless a link other than the one it joined by was opened:
  // the same link opened again comes back as the same member (D267 §1).
  const membership = secret === undefined || kept?.link === secret ? kept : undefined;
  const isSignedIn = token !== undefined;
  const { endReason, over } = useSessionEnd(isSignedIn, secret);
  // Made before the Join, so its tap, and any tap after, lets the film and the voices play with
  // sound: one tap gives a guest both, whichever starts first (the owner, 2026-10-13).
  const keptVideo = useKeptVideo();
  const [hasJoinedHere, setHasJoinedHere] = useState(false);
  const devices = useViewerVoiceDevices();
  const hasMembership = membership !== undefined;
  const sessionsAsked = useGoneFromUnder({ isSignedIn, secret, hasMembership, over });
  useEffect(() => {
    if (membership !== undefined && secret !== undefined) {
      // Come back by the link already joined by: it leaves the address, as at the join.
      navigation.replace({ screen: "together" });
    }
  }, [membership, secret, navigation]);
  usePastedLinks(setSecret, over.again);
  if (endReason !== undefined) {
    return <SessionOver reason={endReason} onAgain={over.again} />;
  }
  if (membership === undefined) {
    if (secret === undefined) {
      return <NoSessionHere isSignedIn={isSignedIn} asked={sessionsAsked} />;
    }
    return (
      <JoinForm
        secret={secret}
        heldCredential={kept?.credential}
        onSubmitted={() => {
          setHasJoinedHere(true);
          // The Join's tap, however the browser submitted it: an iPhone's guest who had just joined
          // was asked to Tap to continue (the owner's test, 2026-10-13).
          keptVideo.tapped();
          devices.prepareToHear();
        }}
        onJoined={(joined) => {
          // Kept with its link in this browser: closing the page does not lose it (D267 §1).
          keepMembership(browserStorage(), "viewer", { ...joined, link: secret });
          forgetEndedHere(tabStorage());
          // The secret is spent: it stays in neither the address nor the history (D243 §3).
          navigation.replace({ screen: "together" });
          setSecret(undefined);
        }}
      />
    );
  }
  if (isSignedIn) {
    // A signed-in guest's film plays on its Play page, the one page every film plays on (D384 §2);
    // with nothing playing yet, this page waits for it (ToTheGroupsFilm).
    return <ToTheGroupsFilm membership={membership} />;
  }
  return (
    <ViewerSession
      membership={membership}
      canCastAsTheMembership
      hasJoinedHere={hasJoinedHere}
      kept={keptVideo}
      devices={devices}
      onOver={over.end}
    />
  );
}

/**
 * The group's film on its Play page, for a signed-in guest in a session that plays it (D384 §2): the
 * one page every film plays on, here as a guest's, the film following the group, with who is here,
 * voice, their own volume and Leave; it moves to the Play page of the next film the group plays, as it
 * does from the guest's own page (/together), where it waits while the group plays nothing, and goes
 * as the group's film stops (the owner, 2026-11-17). Ended, they are taken home and told why
 * (useSessionEnd); sent away for the host's connection, the page says so, with Join again. A page
 * shown by a press needs no tap to continue: the press was it.
 */
export function GroupFilmPage(props: { readonly membership: Membership }): JSX.Element {
  const { navigation } = useServices();
  const { endReason, over } = useSessionEnd(true, undefined);
  const keptVideo = useKeptVideo(hasBeenPressed());
  useNotesTheGroupsFilmShown(); // a cast from over it follows the group (D371 §4)
  const devices = useViewerVoiceDevices();
  const toFilm = useCallback(
    (fileId: number) => {
      const shown = navigation.route();
      if (fileId === 0) {
        if (shown.screen === "play") {
          navigation.replace({ screen: "together" });
        }
        return;
      }
      if (shown.screen === "together" || (shown.screen === "play" && shown.fileId !== fileId)) {
        navigation.replace({ screen: "play", fileId });
      }
    },
    [navigation],
  );
  if (endReason !== undefined) {
    return <SessionOver reason={endReason} onAgain={over.again} />;
  }
  return (
    <ViewerSession
      membership={props.membership}
      hasJoinedHere={false}
      kept={keptVideo}
      devices={devices}
      onOver={over.end}
      onGroupFilm={toFilm}
    />
  );
}

/**
 * A signed-in guest's way into their session, where their join lands and their page's address
 * (/together) leads: the Play page of the group's film in its place, at once where the group plays
 * one (D384 §2); else this page, waiting for the host to start one (GroupFilmPage), which moves to its
 * Play page as one starts. No dialog opens: Watch Together's opens from the tray's sign alone (the
 * owner, 2026-10-30: a guest who joined saw the dialog over home, "Nothing is playing", no video).
 */
function ToTheGroupsFilm(props: { readonly membership: Membership }): JSX.Element | null {
  const { navigation } = useServices();
  const [plays, setPlays] = useState<GroupPlays | undefined>(undefined);
  const fileId = plays?.kind === "film" ? plays.fileId : 0;
  useEffect(() => {
    if (fileId !== 0) {
      navigation.replace({ screen: "play", fileId });
    }
  }, [fileId, navigation]);
  if (plays === undefined) {
    return <AsksWhatPlays membership={props.membership} onAnswered={setPlays} />;
  }
  return plays.kind === "nothing" ? <GroupFilmPage membership={props.membership} /> : null;
}

/** What a viewer's page waits on before its film can play: its tap. */
function standingOf(kept: KeptVideo): Standing {
  return kept.hasHadATap ? "" : "needsTap";
}

/**
 * A viewer in the session: the playback, with nothing asked of how they hear it (D336 §4); named
 * events and who is here; the volume and Leave. Until the player is ready it holds no one up
 * (D246 §6).
 */
function ViewerSession(props: {
  readonly membership: Membership;
  /** Whether it casts to a Chromecast as the membership: a guest by link, signed in to nothing. */
  readonly canCastAsTheMembership?: boolean;
  /** Whether this page joined by the link itself, rather than came back to the membership. */
  readonly hasJoinedHere: boolean;
  readonly kept: KeptVideo;
  readonly devices: BrowserVoiceDevices;
  readonly onOver: (reason: TogetherEndReason) => void;
  /** Told each film the group plays, on a page that follows it to that film's Play page. */
  readonly onGroupFilm?: ((fileId: number) => void) | undefined;
}): JSX.Element {
  const { connection, snapshot } = useTogether({ credential: props.membership.credential });
  const { onOver, kept, membership } = props;
  const isConnected = snapshot?.status === "connected";
  // Everyone in the session is heard from the start, once the page's own device is welcomed and the
  // page has had its tap; Talk only opens the microphone (D281 §1).
  const voice = useVoice(props.membership, undefined, {
    isReady: isConnected && kept.hasHadATap,
    devices: props.devices,
  });
  useOverWhenEnded(snapshot, onOver);
  useFollowsTheGroupsFilm(snapshot, props.onGroupFilm);
  useToldStanding(isConnected ? connection : undefined, standingOf(kept));
  // A guest by link, signed in to nothing, casts as the membership and has a top bar of its own.
  const isByLink = props.canCastAsTheMembership === true;
  const guestCast = useGuestsCast(props.membership, isByLink);
  const isCasting = useIsCastingTheGroupsFilm(snapshot, guestCast.castingTo !== undefined);
  const entries = { isByLink, cast: guestCast, snapshot, membership, onLeft: onOver };
  if (!kept.hasHadATap) {
    // As a page of its own: nothing of the session plays or shows under it (the owner, 2026-10-13).
    return (
      <TapToContinue
        roster={snapshot?.roster ?? []}
        connection={isConnected ? connection : undefined}
        hasJoinedHere={props.hasJoinedHere}
      />
    );
  }
  // The player's overlay reaches the people's volumes and Talk from it (D440 §2, §3).
  return (
    <SessionVoiceContext value={voice}>
      <ViewerPage className="together-viewer" inShellClassName="together-viewer--in-shell">
        <GuestTopBar {...entries} />
        <ViewingArea
          {...{ kept, connection, snapshot, guestCast }}
          isVoiceWaiting={voice.tap.isWaiting}
          credential={props.membership.credential}
        />
        <ComingAndGoing snapshot={snapshot} video={kept.video} />
        <TogetherPanel isHomeHidden={isCasting}>
          <TogetherNews snapshot={snapshot} />
          <TogetherDebug snapshot={snapshot} />
          <GuestsEntries place="panel" {...entries} />
          {/* The voices' tap is asked for over the stage, with the film's (ViewingArea). */}
          <VoiceControls {...voice} tap={{ ...voice.tap, isWaiting: false }} />
        </TogetherPanel>
      </ViewerPage>
    </SessionVoiceContext>
  );
}

/** Tells onGroupFilm each film the group plays, and 0 once one it played stops; unknown, nothing. */
function useFollowsTheGroupsFilm(
  snapshot: TogetherSnapshot | undefined,
  onGroupFilm: ((fileId: number) => void) | undefined,
): void {
  const fileId = snapshot?.group?.fileId;
  const hasPlayedRef = useRef(false);
  useEffect(() => {
    if (fileId !== undefined && (fileId !== 0 || hasPlayedRef.current)) {
      hasPlayedRef.current = true;
      onGroupFilm?.(fileId);
    }
  }, [fileId, onGroupFilm]);
}

/** Ends the viewer's session as the server ends it, with its reason. */
function useOverWhenEnded(
  snapshot: TogetherSnapshot | undefined,
  onOver: (reason: TogetherEndReason) => void,
): void {
  const endReason = snapshot?.status === "ended" ? snapshot.endReason : undefined;
  useEffect(() => {
    if (endReason !== undefined) {
      onOver(endReason);
    }
  }, [endReason, onOver]);
}

/**
 * The group's film followed on a cast's receiver that takes a guest's part in their session (D371 §4):
 * just another follower, by the guest's credential the cast handed it, as the guest's own device
 * followed it, black while nothing plays. Its page's press, Use, gave it its sound.
 */
export function FollowsTheGroupHere(props: { readonly credential: string }): JSX.Element {
  const token = useToken();
  const { connection, snapshot } = useTogether({
    credential: props.credential,
    sessionToken: token,
  });
  const kept = useKeptVideo();
  return (
    <GuestsMenusContext value={false}>
      <ViewingArea
        kept={kept}
        isVoiceWaiting={false}
        credential={props.credential}
        connection={connection}
        snapshot={snapshot}
      />
    </GuestsMenusContext>
  );
}

/**
 * Where the film plays: black while nothing does (NothingPlays), else the group's film, followed, in
 * the session's kept video; over it, a tap while the voices wait for one, as a phone call did (the
 * owner, 2026-10-13: "a small 'tap to re-enable sound' over the video"). While this device casts, its
 * receiver plays the film in its place, and this one holds the group's controls its rights give (D371
 * §4), telling its session it plays no film, so it never leads in its receiver's place (D374 §1).
 */
function ViewingArea(props: {
  readonly kept: KeptVideo;
  readonly isVoiceWaiting: boolean;
  readonly credential: string;
  readonly connection: (TogetherConnection & TellsCannotKeepUp) | undefined;
  readonly snapshot: TogetherSnapshot | undefined;
  /** A guest by link's cast to a Chromecast, which takes their part (D518), and their AirPlay. */
  readonly guestCast?: GuestsCast | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const fileId = props.snapshot?.group?.fileId ?? 0;
  const { connection } = props;
  const airPlayGrants = props.guestCast?.airPlayGrants;
  const { isCasting, stage } = useCastingTheGroupsFilm(props.snapshot, props.guestCast);
  useEffect(
    () => (isCasting && connection !== undefined ? tellsNoFilmHere(connection) : undefined),
    [isCasting, connection],
  );
  if (stage !== undefined) {
    return <CastingStage snapshot={props.snapshot} connection={connection} {...stage} />;
  }
  return (
    <section className="together-viewer__viewing" aria-label={t("together.stage")}>
      {fileId === 0 || connection === undefined ? (
        <NothingPlays snapshot={props.snapshot} isVoiceWaiting={props.isVoiceWaiting} />
      ) : (
        // A guest by link AirPlays by their membership's grants, whatever their controls (D519 §1).
        <AirPlayGrantsContext value={airPlayGrants ?? sessionAirPlayGrants}>
          <GroupFilm
            key={fileId}
            keptVideo={props.kept.video}
            isVoiceWaiting={props.isVoiceWaiting}
            fileId={fileId}
            credential={props.credential}
            following={{ connection }}
            isOfferedToAirPlay={airPlayGrants === undefined ? undefined : true}
          />
        </AirPlayGrantsContext>
      )}
    </section>
  );
}

/**
 * The stage while no film plays here: black (D246 §6), saying, once the session says it plays
 * nothing, that it waits for the host to start a film (the owner, 2026-10-30: a guest who joined
 * "won't know what to do"), with the voices' tap asked for over it while they wait for one.
 */
function NothingPlays(props: {
  readonly snapshot: TogetherSnapshot | undefined;
  readonly isVoiceWaiting: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { snapshot } = props;
  const isWaiting = snapshot?.status === "connected" && (snapshot.group?.fileId ?? 0) === 0;
  return (
    <>
      {isWaiting ? <p>{t("together.waitsForAFilm")}</p> : null}
      <EmptyStage>
        {props.isVoiceWaiting ? (
          // Its tap is the page's (playWaitingAtTaps): the Pill only asks for one.
          <SoundOnPill isForVoices onPress={() => undefined} />
        ) : null}
      </EmptyStage>
    </>
  );
}

/**
 * What a viewer's video follows: the session's connection. Its volume is the video's own, which the
 * player's 🔈 sets (D440 §2).
 */
interface Following {
  readonly connection: TogetherConnection & TellsCannotKeepUp;
}

/**
 * The group's film, asked for afresh each time it changes, playing as the server decided, in the one
 * player every page composes: following the group, with the Play page's controls as far as the
 * guest's rights give them (D271 §4, D555 §8), when it ends under its name as the Play page's is
 * (EndsUnderTheHeading; the owner, 2026-11-26), Tap for sound over it, its stalls told by the
 * membership, its sound and subtitles the guest's own (D471, IsGuestContext). A guest may not ask
 * why the host's file cannot be read: the player's own words are told.
 */
function GroupFilm(props: {
  readonly keptVideo: HTMLVideoElement;
  readonly isVoiceWaiting: boolean;
  readonly fileId: number;
  readonly credential: string;
  readonly following: Following;
  /** Whether the guest's player is offered to AirPlay whatever its controls (D519 §1). */
  readonly isOfferedToAirPlay?: boolean | undefined;
}): JSX.Element {
  const decision = useGroupFilmDecision(props);
  const headers = useMemo(() => membershipHeaders(props.credential), [props.credential]);
  // An update waits while the film shows, and reloads the page as soon as it stops.
  useShowsAFilm();
  const { connection } = props.following;
  // A film on its ladder starts where the group is; following it moves it on from there.
  const [ladderStartMs] = useState(() => connection.snapshot().group?.positionMs ?? 0);
  // The lower quality the viewer's player moved to once it was ready (D253), per playback.
  const [moved, setMoved] = useState<MovedDown | undefined>(undefined);
  const composition = guestsComposition(props.keptVideo, connection, props.isOfferedToAirPlay);
  return (
    <Loaded
      resource={decision}
      render={(value) => {
        const onLowerQuality: GuestFilm["onLowerQuality"] = (offer, audioTrackIndex) => {
          const positionMs = connection.snapshot().group?.positionMs ?? 0;
          const { mediaToken } = value;
          setMoved((already) => already ?? { mediaToken, offer, positionMs, audioTrackIndex });
        };
        return (
          <GuestFilmContext
            value={{
              fileId: props.fileId,
              following: props.following,
              isVoiceWaiting: props.isVoiceWaiting,
              decision: value,
              headers,
              onLowerQuality,
            }}
          >
            <GuestsPage titleName={value.titleName}>
              <GuestsCommands
                connection={connection}
                fileId={props.fileId}
                headers={headers}
                creditsStartMs={creditsStartOf(value.segments ?? [])}
              >
                <FilmPlayer
                  decision={value}
                  moved={moved}
                  ladderStartMs={ladderStartMs}
                  composition={composition}
                />
              </GuestsCommands>
            </GuestsPage>
          </GuestFilmContext>
        );
      }}
    />
  );
}

/**
 * The guest's player, composed as the Play page's (D555 §8): its own controls, never the browser's
 * (D369 §2), following the group. AirPlay is offered where the guest's rights give any control, as
 * a player's with controls is (D473), or whatever they give where isOfferedToAirPlay (D519 §1).
 */
function guestsComposition(
  keptVideo: HTMLVideoElement,
  connection: TogetherConnection,
  isOfferedToAirPlay: boolean | undefined,
): FilmComposition {
  const { canPlayPause, canSeek } = connection.snapshot().rights;
  return {
    hasControls: false,
    isAutoPlay: false,
    askAbout: undefined,
    over: FollowingGroup,
    under: UnderTheGuestsFilm,
    around: GuestsTracks,
    keptVideo,
    isOfferedToAirPlay: isOfferedToAirPlay ?? (canPlayPause || canSeek),
  };
}

/**
 * The guest's page: the film's name over its player, when it ends directly under the name as the
 * Play page's is (EndsUnderTheHeading); its sound and subtitles the guest's own (IsGuestContext), and
 * its menus drawn, as its own device draws them, unless a cast's receiver takes the guest's part and
 * says it draws none (FollowsTheGroupHere, GuestsMenusContext).
 */
function GuestsPage(props: {
  readonly titleName: string | undefined;
  readonly children: ReactNode;
}): JSX.Element {
  const isDrawn = use(GuestsMenusContext) ?? true;
  const name =
    props.titleName === undefined ? null : (
      <p className="together-viewer__title">{props.titleName}</p>
    );
  return (
    <EndsUnderTheHeading heading={name}>
      <IsGuestContext value={true}>
        <GuestsMenusContext value={isDrawn}>{props.children}</GuestsMenusContext>
      </IsGuestContext>
    </EndsUnderTheHeading>
  );
}

/** What the guest's pieces of the player need of its page: the film, the group, the membership. */
interface GuestFilm {
  readonly fileId: number;
  readonly following: Following;
  /** Whether the voices wait for a tap, which is asked for over the film with its own. */
  readonly isVoiceWaiting: boolean;
  readonly decision: PlaybackDecision;
  /** The membership's authorization, by which the viewer's player reports its stalls. */
  readonly headers: Readonly<Record<string, string>>;
  /** Moves the player to offer, its original having played audioTrackIndex (MovedDown). */
  readonly onLowerQuality: (offer: LadderOffer, audioTrackIndex: number | undefined) => void;
}

const GuestFilmContext = createContext<GuestFilm | undefined>(undefined);

/** The guest's page, as GroupFilm gives it to the player's pieces. */
function useGuestFilm(): GuestFilm {
  const film = use(GuestFilmContext);
  if (film === undefined) {
    throw new Error("a guest's piece of the player outside its GroupFilm");
  }
  return film;
}

/** Around the guest's stage: the guest's own tracks and timing, for its menus (GuestTracksProvider). */
function GuestsTracks(props: FilmParts & { readonly children: ReactNode }): JSX.Element {
  const film = useGuestFilm();
  return <GuestTracksProvider {...props} {...film} />;
}

/**
 * Under the guest's film: its stalls reported by the membership, the original's (useMovingDown), or
 * a ladder's word that its lowest cannot keep up (D247 §8). The guest's sound, subtitles and timing
 * are its menus' (GuestsTracks), none of them under the film (the owner, 2026-11-26).
 */
function GuestStalls(props: FilmParts): JSX.Element {
  const film = useGuestFilm();
  const { ladder } = props;
  return (
    <>
      {ladder === undefined ? (
        <ReportsStalls {...film} videoRef={props.videoRef} readLinkKbps={props.readLinkKbps} />
      ) : (
        <TellsCannotKeepUp
          connection={film.following.connection}
          cannotKeepUp={ladder.cannotKeepUp}
        />
      )}
    </>
  );
}

/**
 * Over the guest's film: the video following the group, Tap for sound, and the Play page's own
 * overlay (OverTheFilm, D555 §8) on its side, full screen and on a computer, drawing only the
 * controls the guest's link gives (CommandRightsContext, D555 §10): play and pause, the steps and
 * the timeline's seeks where it may, ⏮ and ⏭ and Up next where it may put on the episode before or
 * after (D471 §2, §3), full screen and the sound always. Upright, the same controls lie under the
 * picture (UnderTheGuestsFilm). The stage fills the window only in full screen, under the guest's
 * top bar out of it (D555 §9).
 */
function FollowingGroup(props: FilmParts): JSX.Element {
  const { fileId, following, isVoiceWaiting } = useGuestFilm();
  const muted = useFollowing(props.videoRef, fileId, following);
  const isTv = isOnTelevision();
  useStageFillingFromTheVideo(props.videoRef, isTv);
  const { canPlayPause } = use(CommandRightsContext);
  const moves = useGuestsMoves();
  return (
    <>
      {muted.isMuted || isVoiceWaiting ? (
        <SoundOnPill isForVoices={isVoiceWaiting} onPress={muted.turnSoundOn} />
      ) : null}
      <OverTheFilm
        {...props}
        offer={GuestsUpNext}
        moves={moves}
        shouldOpenFullScreen={isTv}
        hasPressesOnTheFilm={!isTv && canPlayPause}
      />
    </>
  );
}

/**
 * Under the guest's film: the Play page's bar on a device held upright out of full screen
 * (PageControls, D555 §8), with what the guest's link gives; then its stalls and its own tracks.
 */
function UnderTheGuestsFilm(props: FilmParts): JSX.Element {
  const moves = useGuestsMoves();
  return (
    <>
      <PageControls {...props} offer={GuestsUpNext} moves={moves} />
      <GuestStalls {...props} />
    </>
  );
}

/** A ladder's word that even its lowest quality cannot keep up, told the group (D247 §8). */
function TellsCannotKeepUp(props: {
  readonly connection: TogetherConnection & TellsCannotKeepUp;
  readonly cannotKeepUp: CannotKeepUp | undefined;
}): null {
  useToldCannotKeepUp(props.connection, props.cannotKeepUp);
  return null;
}

/** The original: its stalls reported by the membership (useMovingDown). */
function ReportsStalls(props: {
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly readLinkKbps: () => number | undefined;
  readonly decision: PlaybackDecision;
  readonly headers: Readonly<Record<string, string>>;
  readonly onLowerQuality: GuestFilm["onLowerQuality"];
}): null {
  useMovingDown(props.videoRef, props, props.readLinkKbps);
  return null;
}

/**
 * Reports a viewer's stalls and link (D247 §2), and hands on the lower quality a stall started, once
 * it is ready and the player still cannot keep up (D253), with the audio track the original plays
 * as it moves, which the lower quality starts with (ownTrackPlaying).
 */
function useMovingDown(
  videoRef: RefObject<HTMLVideoElement | null>,
  props: {
    readonly decision: PlaybackDecision;
    readonly headers: Readonly<Record<string, string>>;
    readonly onLowerQuality: GuestFilm["onLowerQuality"];
  },
  readLinkKbps: () => number | undefined,
): void {
  const { api } = useServices();
  const lowerQuality = useStallReports(api, videoRef, {
    mediaToken: props.decision.mediaToken,
    headers: props.headers,
    readLinkKbps,
  });
  const { onLowerQuality } = props;
  useEffect(() => {
    if (lowerQuality !== undefined) {
      onLowerQuality(lowerQuality, ownTrackPlaying(videoRef.current));
    }
  }, [lowerQuality, onLowerQuality, videoRef]);
}

/** Whether the film plays muted, the browser having refused its sound, and the tap that turns it on. */
interface MutedFilm {
  readonly isMuted: boolean;
  readonly turnSoundOn: () => void;
}

/**
 * Makes the video follow the group while it plays fileId, and says when it plays
 * muted: a browser refuses a film's sound that no recent tap started, and a guest's starts
 * long after their Join (playedMutedEvent).
 */
function useFollowing(
  videoRef: RefObject<HTMLVideoElement | null>,
  fileId: number,
  following: Following,
): MutedFilm {
  const { connection } = following;
  const [isMuted, setIsMuted] = useState(false);
  // The rights the welcome gave, known by the time a film is on: a link's never change.
  const { canPlayPause, canSeek } = connection.snapshot().rights;
  useDiagnosisNotes(videoRef, connection);
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    const onPlayedMuted = (): void => {
      setIsMuted(true);
    };
    video.addEventListener(playedMutedEvent, onPlayedMuted);
    const stopFollowing = followGroup(video, connection, {
      fileId,
      // A player's own presses never move to another episode: that is the Up next offer's.
      leads: { canPlayPause, canSeek, canNext: false },
      monotonicMs,
      page: pageThatPausesWhenHidden(),
      // On an AirPlay receiver its sync gives the receiver room (D522 §2).
      isOnWirelessTarget: () => isOnWirelessTarget(video),
    });
    return () => {
      stopFollowing();
      video.removeEventListener(playedMutedEvent, onPlayedMuted);
    };
  }, [videoRef, connection, fileId, canPlayPause, canSeek]);
  const turnSoundOn = useCallback(() => {
    const video = videoRef.current;
    if (video !== null) {
      soundOnAtTap(video, connection);
    }
    setIsMuted(false);
  }, [videoRef, connection]);
  // While the film plays muted, any tap on the page turns its sound on, as any tap plays the voices
  // the browser refused: an iPhone guest tapped once for the film and again for the voices (the
  // owner, 2026-10-13: "only needs to tap once to hear both video and voice").
  useEffect(() => {
    if (!isMuted) {
      return undefined;
    }
    document.addEventListener("click", turnSoundOn, true);
    return () => {
      document.removeEventListener("click", turnSoundOn, true);
    };
  }, [isMuted, turnSoundOn]);
  return { isMuted, turnSoundOn };
}

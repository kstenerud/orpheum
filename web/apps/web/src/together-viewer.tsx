// SPDX-License-Identifier: MPL-2.0

import "./together-viewer.css";

import {
  followGroup,
  playedMutedEvent,
  soundOnAtTap,
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
  useState,
  type JSX,
  type RefObject,
} from "react";
import { useTranslation } from "react-i18next";

import { secretFromHash } from "./accounts.ts";
import logo from "./assets/orpheum-logo.svg";
import { browserStorage } from "./browser-storage.ts";
import { pageThatPausesWhenHidden } from "./handheld.ts";
import { idempotent } from "./idempotent.ts";
import { JoinForm, NoLink } from "./together-join.tsx";
import { EmptyStage, FilmPlayer, type FilmParts, type MovedDown } from "./film-player.tsx";
import type { PlaybackDecision } from "./watching.ts";
import type { LadderOffer } from "./moving-down.ts";
import { useStallReports } from "./play-state.ts";
import { problemText, throwIfRefused } from "./problem.ts";
import { productName } from "./product-name.ts";
import { useDiagnosisNotes } from "./together-diagnosis.ts";
import { useShowsAFilm } from "./film-showing.ts";
import { haveBackMySessions } from "./together-mine.ts";
import { useMembership } from "./together-membership.ts";
import { showNotice } from "./notice.ts";
import { useServices, useToken } from "./services.tsx";
import { useLive } from "./live.tsx";
import { SessionRemote } from "./session-remote.tsx";
import { Loaded } from "./loaded.tsx";
import { TogetherNews } from "./together-news.tsx";
import { requestTogetherPlayback } from "./together-playback.ts";
import {
  TapToContinue,
  useKeptVideo,
  useViewerVoiceDevices,
  type KeptVideo,
} from "./together-viewer-tap.tsx";
import { useVoice, VoiceControls } from "./together-voice.tsx";
import {
  forgetMembership,
  keepMembership,
  membershipHeaders,
  readMembership,
  type Membership,
} from "./together-storage.ts";
import { monotonicMs, useTogether, useToldCannotKeepUp, useToldStanding } from "./use-together.ts";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";
import { VolumeRange } from "./volume-range.tsx";
import { useIsVolumeSettable, VolumeButtonsNote } from "./film-volume-slider.tsx";

/**
 * A Watch Together viewer's page (D246 §3, §5, §6), for someone who need not be signed in: they
 * join by their link with a name everyone sees, asked nothing of how their device makes sound
 * (D336 §4), and then see only the host's playback, black while nothing plays, with their own
 * volume and Leave. The link's secret is after #, and leaves the address once used.
 */
export function TogetherViewer(): JSX.Element {
  const { api, navigation } = useServices();
  const token = useToken();
  const [secret, setSecret] = useState(() => secretFromHash(navigation.fragment()));
  const kept = useMembership(browserStorage, "viewer");
  // The membership this browser keeps, unless a link other than the one it joined by was opened:
  // the same link opened again comes back as the same member (D267 §1).
  const membership = secret === undefined || kept?.link === secret ? kept : undefined;
  const { endReason, over } = useSessionEnd(token !== undefined);
  // Made before the Join, so its tap, and any tap after, lets the film and the voices play with
  // sound: one tap gives a guest both, whichever starts first (the owner, 2026-10-13).
  const keptVideo = useKeptVideo();
  const [hasJoinedHere, setHasJoinedHere] = useState(false);
  const devices = useViewerVoiceDevices();
  useEffect(() => {
    // Signed in, a session joined on another device follows them here (D267 §2).
    if (token !== undefined && secret === undefined) {
      void haveBackMySessions(api).catch(() => undefined);
    }
  }, [api, token, secret]);
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
      return <NoLink />;
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
          // The secret is spent: it stays in neither the address nor the history (D243 §3).
          navigation.replace({ screen: "together" });
          setSecret(undefined);
        }}
      />
    );
  }
  return (
    <ViewerSession
      membership={membership}
      hasJoinedHere={hasJoinedHere}
      kept={keptVideo}
      devices={devices}
      onOver={over.end}
    />
  );
}

/**
 * A link pasted into a tab already here changes only what follows #, and the page does not load
 * again: the new link is joined in place of whatever this browser was in, unless it is the same.
 */
function usePastedLinks(setSecret: (secret: string) => void, again: () => void): void {
  const { navigation } = useServices();
  useEffect(
    () =>
      navigation.subscribe(() => {
        const pasted = secretFromHash(navigation.fragment());
        if (pasted !== undefined) {
          if (readMembership(browserStorage(), "viewer")?.link !== pasted) {
            forgetMembership(browserStorage(), "viewer");
          }
          setSecret(pasted);
          again();
        }
      }),
    [navigation, setSecret, again],
  );
}

/**
 * Why the session is over for this viewer, if it is, and how it ends and starts again: ended, the
 * membership is forgotten, but for a viewer sent away for the host's upload, who is still a member
 * and may come back (D286 §5). A signed-in guest has the app to go back to: ended, they are taken
 * home, told why there, as a page that said only "You can close this page" stopped their app in
 * its tracks (the owner, 2026-10-29); a guest by link alone has nowhere else, and is told here.
 */
function useSessionEnd(isSignedIn: boolean): {
  readonly endReason: TogetherEndReason | undefined;
  readonly over: { readonly end: (reason: TogetherEndReason) => void; readonly again: () => void };
} {
  const { t } = useTranslation();
  const { navigation } = useServices();
  const [endReason, setEndReason] = useState<TogetherEndReason | undefined>(undefined);
  const over = useMemo(
    () => ({
      end: (reason: TogetherEndReason) => {
        if (reason === "uploadFull") {
          setEndReason(reason);
          return;
        }
        forgetMembership(browserStorage(), "viewer");
        if (isSignedIn) {
          // Home first, so the notice is first seen there, the screen it stays on (notice-note.tsx).
          navigation.replace({ screen: "home" });
          showNotice(t(`together.overNotice.${reason}`));
          return;
        }
        setEndReason(reason);
      },
      again: () => {
        setEndReason(undefined);
      },
    }),
    [isSignedIn, navigation, t],
  );
  return { endReason, over };
}

/**
 * Why the session is over for this viewer, and that the page has nothing more to show; or, sent away
 * because the host's connection could not carry everyone, a way back in (D286 §5).
 */
function SessionOver(props: {
  readonly reason: TogetherEndReason;
  readonly onAgain: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <main className="together-viewer">
      <output>{t(`together.over.${props.reason}`)}</output>
      {props.reason === "uploadFull" ? (
        <button type="button" onClick={props.onAgain}>
          {t("together.over.joinAgain")}
        </button>
      ) : null}
    </main>
  );
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
  /** Whether this page joined by the link itself, rather than came back to the membership. */
  readonly hasJoinedHere: boolean;
  readonly kept: KeptVideo;
  readonly devices: BrowserVoiceDevices;
  readonly onOver: (reason: TogetherEndReason) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [volume, setVolume] = useState(1);
  const { connection, snapshot } = useTogether({ credential: props.membership.credential });
  const { onOver, kept } = props;
  const isConnected = snapshot?.status === "connected";
  // Everyone in the session is heard from the start, once the page's own device is welcomed and the
  // page has had its tap; Talk only opens the microphone (D281 §1).
  const voice = useVoice(props.membership, undefined, {
    isReady: isConnected && kept.hasHadATap,
    devices: props.devices,
  });
  useOverWhenEnded(snapshot, onOver);
  useToldStanding(isConnected ? connection : undefined, standingOf(kept));
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
  return (
    <main className="together-viewer">
      <img className="together-viewer__logo" src={logo} alt={productName} />
      <h1 className="together-viewer__heading">{t("together.viewerHeading")}</h1>
      <ViewingArea
        kept={kept}
        isVoiceWaiting={voice.tap.isWaiting}
        credential={props.membership.credential}
        connection={connection}
        snapshot={snapshot}
        volume={volume}
      />
      <TogetherNews snapshot={snapshot} />
      {/* The voices' tap is asked for over the stage, with the film's (ViewingArea). */}
      <VoiceControls {...voice} tap={{ ...voice.tap, isWaiting: false }} />
      <VolumeSlider volume={volume} onChange={setVolume} />
      <LeaveButton membership={props.membership} onLeft={onOver} />
    </main>
  );
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

/** The viewer's own volume, which no one else hears (watch-together.md: per person). */
function VolumeSlider(props: {
  readonly volume: number;
  readonly onChange: (volume: number) => void;
}): JSX.Element {
  const { t } = useTranslation();
  if (!useIsVolumeSettable()) {
    return <VolumeButtonsNote />;
  }
  return (
    <label className="together-viewer__volume">
      {t("together.volume")} <VolumeRange value={props.volume} most={1} onChange={props.onChange} />
    </label>
  );
}

/** Leave: the membership ends, and with it every device of it. */
function LeaveButton(props: {
  readonly membership: Membership;
  readonly onLeft: (reason: TogetherEndReason) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const { isBusy, problem, runRequest } = useRequest((cause) => problemText(t, cause));
  return (
    <>
      <button
        type="button"
        disabled={isBusy}
        onClick={() => {
          runRequest(async (idempotencyKey) => {
            throwIfRefused(
              await api.POST("/together/leave", {
                headers: {
                  ...idempotent(idempotencyKey).headers,
                  ...membershipHeaders(props.membership.credential),
                },
              }),
            );
            props.onLeft("left");
          });
        }}
      >
        {t("together.leave")}
      </button>
      {problem === undefined ? null : <p role="alert">{problem}</p>}
    </>
  );
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
    <ViewingArea
      kept={kept}
      isVoiceWaiting={false}
      credential={props.credential}
      connection={connection}
      snapshot={snapshot}
      volume={1}
    />
  );
}

/**
 * Where the film plays: black while nothing does (D246 §6), else the group's film, followed, in the
 * session's kept video; over either, a tap asked for while the voices wait for one, as when a phone
 * call took the sound (the owner, 2026-10-13: "a small 'tap to re-enable sound' over the video").
 * While this device casts, the device it casts to plays the film in its place, a follower too, and
 * this one holds the group's controls its rights give (D371 §4).
 */
function ViewingArea(props: {
  readonly kept: KeptVideo;
  readonly isVoiceWaiting: boolean;
  readonly credential: string;
  readonly connection: (TogetherConnection & TellsCannotKeepUp) | undefined;
  readonly snapshot: TogetherSnapshot | undefined;
  readonly volume: number;
}): JSX.Element {
  const { t } = useTranslation();
  const { sending } = useLive();
  const fileId = props.snapshot?.group?.fileId ?? 0;
  const { connection } = props;
  if (sending?.isAccepted === true && fileId !== 0) {
    return (
      <section className="together-viewer__viewing" aria-label={t("together.stage")}>
        <p>{t("cast.playingOn", { name: sending.receiverName })}</p>
        <SessionRemote snapshot={props.snapshot} connection={connection} durationMs={0} />
      </section>
    );
  }
  return (
    <section className="together-viewer__viewing" aria-label={t("together.stage")}>
      {fileId === 0 || connection === undefined ? (
        <EmptyStage>
          {props.isVoiceWaiting ? (
            // Its tap is the page's (playWaitingAtTaps): the button only asks for one.
            <button type="button" className="together-viewer__sound-on">
              {t("together.soundBack")}
            </button>
          ) : null}
        </EmptyStage>
      ) : (
        <GroupFilm
          key={fileId}
          keptVideo={props.kept.video}
          isVoiceWaiting={props.isVoiceWaiting}
          fileId={fileId}
          credential={props.credential}
          following={{ connection, volume: props.volume }}
        />
      )}
    </section>
  );
}

/** What a viewer's video follows: the session's connection, at the viewer's own volume. */
interface Following {
  readonly connection: TogetherConnection & TellsCannotKeepUp;
  readonly volume: number;
}

/**
 * The group's film, asked for afresh each time it changes, playing as the server decided, in the one
 * player every page composes: following the group, with the player's own controls where the guest's
 * rights give them (D271 §4), Tap for sound and Full screen over it, and its stalls told by the
 * membership. A guest may not ask why the host's file cannot be read: the player's own words are
 * told.
 */
function GroupFilm(props: {
  readonly keptVideo: HTMLVideoElement;
  readonly isVoiceWaiting: boolean;
  readonly fileId: number;
  readonly credential: string;
  readonly following: Following;
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
  const { canPlayPause, canSeek } = connection.snapshot().rights;
  return (
    <Loaded
      resource={decision}
      render={(value) => {
        const onLowerQuality = (offer: LadderOffer): void => {
          const positionMs = connection.snapshot().group?.positionMs ?? 0;
          setMoved((already) => already ?? { mediaToken: value.mediaToken, offer, positionMs });
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
            {value.titleName === undefined ? null : (
              <p className="together-viewer__title">{value.titleName}</p>
            )}
            <FilmPlayer
              decision={value}
              moved={moved}
              ladderStartMs={ladderStartMs}
              composition={{
                hasControls: canPlayPause || canSeek,
                isAutoPlay: false,
                askAbout: undefined,
                over: FollowingGroup,
                under: GuestStalls,
                keptVideo: props.keptVideo,
              }}
            />
          </GuestFilmContext>
        );
      }}
    />
  );
}

/** How the group's film plays for this guest, as the server decides, asked for once per film. */
function useGroupFilmDecision(film: {
  readonly fileId: number;
  readonly credential: string;
}): ReturnType<typeof useResource<PlaybackDecision>>[0] {
  const { api, playableFormats } = useServices();
  const [decision] = useResource(`together-play/${String(film.fileId)}`, () =>
    requestTogetherPlayback(api, film.credential, { playableFormats: playableFormats() }),
  );
  return decision;
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
  readonly onLowerQuality: (offer: LadderOffer) => void;
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

/**
 * Under the guest's film: its stalls reported by the membership, the original's
 * (useMovingDown), or a ladder's word that even its lowest cannot keep up (D247 §8).
 */
function GuestStalls(props: FilmParts): JSX.Element {
  const film = useGuestFilm();
  const { ladder } = props;
  return ladder === undefined ? (
    <ReportsStalls {...film} videoRef={props.videoRef} readLinkKbps={props.readLinkKbps} />
  ) : (
    <TellsCannotKeepUp connection={film.following.connection} cannotKeepUp={ladder.cannotKeepUp} />
  );
}

/** Over the guest's film: the video following the group, and Tap for sound and Full screen. */
function FollowingGroup(props: FilmParts): JSX.Element {
  const { fileId, following, isVoiceWaiting } = useGuestFilm();
  const muted = useFollowing(props.videoRef, fileId, following);
  return <FilmButtons {...muted} isVoiceWaiting={isVoiceWaiting} videoRef={props.videoRef} />;
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
  readonly onLowerQuality: (offer: LadderOffer) => void;
}): null {
  useMovingDown(props.videoRef, props, props.readLinkKbps);
  return null;
}

/**
 * Reports a viewer's stalls and link (D247 §2), and hands on the lower quality a stall started, once
 * it is ready and the player still cannot keep up (D253).
 */
function useMovingDown(
  videoRef: RefObject<HTMLVideoElement | null>,
  props: {
    readonly decision: PlaybackDecision;
    readonly headers: Readonly<Record<string, string>>;
    readonly onLowerQuality: (offer: LadderOffer) => void;
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
      onLowerQuality(lowerQuality);
    }
  }, [lowerQuality, onLowerQuality]);
}

/**
 * Whether the film plays muted, the browser having refused its sound, and the tap that turns it on;
 * and whether the viewer has the player's own controls, which their link's rights give (D271 §4).
 */
interface MutedFilm {
  readonly isMuted: boolean;
  readonly turnSoundOn: () => void;
  readonly hasControls: boolean;
}

/**
 * Makes the video follow the group while it plays fileId, at the viewer's volume, and says when it
 * plays muted: a browser refuses a film's sound that no recent tap started, and a guest's starts
 * long after their Join (playedMutedEvent).
 */
function useFollowing(
  videoRef: RefObject<HTMLVideoElement | null>,
  fileId: number,
  following: Following,
): MutedFilm {
  const { connection, volume } = following;
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
      leads: { canPlayPause, canSeek },
      monotonicMs,
      page: pageThatPausesWhenHidden(),
    });
    return () => {
      stopFollowing();
      video.removeEventListener(playedMutedEvent, onPlayedMuted);
    };
  }, [videoRef, connection, fileId, canPlayPause, canSeek]);
  useEffect(() => {
    const video = videoRef.current;
    if (video !== null) {
      video.volume = volume;
    }
  }, [videoRef, volume]);
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
  return { isMuted, turnSoundOn, hasControls: canPlayPause || canSeek };
}

/**
 * Over a film the browser would only play muted, or while the voices wait for a tap, the tap that
 * turns the sound back on, small, at the film's top (the voices' is the page's, playWaitingAtTaps);
 * and for a viewer without the player's own controls, Full screen, which every viewer has.
 */
function FilmButtons(
  props: MutedFilm & {
    readonly isVoiceWaiting: boolean;
    readonly videoRef: RefObject<HTMLVideoElement | null>;
  },
): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      {props.isMuted || props.isVoiceWaiting ? (
        <button type="button" className="together-viewer__sound-on" onClick={props.turnSoundOn}>
          {t("together.soundBack")}
        </button>
      ) : null}
      {props.hasControls ? null : (
        <button
          type="button"
          className="together-viewer__fullscreen"
          onClick={() => {
            enterFullscreen(props.videoRef.current);
          }}
        >
          {t("together.fullscreen")}
        </button>
      )}
    </>
  );
}

/** A video that has its own fullscreen, as an iPhone's alone does: requestFullscreen it has not. */
interface WebKitVideo {
  readonly webkitEnterFullscreen?: () => void;
}

/** Shows video full screen: by the standard way where the browser has it, else the iPhone's. */
function enterFullscreen(video: (HTMLVideoElement & WebKitVideo) | null): void {
  if (video === null) {
    return;
  }
  if (typeof video.requestFullscreen === "function") {
    void video.requestFullscreen().catch(() => undefined);
  } else {
    video.webkitEnterFullscreen?.();
  }
}

// SPDX-License-Identifier: MPL-2.0

import "./together-viewer.css";

import {
  followGroup,
  playedMutedEvent,
  soundOnAtTap,
  type CannotKeepUp,
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

import { secretFromHash, type AudioOutput } from "./accounts.ts";
import logo from "./assets/orpheum-logo.svg";
import { browserStorage } from "./browser-storage.ts";
import {
  CredentialsField,
  CredentialsForm,
  CredentialsNotice,
  CredentialsProblem,
} from "./credentials-form.tsx";
import { idempotent } from "./idempotent.ts";
import { EmptyStage, FilmPlayer, type FilmParts, type MovedDown } from "./film-player.tsx";
import type { PlaybackDecision } from "./watching.ts";
import type { LadderOffer } from "./moving-down.ts";
import { useStallReports } from "./play-state.ts";
import { dataOrThrow, fieldProblemText, problemText, throwIfRefused } from "./problem.ts";
import { productName } from "./product-name.ts";
import { useDiagnosisNotes } from "./together-diagnosis.ts";
import { useShowsAFilm } from "./film-showing.ts";
import { haveBackMySessions } from "./together-mine.ts";
import { useMembership } from "./together-membership.ts";
import { useServices, useToken } from "./services.tsx";
import { SoundChoices, SoundLine } from "./sound.tsx";
import { Loaded } from "./title.tsx";
import { TogetherNews } from "./together-news.tsx";
import { requestTogetherPlayback } from "./together-playback.ts";
import { useVoice, VoiceControls } from "./together-voice.tsx";
import {
  forgetMembership,
  keepMembership,
  keepViewerAudioOutput,
  membershipHeaders,
  readMembership,
  readViewerAudioOutput,
  startLagMemory,
  type Membership,
} from "./together-storage.ts";
import { monotonicMs, useTogether, useToldCannotKeepUp } from "./use-together.ts";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";
import { VolumeRange } from "./volume-range.tsx";

/**
 * A Watch Together viewer's page (D246 §3, §5, §6), for someone who need not be signed in: they
 * join by their link with a name everyone sees, say how their device makes sound unless this
 * browser already knows, and then see only the host's playback, black while nothing plays, with
 * their own volume and Leave. The link's secret is after #, and leaves the address once used.
 */
export function TogetherViewer(): JSX.Element {
  const { api, navigation } = useServices();
  const token = useToken();
  const [secret, setSecret] = useState(() => secretFromHash(navigation.fragment()));
  const kept = useMembership(browserStorage, "viewer");
  // The membership this browser keeps, unless a link other than the one it joined by was opened:
  // the same link opened again comes back as the same member (D267 §1).
  const membership = secret === undefined || kept?.link === secret ? kept : undefined;
  const { endReason, over } = useSessionEnd();
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
  // A link pasted into a tab already here changes only what follows #, and the page does not load
  // again: the new link is joined in place of whatever this browser was in, unless it is the same.
  useEffect(
    () =>
      navigation.subscribe(() => {
        const pasted = secretFromHash(navigation.fragment());
        if (pasted !== undefined) {
          if (readMembership(browserStorage(), "viewer")?.link !== pasted) {
            forgetMembership(browserStorage(), "viewer");
          }
          setSecret(pasted);
          over.again();
        }
      }),
    [navigation, over],
  );
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
  return <ViewerSession membership={membership} onOver={over.end} />;
}

/** An address with no link in it, and no session this tab is in: only what to do instead. */
function NoLink(): JSX.Element {
  const { t } = useTranslation();
  return (
    <CredentialsNotice heading={t("together.joinHeading")}>
      <p>{t("together.noLink")}</p>
    </CredentialsNotice>
  );
}

/**
 * Joining by a link: the name the others will see, and Join (watch-together.md: one Join). Someone
 * signed in here finds their own name already filled in, theirs to change (the owner, 2026-10-06).
 */
function JoinForm(props: {
  readonly secret: string;
  readonly onJoined: (membership: Membership) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api, canFollowAGroup } = useServices();
  const token = useToken();
  // What was typed, once anything was: a late answer about who is signed in never replaces it.
  const [typedName, setTypedName] = useState<string | undefined>(undefined);
  const [me] = useResource(`join-name:${token ?? ""}`, async () =>
    token === undefined ? undefined : dataOrThrow(await api.GET("/me")),
  );
  const name = typedName ?? (me.state === "loaded" ? me.value?.name : undefined) ?? "";
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, {
      fields: { "body.name": t("together.name") },
      overrides: { notFound: t("together.badLink") },
    }),
  );
  const { secret } = props;
  if (!canFollowAGroup()) {
    // Told before joining, so no one waits for a device that could never keep up (D250 §7).
    return (
      <CredentialsNotice heading={t("together.joinHeading")}>
        <p role="alert">{t("together.cannotFollow")}</p>
      </CredentialsNotice>
    );
  }
  return (
    <CredentialsForm
      heading={t("together.joinHeading")}
      onSubmit={() => {
        runRequest(async (idempotencyKey) => {
          const joined = dataOrThrow(
            await api.POST("/together/join", {
              body: { secret, name },
              ...idempotent(idempotencyKey),
            }),
          );
          props.onJoined({ sessionId: joined.sessionId, credential: joined.membershipCredential });
        });
      }}
    >
      <p>{t("together.joinExplain")}</p>
      <CredentialsField label={t("together.name")}>
        <input
          required
          maxLength={100}
          autoComplete="nickname"
          value={name}
          onChange={(event) => {
            setTypedName(event.target.value);
          }}
        />
      </CredentialsField>
      {problem === undefined ? null : <CredentialsProblem isAlert>{problem}</CredentialsProblem>}
      <button type="submit" disabled={isBusy}>
        {t("together.join")}
      </button>
    </CredentialsForm>
  );
}

/**
 * Why the session is over for this viewer, if it is, and how it ends and starts again: ended, the
 * membership is forgotten, but for a viewer sent away for the host's upload, who is still a member
 * and may come back (D286 §5).
 */
function useSessionEnd(): {
  readonly endReason: TogetherEndReason | undefined;
  readonly over: { readonly end: (reason: TogetherEndReason) => void; readonly again: () => void };
} {
  const [endReason, setEndReason] = useState<TogetherEndReason | undefined>(undefined);
  const over = useMemo(
    () => ({
      end: (reason: TogetherEndReason) => {
        if (reason !== "uploadFull") {
          forgetMembership(browserStorage(), "viewer");
        }
        setEndReason(reason);
      },
      again: () => {
        setEndReason(undefined);
      },
    }),
    [],
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

/**
 * A viewer in the session: the speaker question first unless this browser has the answer, then the
 * playback; named events and who is here; the volume and Leave. Nobody waits for the question
 * (D246 §6): until the player is ready it holds no one up.
 */
function ViewerSession(props: {
  readonly membership: Membership;
  readonly onOver: (reason: TogetherEndReason) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [audioOutput, setAudioOutput] = useState(() => readViewerAudioOutput(browserStorage()));
  const [volume, setVolume] = useState(1);
  const { connection, snapshot } = useTogether({ credential: props.membership.credential });
  // Everyone in the session is heard from the start, once the page's own device is welcomed; Talk
  // only opens the microphone (D281 §1).
  const voice = useVoice(props.membership, undefined, snapshot?.status === "connected");
  const { onOver } = props;
  const keep = (output: AudioOutput): void => {
    keepViewerAudioOutput(browserStorage(), output);
    setAudioOutput(output);
  };
  const endReason = snapshot?.status === "ended" ? snapshot.endReason : undefined;
  useEffect(() => {
    if (endReason !== undefined) {
      onOver(endReason);
    }
  }, [endReason, onOver]);
  return (
    <main className="together-viewer">
      <img className="together-viewer__logo" src={logo} alt={productName} />
      <h1 className="together-viewer__heading">{t("together.viewerHeading")}</h1>
      {audioOutput === undefined ? (
        <SoundChoices current={undefined} isBusy={false} problem={undefined} onChosen={keep} />
      ) : (
        <>
          <ViewingArea
            membership={props.membership}
            audioOutput={audioOutput}
            connection={connection}
            snapshot={snapshot}
            volume={volume}
          />
          <SoundLine current={audioOutput} onChanged={() => undefined} keep={keep} />
        </>
      )}
      <TogetherNews snapshot={snapshot} />
      <VoiceControls {...voice} audioOutput={audioOutput} />
      <VolumeSlider volume={volume} onChange={setVolume} />
      <LeaveButton membership={props.membership} onLeft={onOver} />
    </main>
  );
}

/** The viewer's own volume, which no one else hears (watch-together.md: per person). */
function VolumeSlider(props: {
  readonly volume: number;
  readonly onChange: (volume: number) => void;
}): JSX.Element {
  const { t } = useTranslation();
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

/** Where the film plays: black while nothing does (D246 §6), else the group's film, followed. */
function ViewingArea(props: {
  readonly membership: Membership;
  readonly audioOutput: AudioOutput;
  readonly connection: (TogetherConnection & TellsCannotKeepUp) | undefined;
  readonly snapshot: TogetherSnapshot | undefined;
  readonly volume: number;
}): JSX.Element {
  const { t } = useTranslation();
  const fileId = props.snapshot?.group?.fileId ?? 0;
  const { connection } = props;
  return (
    <section className="together-viewer__viewing" aria-label={t("together.stage")}>
      {fileId === 0 || connection === undefined ? (
        <EmptyStage />
      ) : (
        <GroupFilm
          key={`${String(fileId)}/${props.audioOutput}`}
          fileId={fileId}
          credential={props.membership.credential}
          audioOutput={props.audioOutput}
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
  readonly fileId: number;
  readonly credential: string;
  readonly audioOutput: AudioOutput;
  readonly following: Following;
}): JSX.Element {
  const { api, playableFormats } = useServices();
  const [decision] = useResource(`together-play/${String(props.fileId)}/${props.audioOutput}`, () =>
    requestTogetherPlayback(api, props.credential, {
      playableFormats: playableFormats(),
      audioOutput: props.audioOutput,
    }),
  );
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
              }}
            />
          </GuestFilmContext>
        );
      }}
    />
  );
}

/** What the guest's pieces of the player need of its page: the film, the group, the membership. */
interface GuestFilm {
  readonly fileId: number;
  readonly following: Following;
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
 * Under the guest's film: its stalls reported by the membership, the original's or a prepared
 * copy's (useMovingDown), or a ladder's word that even its lowest cannot keep up (D247 §8).
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
  const { fileId, following } = useGuestFilm();
  const muted = useFollowing(props.videoRef, fileId, following);
  return <FilmButtons {...muted} videoRef={props.videoRef} />;
}

/** A ladder's word that even its lowest quality cannot keep up, told the group (D247 §8). */
function TellsCannotKeepUp(props: {
  readonly connection: TogetherConnection & TellsCannotKeepUp;
  readonly cannotKeepUp: CannotKeepUp | undefined;
}): null {
  useToldCannotKeepUp(props.connection, props.cannotKeepUp);
  return null;
}

/** The original or a prepared copy: its stalls reported by the membership (useMovingDown). */
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
      startLag: startLagMemory(browserStorage()),
      page: document,
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
  return { isMuted, turnSoundOn, hasControls: canPlayPause || canSeek };
}

/**
 * Over a film the browser would only play muted, the tap that turns its sound on; and for a viewer
 * without the player's own controls, Full screen, which every viewer has.
 */
function FilmButtons(
  props: MutedFilm & { readonly videoRef: RefObject<HTMLVideoElement | null> },
): JSX.Element {
  const { t } = useTranslation();
  return (
    <>
      {props.isMuted ? (
        <button type="button" className="together-viewer__sound-on" onClick={props.turnSoundOn}>
          {t("together.soundOn")}
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

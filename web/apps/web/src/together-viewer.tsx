// SPDX-License-Identifier: MPL-2.0

import "./together-viewer.css";

import { mediaErrorFailure, type PlaybackFailure } from "@orpheum/player";
import {
  followGroup,
  playedMutedEvent,
  soundOnAtTap,
  startingTuning,
  type TellsCannotKeepUp,
  type TogetherConnection,
  type TogetherEndReason,
  type TogetherSnapshot,
} from "@orpheum/sdk";
import { useCallback, useEffect, useMemo, useRef, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { secretFromHash, type AudioOutput } from "./accounts.ts";
import logo from "./assets/orpheum-logo.svg";
import { startingAudioTrack } from "./audio-tracks.tsx";
import { browserStorage } from "./browser-storage.ts";
import {
  CredentialsField,
  CredentialsForm,
  CredentialsNotice,
  CredentialsProblem,
} from "./credentials-form.tsx";
import { idempotent } from "./idempotent.ts";
import {
  LadderNotes,
  audioRenditionIndex,
  playbackFailureText,
  useBufferMeter,
  useChunkedPlayback,
  usePreparedPlayback,
} from "./play.tsx";
import type { PlaybackDecision } from "./watching.ts";
import type { LadderOffer } from "./moving-down.ts";
import { useStallReports } from "./play-state.ts";
import { dataOrThrow, fieldProblemText, problemText, throwIfRefused } from "./problem.ts";
import { productName } from "./product-name.ts";
import { haveBackMySessions } from "./together-mine.ts";
import { useMembership } from "./together-membership.ts";
import { useServices, useToken } from "./services.tsx";
import { SoundChoices, SoundLine } from "./sound.tsx";
import { Loaded } from "./title.tsx";
import { TogetherNews } from "./together-news.tsx";
import { requestTogetherPlayback } from "./together-playback.ts";
import { VoiceControls } from "./together-voice.tsx";
import {
  forgetMembership,
  keepMembership,
  keepViewerAudioOutput,
  membershipHeaders,
  readMembership,
  readViewerAudioOutput,
  type Membership,
} from "./together-storage.ts";
import { monotonicMs, useTogether, useToldCannotKeepUp } from "./use-together.ts";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

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
  const [endReason, setEndReason] = useState<TogetherEndReason | undefined>(undefined);
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
          setEndReason(undefined);
        }
      }),
    [navigation],
  );
  if (endReason !== undefined) {
    return <SessionOver reason={endReason} />;
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
  return (
    <ViewerSession
      membership={membership}
      onOver={(reason) => {
        forgetMembership(browserStorage(), "viewer");
        setEndReason(reason);
      }}
    />
  );
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

/** Why the session is over for this viewer, and that the page has nothing more to show. */
function SessionOver(props: { readonly reason: TogetherEndReason }): JSX.Element {
  const { t } = useTranslation();
  return (
    <main className="together-viewer">
      <output>{t(`together.over.${props.reason}`)}</output>
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
      <VoiceControls
        membership={props.membership}
        tuning={snapshot?.tuning ?? startingTuning}
        audioOutput={audioOutput}
      />
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
      {t("together.volume")}{" "}
      <input
        type="range"
        min={0}
        max={100}
        value={Math.round(props.volume * 100)}
        onChange={(event) => {
          props.onChange(Number(event.target.value) / 100);
        }}
      />
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
    <section className="together-viewer__stage" aria-label={t("together.stage")}>
      {fileId === 0 || connection === undefined ? null : (
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

/** The group's film, asked for afresh each time it changes, playing as the server decided. */
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
  // The lower quality the viewer's player moved to once it was ready (D253), per playback.
  const [moved, setMoved] = useState<
    { readonly mediaToken: string; readonly offer: LadderOffer } | undefined
  >(undefined);
  return (
    <Loaded
      resource={decision}
      render={(value) => {
        const films = {
          fileId: props.fileId,
          decision: value,
          following: props.following,
          headers,
        };
        const onLowerQuality = (offer: LadderOffer): void => {
          setMoved((already) => already ?? { mediaToken: value.mediaToken, offer });
        };
        let film: JSX.Element;
        const onLadder = value.onLadder ?? undefined;
        if (moved?.mediaToken === value.mediaToken) {
          film = <ChunkedFilm {...films} offer={moved.offer} />;
        } else if (onLadder !== undefined) {
          // A film cut into chunks plays from its ladder from the start (D260).
          film = <ChunkedFilm {...films} offer={onLadder} isOnLadder />;
        } else if (value.delivery === "direct") {
          film = <DirectFilm {...films} onLowerQuality={onLowerQuality} />;
        } else {
          film = <PreparedFilm {...films} onLowerQuality={onLowerQuality} />;
        }
        return (
          <>
            {value.titleName === undefined ? null : (
              <p className="together-viewer__title">{value.titleName}</p>
            )}
            {film}
          </>
        );
      }}
    />
  );
}

/** What a viewer's film is given: the film, how it plays, the group it follows, and its membership. */
interface FilmProps {
  readonly fileId: number;
  readonly decision: PlaybackDecision;
  readonly following: Following;
  /** The membership's authorization, by which the viewer's player reports its stalls. */
  readonly headers: Readonly<Record<string, string>>;
}

/**
 * Reports a viewer's stalls and link (D247 §2), and hands on the lower quality a stall started, once
 * it is ready and the player still cannot keep up (D253).
 */
function useMovingDown(
  videoRef: RefObject<HTMLVideoElement | null>,
  props: FilmProps & { readonly onLowerQuality: (offer: LadderOffer) => void },
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

/** Whether the film plays muted, the browser having refused its sound, and the tap that turns it on. */
interface MutedFilm {
  readonly isMuted: boolean;
  readonly turnSoundOn: () => void;
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
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    const onPlayedMuted = (): void => {
      setIsMuted(true);
    };
    video.addEventListener(playedMutedEvent, onPlayedMuted);
    const stopFollowing = followGroup(video, connection, { fileId, isLeading: false, monotonicMs });
    return () => {
      stopFollowing();
      video.removeEventListener(playedMutedEvent, onPlayedMuted);
    };
  }, [videoRef, connection, fileId]);
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
  return { isMuted, turnSoundOn };
}

/** Over a film the browser would only play muted, the tap that turns its sound on. */
function SoundOnButton(props: MutedFilm): JSX.Element | null {
  const { t } = useTranslation();
  return props.isMuted ? (
    <button type="button" className="together-viewer__sound-on" onClick={props.turnSoundOn}>
      {t("together.soundOn")}
    </button>
  ) : null;
}

/** The original file, following the group; it plays when the group does, never on its own. */
function DirectFilm(
  props: FilmProps & { readonly onLowerQuality: (offer: LadderOffer) => void },
): JSX.Element {
  const { t } = useTranslation();
  const videoRef = useRef<HTMLVideoElement>(null);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const muted = useFollowing(videoRef, props.fileId, props.following);
  useMovingDown(videoRef, props, useBufferMeter(videoRef, props.decision.fileBitRateKbps));
  return (
    <>
      {/* eslint-disable-next-line jsx-a11y-x/media-has-caption -- subtitles for a viewer come later; the host's choice of track is not theirs (watch-together.md) */}
      <video
        className="together-viewer__video"
        ref={videoRef}
        src={props.decision.url}
        playsInline
        onError={(event) => {
          setFailure(
            t("play.directFailed", {
              reason: playbackFailureText(t, mediaErrorFailure(event.currentTarget.error)),
            }),
          );
        }}
      />
      <SoundOnButton {...muted} />
      {failure === undefined ? null : <p role="alert">{failure}</p>}
    </>
  );
}

/** A prepared copy, following the group once its first part is made. */
function PreparedFilm(
  props: FilmProps & { readonly onLowerQuality: (offer: LadderOffer) => void },
): JSX.Element {
  const { t } = useTranslation();
  const { decision } = props;
  const [startIndex] = useState(() =>
    audioRenditionIndex(
      decision,
      startingAudioTrack(decision)?.audioTrackIndex ?? 0,
      decision.preferences.isDialogueBoostOn,
    ),
  );
  // A viewer may not ask why the host's file cannot be read: the player's own words are told.
  const { videoRef, playbackRef, isWaiting, failure } = usePreparedPlayback(
    undefined,
    decision,
    startIndex,
  );
  const muted = useFollowing(videoRef, props.fileId, props.following);
  useMovingDown(videoRef, props, () => playbackRef.current?.estimateKbps());
  return (
    <>
      {/* eslint-disable-next-line jsx-a11y-x/media-has-caption -- subtitles for a viewer come later; the host's choice of track is not theirs (watch-together.md) */}
      <video className="together-viewer__video" ref={videoRef} playsInline />
      <SoundOnButton {...muted} />
      {isWaiting && failure === undefined ? <p>{t("play.preparing")}</p> : null}
      {failure === undefined ? null : (
        <p role="alert">{t("play.preparedFailed", { reason: failure })}</p>
      )}
    </>
  );
}

/**
 * The film on its ladder, every quality of it cut into chunks, following the group: from the start
 * (D260), or moved to by the viewer's stalling player (D247 §2, §10), from the quality offered.
 */
function ChunkedFilm(
  props: Pick<FilmProps, "fileId" | "following"> & {
    readonly offer: LadderOffer;
    readonly isOnLadder?: boolean;
  },
): JSX.Element {
  const { t } = useTranslation();
  const videoRef = useRef<HTMLVideoElement>(null);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  const onFailed = useCallback(
    (seen: PlaybackFailure) => {
      setFailure(t("play.preparedFailed", { reason: playbackFailureText(t, seen) }));
    },
    [t],
  );
  // It starts where the group is; following it moves it on from there.
  const [startMs] = useState(() => props.following.connection.snapshot().group?.positionMs ?? 0);
  const { playing, cannotKeepUp } = useChunkedPlayback(
    videoRef,
    { offer: props.offer, positionMs: startMs },
    onFailed,
  );
  const muted = useFollowing(videoRef, props.fileId, props.following);
  useToldCannotKeepUp(props.following.connection, cannotKeepUp);
  return (
    <>
      {/* eslint-disable-next-line jsx-a11y-x/media-has-caption -- subtitles for a viewer come later; the host's choice of track is not theirs (watch-together.md) */}
      <video className="together-viewer__video" ref={videoRef} playsInline />
      <SoundOnButton {...muted} />
      <LadderNotes
        offer={props.offer}
        playing={playing}
        cannotKeepUp={cannotKeepUp}
        isOnLadder={props.isOnLadder === true}
      />
      {failure === undefined ? null : <p role="alert">{failure}</p>}
    </>
  );
}

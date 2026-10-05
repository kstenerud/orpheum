// SPDX-License-Identifier: MPL-2.0

import "./together-viewer.css";

import { mediaErrorFailure } from "@orpheum/player";
import {
  followGroup,
  type TogetherConnection,
  type TogetherEndReason,
  type TogetherSnapshot,
} from "@orpheum/sdk";
import { useEffect, useRef, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { secretFromHash, type AudioOutput } from "./accounts.ts";
import { startingAudioTrack } from "./audio-tracks.tsx";
import { browserStorage, tabStorage } from "./browser-storage.ts";
import { CredentialsField, CredentialsForm, CredentialsProblem } from "./credentials-form.tsx";
import { idempotent } from "./idempotent.ts";
import {
  audioRenditionIndex,
  playbackFailureText,
  usePreparedPlayback,
  type PlaybackDecision,
} from "./play.tsx";
import { dataOrThrow, fieldProblemText, problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { SoundChoices } from "./sound.tsx";
import { Loaded } from "./title.tsx";
import { TogetherNews } from "./together-news.tsx";
import { requestTogetherPlayback } from "./together-playback.ts";
import {
  forgetMembership,
  keepMembership,
  keepViewerAudioOutput,
  membershipHeaders,
  readMembership,
  readViewerAudioOutput,
  type Membership,
} from "./together-storage.ts";
import { monotonicMs, useTogether } from "./use-together.ts";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";

/**
 * A Watch Together viewer's page (D246 §3, §5, §6), for someone who need not be signed in: they
 * join by their link with a name everyone sees, say how their device makes sound unless this
 * browser already knows, and then see only the host's playback, black while nothing plays, with
 * their own volume and Leave. The link's secret is after #, and leaves the address once used.
 */
export function TogetherViewer(): JSX.Element {
  const { navigation } = useServices();
  const [secret] = useState(() => secretFromHash(navigation.fragment()));
  const [membership, setMembership] = useState(() =>
    secret === undefined ? readMembership(tabStorage(), "viewer") : undefined,
  );
  const [endReason, setEndReason] = useState<TogetherEndReason | undefined>(undefined);
  if (endReason !== undefined) {
    return <SessionOver reason={endReason} />;
  }
  if (membership === undefined) {
    return (
      <JoinForm
        secret={secret}
        onJoined={(joined) => {
          keepMembership(tabStorage(), "viewer", joined);
          // The secret is spent: it stays in neither the address nor the history (D243 §3).
          navigation.replace({ screen: "together" });
          setMembership(joined);
        }}
      />
    );
  }
  return (
    <ViewerSession
      membership={membership}
      onOver={(reason) => {
        forgetMembership(tabStorage(), "viewer");
        setMembership(undefined);
        setEndReason(reason);
      }}
    />
  );
}

/** Joining by a link: the name the others will see, and Join (watch-together.md: one Join). */
function JoinForm(props: {
  readonly secret: string | undefined;
  readonly onJoined: (membership: Membership) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [name, setName] = useState("");
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    fieldProblemText(t, cause, {
      fields: { "body.name": t("together.name") },
      overrides: { notFound: t("together.badLink") },
    }),
  );
  const { secret } = props;
  return (
    <CredentialsForm
      heading={t("together.joinHeading")}
      onSubmit={() => {
        if (secret === undefined) {
          return;
        }
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
      <p>{t(secret === undefined ? "together.noLink" : "together.joinExplain")}</p>
      <CredentialsField label={t("together.name")}>
        <input
          required
          maxLength={100}
          autoComplete="nickname"
          value={name}
          onChange={(event) => {
            setName(event.target.value);
          }}
        />
      </CredentialsField>
      {problem === undefined ? null : <CredentialsProblem isAlert>{problem}</CredentialsProblem>}
      <button type="submit" disabled={secret === undefined || isBusy}>
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
  const endReason = snapshot?.status === "ended" ? snapshot.endReason : undefined;
  useEffect(() => {
    if (endReason !== undefined) {
      onOver(endReason);
    }
  }, [endReason, onOver]);
  return (
    <main className="together-viewer">
      <h1 className="together-viewer__heading">{t("together.viewerHeading")}</h1>
      {audioOutput === undefined ? (
        <SoundChoices
          current={undefined}
          isBusy={false}
          problem={undefined}
          onChosen={(output) => {
            keepViewerAudioOutput(browserStorage(), output);
            setAudioOutput(output);
          }}
        />
      ) : (
        <ViewingArea
          membership={props.membership}
          audioOutput={audioOutput}
          connection={connection}
          snapshot={snapshot}
          volume={volume}
        />
      )}
      <TogetherNews snapshot={snapshot} />
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
  readonly connection: TogetherConnection | undefined;
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
          key={fileId}
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
  readonly connection: TogetherConnection;
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
  const [decision] = useResource(`together-play/${String(props.fileId)}`, () =>
    requestTogetherPlayback(api, props.credential, {
      playableFormats: playableFormats(),
      audioOutput: props.audioOutput,
    }),
  );
  return (
    <Loaded
      resource={decision}
      render={(value) =>
        value.delivery === "direct" ? (
          <DirectFilm fileId={props.fileId} decision={value} following={props.following} />
        ) : (
          <PreparedFilm fileId={props.fileId} decision={value} following={props.following} />
        )
      }
    />
  );
}

/** Makes the video follow the group while it plays fileId, at the viewer's volume. */
function useFollowing(
  videoRef: RefObject<HTMLVideoElement | null>,
  fileId: number,
  following: Following,
): void {
  const { connection, volume } = following;
  useEffect(() => {
    const video = videoRef.current;
    return video === null
      ? undefined
      : followGroup(video, connection, { fileId, isLeading: false, monotonicMs });
  }, [videoRef, connection, fileId]);
  useEffect(() => {
    const video = videoRef.current;
    if (video !== null) {
      video.volume = volume;
    }
  }, [videoRef, volume]);
}

/** The original file, following the group; it plays when the group does, never on its own. */
function DirectFilm(props: {
  readonly fileId: number;
  readonly decision: PlaybackDecision;
  readonly following: Following;
}): JSX.Element {
  const { t } = useTranslation();
  const videoRef = useRef<HTMLVideoElement>(null);
  const [failure, setFailure] = useState<string | undefined>(undefined);
  useFollowing(videoRef, props.fileId, props.following);
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
      {failure === undefined ? null : <p role="alert">{failure}</p>}
    </>
  );
}

/** A prepared copy, following the group once its first part is made. */
function PreparedFilm(props: {
  readonly fileId: number;
  readonly decision: PlaybackDecision;
  readonly following: Following;
}): JSX.Element {
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
  const { videoRef, isWaiting, failure } = usePreparedPlayback(undefined, decision, startIndex);
  useFollowing(videoRef, props.fileId, props.following);
  return (
    <>
      {/* eslint-disable-next-line jsx-a11y-x/media-has-caption -- subtitles for a viewer come later; the host's choice of track is not theirs (watch-together.md) */}
      <video className="together-viewer__video" ref={videoRef} playsInline />
      {isWaiting && failure === undefined ? <p>{t("play.preparing")}</p> : null}
      {failure === undefined ? null : (
        <p role="alert">{t("play.preparedFailed", { reason: failure })}</p>
      )}
    </>
  );
}

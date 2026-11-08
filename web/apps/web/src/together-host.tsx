// SPDX-License-Identifier: MPL-2.0

import {
  everyRight,
  followGroup,
  playedMutedEvent,
  soundOnAtTap,
  type CannotKeepUp,
  type PlaybackReport,
  type PlaybackPicture,
  type PlaybackSound,
  type TogetherConnection,
  type TogetherEndReason,
} from "@orpheum/sdk";
import { useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { tabStorage } from "./browser-storage.ts";
import { pageThatPausesWhenHidden } from "./handheld.ts";
import { useToken } from "./services.tsx";
import { TogetherNews } from "./together-news.tsx";
import { useMembership } from "./together-membership.ts";
import { useHostsSessionOver } from "./together-session-end.ts";
import type { Membership } from "./together-storage.ts";
import { followFilmVolume } from "./film-volume.ts";
import { FilmVolumeSlider } from "./film-volume-slider.tsx";
import { ComingAndGoing, onTheFilmsStage, TogetherPanel } from "./together-over-the-film.tsx";
import { OnlyOnThePanelsPage } from "./together-panel-place.tsx";
import { useHostVoice, VoiceControls } from "./together-voice.tsx";
import { monotonicMs, useTogether, useToldCannotKeepUp } from "./use-together.ts";

/** What the host's player is given: the film, its video, and where it starts. */
interface HostPlayer {
  readonly fileId: number;
  /**
   * The video playing the film now, null while there is none: another when the player moves to a
   * lower quality, which the session follows without stopping the film for everyone.
   */
  readonly video: HTMLVideoElement | null;
  /** Where the film starts: where the host left it (play-state.md). */
  readonly startPositionMs: number;
  /** Whether even the host's lowest quality cannot keep up, which everyone is told (D247 §8). */
  readonly cannotKeepUp: CannotKeepUp | undefined;
  /** How the film is given to the player, which the session is told (D371 §4). */
  readonly delivery?: PlaybackReport["delivery"] | undefined;
}

/**
 * The host's player in their Watch Together session, while this tab hosts one (D246 §5): the film
 * the host starts plays for everyone; the host's own play, pause and seeks are the group's; the
 * host's video leads the session, playing as it would alone, and the viewers catch up to it (D371). Leaving the player
 * leaves the film playing for the group, on the server's clock, the host's session page holding its
 * controls; Stop there, or another film the host plays, ends it (D364 §15, withdrawing D246 §5's stop
 * on leaving). The session's end, however it ends, takes the page home, the film played on alone no
 * more (D389 §1). With no session it shows nothing and does nothing.
 */
export function HostPlaysTogether(props: HostPlayer): JSX.Element | null {
  // Started on this device or another of the host's, had back from the server (D267 §2).
  const membership = useMembership(tabStorage, "host");
  const over = useHostsSessionOver();
  if (membership === undefined) {
    return null;
  }
  return (
    <HostLeading
      key={membership.credential}
      {...props}
      membership={membership}
      onOver={(reason) => {
        over(membership.sessionId, reason);
      }}
    />
  );
}

/** A player leading its session: the host's in Watch Together, a cast's receiver (D88). */
interface LeadingPlayer {
  readonly connection: TogetherConnection | undefined;
  /** Whether the connection is welcomed, and so may put the film on. */
  readonly isWelcomed: boolean;
  readonly fileId: number;
  readonly video: HTMLVideoElement | null;
  /** Where the film is put on from while the video cannot yet say where it stands. */
  readonly startPositionMs: number;
  /** Where the film is put on from whatever the video says: where a cast's sender stood. */
  readonly putOnAtMs?: number | undefined;
  /** Whether the first session is told the film waits paused at putOnAtMs: a cast's sender had it so. */
  readonly isPutOnPaused?: boolean | undefined;
  /** How the film is given to the player, which the session is told (D371 §4). */
  readonly delivery?: PlaybackReport["delivery"] | undefined;
  /** How the film sounds now, which the session is told for a cast's remote (sound-panel.ts). */
  readonly readSound?: (() => PlaybackSound | undefined) | undefined;
  /** What the film shows that may be chosen, which the session is told for a cast's remote (picture-panel.ts). */
  readonly readPicture?: (() => PlaybackPicture | undefined) | undefined;
}

/**
 * Leads a session from this player: once welcomed, puts its film on for the session from where the
 * video stands, unless the session plays it already; the session's roster names this player its
 * leader, which plays as it would alone, the group following it, and carries out another device's
 * play, pause and seek as its own press; its own play, pause and seeks are the session's (D371);
 * the film plays at the tab's volume (D281 §5).
 */
export function useLeadingPlayer(leading: LeadingPlayer): void {
  const { connection, isWelcomed, fileId, video, startPositionMs, putOnAtMs, delivery } = leading;
  const { readSound, readPicture, isPutOnPaused } = leading;
  // The session it put its film on for: each it comes to lead, once (a cast's receiver taking its
  // host's part in a Watch Together session started while it played, D364 §11).
  const putOnForRef = useRef<TogetherConnection | undefined>(undefined);
  useEffect(() => {
    if (connection === undefined || !isWelcomed || putOnForRef.current === connection) {
      return;
    }
    const isFirst = putOnForRef.current === undefined;
    putOnForRef.current = connection;
    if (connection.snapshot().group?.fileId !== fileId) {
      putOn(connection, { fileId, video, startPositionMs, putOnAtMs, isPutOnPaused }, isFirst);
    }
  }, [connection, isWelcomed, fileId, video, startPositionMs, putOnAtMs, isPutOnPaused]);
  useEffect(
    () =>
      video === null || connection === undefined
        ? undefined
        : followGroup(video, connection, {
            fileId,
            leads: everyRight,
            monotonicMs,
            page: pageThatPausesWhenHidden(),
            delivery,
            readSound,
            readPicture,
          }),
    [connection, fileId, video, delivery, readSound, readPicture],
  );
  useEffect(() => (video === null ? undefined : followFilmVolume(video)), [video]);
}

/**
 * Puts the leading player's film on for the session connection: the first session it leads from
 * putOnAtMs, else where the video stands, else where the film starts, paused there where
 * isPutOnPaused (a cast started from a film paused, the owner, 2026-11-08: "It should maintain the
 * paused status of the caster"); a session it is led on into later, the same film playing on, from
 * where the video stands, and paused if it is.
 */
function putOn(
  connection: TogetherConnection,
  film: Pick<LeadingPlayer, "fileId" | "video" | "startPositionMs" | "putOnAtMs" | "isPutOnPaused">,
  isFirst: boolean,
): void {
  const { fileId, video, startPositionMs, putOnAtMs } = film;
  const standsAtMs = whereItStandsMs(video);
  const positionMs = isFirst
    ? (putOnAtMs ?? standsAtMs ?? startPositionMs)
    : (standsAtMs ?? putOnAtMs ?? startPositionMs);
  connection.command({ command: "changeItem", fileId, positionMs });
  if (isFirst ? film.isPutOnPaused === true : video?.paused === true) {
    connection.command({ command: "pause" });
  }
}

/** Where video stands, in ms, once it knows; undefined while there is none or it cannot yet say. */
function whereItStandsMs(video: HTMLVideoElement | null): number | undefined {
  const haveMetadata = 1; // HTMLMediaElement.HAVE_METADATA, which a plain page may not define
  return video !== null && video.readyState >= haveMetadata ? video.currentTime * 1000 : undefined;
}

/** The host's player, leading the session it is connected to. */
function HostLeading(
  props: HostPlayer & {
    readonly membership: Membership;
    readonly onOver: (reason: TogetherEndReason) => void;
  },
): JSX.Element {
  const { t } = useTranslation();
  const token = useToken();
  const { connection, snapshot } = useTogether({
    credential: props.membership.credential,
    sessionToken: token,
  });
  const { fileId, video, startPositionMs, onOver, delivery } = props;
  // The tab's one voice for the session, shared with the session's page (D281 §5).
  const hostVoice = useHostVoice();
  const isWelcomed = snapshot?.status === "connected";
  const endReason = snapshot?.status === "ended" ? snapshot.endReason : undefined;
  useLeadingPlayer({ connection, isWelcomed, fileId, video, startPositionMs, delivery });
  useToldCannotKeepUp(connection, props.cannotKeepUp);
  useEffect(() => {
    if (endReason !== undefined) {
      onOver(endReason);
    }
  }, [endReason, onOver]);
  return (
    <div className="together-host">
      {onTheFilmsStage(video, <SoundOn video={video} connection={connection} />)}
      <ComingAndGoing snapshot={snapshot} video={video} />
      <TogetherPanel>
        <OnlyOnThePanelsPage>
          <p>{t("together.hostPlaying")}</p>
        </OnlyOnThePanelsPage>
        <TogetherNews snapshot={snapshot} />
        {hostVoice === undefined ? null : <VoiceControls {...hostVoice} />}
        <OnlyOnThePanelsPage>
          <FilmVolumeSlider />
        </OnlyOnThePanelsPage>
      </TogetherPanel>
    </div>
  );
}

/**
 * The tap that turns the host's film's sound on, while the browser plays it muted: it refuses a
 * film's sound that no recent tap started, as after the host's page is loaded again, and the film
 * played on for the group with no sound and nothing said why (the owner's test, 2026-10-12).
 */
export function SoundOn(props: {
  readonly video: HTMLVideoElement | null;
  readonly connection: TogetherConnection | undefined;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { video, connection } = props;
  const [isMuted, setIsMuted] = useState(false);
  useEffect(() => {
    if (video === null) {
      return undefined;
    }
    const onPlayedMuted = (): void => {
      setIsMuted(true);
    };
    video.addEventListener(playedMutedEvent, onPlayedMuted);
    return () => {
      video.removeEventListener(playedMutedEvent, onPlayedMuted);
    };
  }, [video]);
  if (!isMuted || video === null || connection === undefined) {
    return null;
  }
  return (
    <button
      type="button"
      className="together-host__sound-on"
      onClick={() => {
        soundOnAtTap(video, connection);
        setIsMuted(false);
      }}
    >
      {t("together.soundOn")}
    </button>
  );
}

// SPDX-License-Identifier: MPL-2.0

import {
  everyRight,
  followGroup,
  playedMutedEvent,
  soundOnAtTap,
  type CannotKeepUp,
  type GroupCommand,
  type TogetherConnection,
} from "@orpheum/sdk";
import { useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { browserStorage, tabStorage } from "./browser-storage.ts";
import { useToken } from "./services.tsx";
import { TogetherNews } from "./together-news.tsx";
import { useMembership } from "./together-membership.ts";
import { forgetMembership, startLagMemory, type Membership } from "./together-storage.ts";
import { followFilmVolume } from "./film-volume.ts";
import { FilmVolumeSlider } from "./film-volume-slider.tsx";
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
}

/**
 * The host's player in their Watch Together session, while this tab hosts one (D246 §5): the film
 * the host starts plays for everyone; the host's own play, pause and seeks are the group's; the
 * host's video waits for the viewers only at the group's sync points (D250 §2); leaving the player
 * stops it for everyone. With no
 * session it shows nothing and does nothing.
 */
export function HostPlaysTogether(props: HostPlayer): JSX.Element | null {
  // Started on this device or another of the host's, had back from the server (D267 §2).
  const membership = useMembership(tabStorage, "host");
  if (membership === undefined) {
    return null;
  }
  return (
    <HostLeading
      key={membership.credential}
      {...props}
      membership={membership}
      onOver={() => {
        forgetMembership(tabStorage(), "host");
      }}
    />
  );
}

/** The host's stop, sent as they leave the player: every screen goes black (D246 §5). */
const stopForEveryone: GroupCommand = { command: "stop" };

/** The host's player, leading the session it is connected to. */
function HostLeading(
  props: HostPlayer & { readonly membership: Membership; readonly onOver: () => void },
): JSX.Element {
  const { t } = useTranslation();
  const token = useToken();
  const { connection, snapshot } = useTogether({
    credential: props.membership.credential,
    sessionToken: token,
    commandOnLeaving: stopForEveryone,
  });
  const { fileId, video, startPositionMs, onOver } = props;
  // The tab's one voice for the session, shared with the status tray's pop-up (D281 §5).
  const hostVoice = useHostVoice();
  const isWelcomed = snapshot?.status === "connected";
  const isOver = snapshot?.status === "ended" && snapshot.endReason !== undefined;
  const hasPutOnRef = useRef(false);
  useEffect(() => {
    // Once welcomed, this film is put on for everyone, from where it starts here.
    if (connection === undefined || !isWelcomed || hasPutOnRef.current) {
      return;
    }
    hasPutOnRef.current = true;
    if (connection.snapshot().group?.fileId !== fileId) {
      const positionMs =
        video !== null && video.readyState >= HTMLMediaElement.HAVE_METADATA
          ? video.currentTime * 1000
          : startPositionMs;
      connection.command({ command: "changeItem", fileId, positionMs });
    }
  }, [connection, isWelcomed, fileId, video, startPositionMs]);
  useEffect(
    () =>
      video === null || connection === undefined
        ? undefined
        : followGroup(video, connection, {
            fileId,
            leads: everyRight,
            monotonicMs,
            startLag: startLagMemory(browserStorage()),
            page: document,
          }),
    [connection, fileId, video],
  );
  // The film plays at the tab's volume, which the status tray's pop-up sets too (D281 §5).
  useEffect(() => (video === null ? undefined : followFilmVolume(video)), [video]);
  useToldCannotKeepUp(connection, props.cannotKeepUp);
  useEffect(() => {
    if (isOver) {
      onOver();
    }
  }, [isOver, onOver]);
  return (
    <div className="together-host">
      <SoundOn video={video} connection={connection} />
      <p>{t("together.hostPlaying")}</p>
      <TogetherNews snapshot={snapshot} />
      {hostVoice === undefined ? null : <VoiceControls {...hostVoice} />}
      <FilmVolumeSlider />
    </div>
  );
}

/**
 * The tap that turns the host's film's sound on, while the browser plays it muted: it refuses a
 * film's sound that no recent tap started, as after the host's page is loaded again, and the film
 * played on for the group with no sound and nothing said why (the owner's test, 2026-10-12).
 */
function SoundOn(props: {
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
      onClick={() => {
        soundOnAtTap(video, connection);
        setIsMuted(false);
      }}
    >
      {t("together.soundOn")}
    </button>
  );
}

// SPDX-License-Identifier: MPL-2.0

import { followGroup, startingTuning, type CannotKeepUp, type GroupCommand } from "@orpheum/sdk";
import { useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { tabStorage } from "./browser-storage.ts";
import { useToken } from "./services.tsx";
import { TogetherNews } from "./together-news.tsx";
import { forgetMembership, readMembership, type Membership } from "./together-storage.ts";
import { VoiceControls } from "./together-voice.tsx";
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
 * host's video waits while a viewer buffers; leaving the player stops it for everyone. With no
 * session it shows nothing and does nothing.
 */
export function HostPlaysTogether(props: HostPlayer): JSX.Element | null {
  const [membership, setMembership] = useState(() => readMembership(tabStorage(), "host"));
  if (membership === undefined) {
    return null;
  }
  return (
    <HostLeading
      {...props}
      membership={membership}
      onOver={() => {
        forgetMembership(tabStorage(), "host");
        setMembership(undefined);
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
        : followGroup(video, connection, { fileId, isLeading: true, monotonicMs }),
    [connection, fileId, video],
  );
  useToldCannotKeepUp(connection, props.cannotKeepUp);
  useEffect(() => {
    if (isOver) {
      onOver();
    }
  }, [isOver, onOver]);
  return (
    <div className="together-host">
      <p>{t("together.hostPlaying")}</p>
      <TogetherNews snapshot={snapshot} />
      <VoiceControls
        membership={props.membership}
        sessionToken={token}
        isMicrophoneProcessed={(snapshot?.tuning ?? startingTuning).shouldProcessMicrophone}
      />
    </div>
  );
}

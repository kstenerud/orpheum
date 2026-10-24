// SPDX-License-Identifier: MPL-2.0

import { useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { FrameTimingNote } from "./frame-timing-note.tsx";
import { SoundDelayControl } from "./sound-delay.tsx";

/**
 * The film's own file in a video element and nothing else: no subtitles, no drawing over it, the
 * browser's own controls, from where the playback resumes. A test of whether what the player draws
 * over a video makes a browser show its picture late (the owner's TCL TV, 2026-10-24); its frame
 * timing under it.
 */
export function BarePlayer(props: {
  readonly url: string;
  readonly resumePositionMs: number;
  /** How long the device holds its sound back after its picture (sound-delay.tsx). */
  readonly soundDelayMs: number;
  readonly onBack: () => void;
}): JSX.Element {
  const { t } = useTranslation();
  const [video, setVideo] = useState<HTMLVideoElement | null>(null);
  const videoRef = useRef<HTMLVideoElement | null>(null);
  return (
    <div className="play">
      {/* eslint-disable-next-line jsx-a11y-x/media-has-caption -- nothing over the picture, subtitles included: the test is of the video alone */}
      <video
        ref={(element) => {
          videoRef.current = element;
          setVideo(element);
        }}
        className="bare-player"
        src={props.url}
        controls
        autoPlay
        playsInline
        onLoadedMetadata={(event) => {
          // eslint-disable-next-line no-param-reassign -- where the playback resumes
          event.currentTarget.currentTime = props.resumePositionMs / 1000;
        }}
      />
      <FrameTimingNote video={video} />
      <SoundDelayControl videoRef={videoRef} initialMs={props.soundDelayMs} />
      <button type="button" onClick={props.onBack}>
        {t("play.bare.back")}
      </button>
    </div>
  );
}

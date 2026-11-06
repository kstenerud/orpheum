// SPDX-License-Identifier: MPL-2.0

import { playerMovedSoundEvent, soundMovedOf } from "@orpheum/sdk";
import { useEffect, useLayoutEffect, useRef, useState, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import { LabFrame } from "./lab-frame.tsx";
import { soundTimingBeeps, soundTimingClipMs, timingPictureAt } from "./lab-sound-clip.ts";
import { beepsWav, wavUrl } from "./lab-tone.ts";
import { useLevelGain, type SoundLevel } from "./levelling.tsx";
import { Loaded } from "./loaded.tsx";
import { dataOrThrow } from "./problem.ts";
import { DeviceSoundOffsetControl } from "./sound-delay.tsx";
import { OwnChoiceBox, useOwnChoice } from "./sound.tsx";
import { useServices } from "./services.tsx";
import { useResource } from "./use-resource.ts";

/**
 * The gain the test's beeps are levelled by while Level loudness is on, in decibels: a film's
 * levelling hands its sound to Web Audio for any gain but 0 dB, so the beeps take the levelled film's
 * path, and their sound, and its lag, are a film's. A justified constant: −6 dB is a typical film's
 * levelling (−11.6 dB on the staging ruler clip, 2026-11-06) and keeps the beeps comfortable.
 */
export const testLevellingGainDb = -6;

/** The beeps' level while Level loudness is on: measured, so levelled by testLevellingGainDb. */
const testLevel: SoundLevel = {
  levelling: { isMeasured: true, gainDb: testLevellingGainDb },
  carriedGainDb: 0,
};

/**
 * The sound timing test (the owner, 2026-11-06): a flash and a beep each second, the beep played
 * through a video element as a film's sound is, levelled as the person's films are, and moved by the
 * device's sound offset as theirs are; Sound earlier or later moves it until the flash and the beep
 * come together, and keeps the offset with the device for every film. A ruler clip in a library of
 * its own failed on the owner's TCL TV, its marker never moving (2026-11-06); this one is drawn by
 * the page and needs no clip.
 */
export function LabSound(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [preferences] = useResource("lab-sound-preferences", async () =>
    dataOrThrow(await api.GET("/me/preferences")),
  );
  return (
    <LabFrame id="lab-sound" heading={t("lab.sound.heading")}>
      <p className="lab__lead">{t("lab.sound.explain")}</p>
      <Loaded
        resource={preferences}
        render={(value) => <SoundTimingTest isLevellingInitiallyOn={value.isLevellingOn} />}
      />
    </LabFrame>
  );
}

/**
 * The test as the person's Level loudness stands, its switch the one under a film: switched, the
 * clip starts anew on an element of its own, as a film started then would, since Web Audio keeps an
 * element's sound once it has it.
 */
function SoundTimingTest(props: { readonly isLevellingInitiallyOn: boolean }): JSX.Element {
  const { t } = useTranslation();
  const levelling = useOwnChoice("isLevellingOn", {
    isInitiallyOn: props.isLevellingInitiallyOn,
    onSwitched: () => undefined,
    isOffered: false,
  });
  return (
    <>
      <div>
        <OwnChoiceBox label={t("play.level.switch")} state={levelling} />
        <p className="lab__small">
          {t(levelling.isOn ? "lab.sound.levelled" : "lab.sound.notLevelled")}
        </p>
      </div>
      <SoundTimingClip key={String(levelling.isOn)} isLevelled={levelling.isOn} />
    </>
  );
}

/**
 * The clip and the device's sound offset under it: the beeps play through the video element, which
 * shows no picture, and the picture is drawn from the element's clock each frame, held back by as
 * much as the player moves the element's sound earlier (playerMovedSoundEvent), which a film's
 * chunks would carry out and a file played whole cannot.
 */
function SoundTimingClip(props: { readonly isLevelled: boolean }): JSX.Element {
  const { t } = useTranslation();
  const videoRef = useRef<HTMLVideoElement | null>(null);
  const panelRef = useRef<HTMLDivElement | null>(null);
  const [isPlaying, setIsPlaying] = useState(false);
  useClipSound(videoRef);
  useLevelGain(testLevel, props.isLevelled, videoRef);
  useDrawnPicture(videoRef, panelRef);
  return (
    <>
      {/* eslint-disable-next-line jsx-a11y-x/media-has-caption -- beeps with no words; the flash drawn beside them is their picture */}
      <video
        ref={videoRef}
        className="lab-sound__video"
        loop={true}
        playsInline={true}
        preload="auto"
        aria-label={t("lab.sound.clipLabel")}
        onPlay={() => {
          setIsPlaying(true);
        }}
        onPause={() => {
          setIsPlaying(false);
        }}
      />
      <div ref={panelRef} className="lab-sound__panel" data-beat="1" aria-hidden="true">
        <span className="lab-sound__beat">1</span>
        <span className="lab-sound__bar" />
      </div>
      <p>
        <button
          type="button"
          className="lab__button"
          onClick={() => {
            const video = videoRef.current;
            if (video === null) {
              return;
            }
            if (video.paused) {
              video.play().catch(() => undefined);
            } else {
              video.pause();
            }
          }}
        >
          {t(isPlaying ? "lab.sound.stop" : "lab.sound.start")}
        </button>
      </p>
      <DeviceSoundOffsetControl videoRef={videoRef} />
    </>
  );
}

/** Gives the element the clip's sound, made in the page, for as long as it is drawn. */
function useClipSound(videoRef: RefObject<HTMLVideoElement | null>): void {
  useEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    const url = wavUrl(beepsWav(soundTimingBeeps, { lastingMs: soundTimingClipMs }));
    video.src = url;
    return () => {
      video.pause();
      video.removeAttribute("src");
      URL.revokeObjectURL(url);
    };
  }, [videoRef]);
}

/**
 * Draws the picture from the element's clock at each frame, held back by the player's last move of
 * the element's sound earlier. The move is heard from before any effect can make one: the levelling
 * makes one as it starts, by the latency the device remembers.
 */
function useDrawnPicture(
  videoRef: RefObject<HTMLVideoElement | null>,
  panelRef: RefObject<HTMLDivElement | null>,
): void {
  const pictureLateByMsRef = useRef(0);
  useLayoutEffect(() => {
    const video = videoRef.current;
    if (video === null) {
      return undefined;
    }
    const heard = (event: Event): void => {
      const moved = soundMovedOf(event);
      if (moved !== undefined) {
        pictureLateByMsRef.current = moved.earlierMs;
      }
    };
    video.addEventListener(playerMovedSoundEvent, heard);
    return () => {
      video.removeEventListener(playerMovedSoundEvent, heard);
    };
  }, [videoRef]);
  useEffect(() => {
    let frame = 0;
    const draw = (): void => {
      const video = videoRef.current;
      const panel = panelRef.current;
      if (video !== null && panel !== null) {
        const picture = timingPictureAt(video.currentTime * 1000, pictureLateByMsRef.current);
        panel.dataset["beat"] = String(picture.beat);
        panel.classList.toggle("lab-sound__panel--flash", picture.isFlash);
        panel.style.setProperty("--lab-sound-fill", String(picture.fill));
        const beat = panel.firstElementChild;
        if (beat !== null) {
          beat.textContent = String(picture.beat);
        }
      }
      frame = requestAnimationFrame(draw);
    };
    frame = requestAnimationFrame(draw);
    return () => {
      cancelAnimationFrame(frame);
    };
  }, [videoRef, panelRef]);
}

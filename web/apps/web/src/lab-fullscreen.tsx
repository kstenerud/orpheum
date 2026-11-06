// SPDX-License-Identifier: MPL-2.0

import "./lab-words.ts";

import { useCallback, useEffect, useRef, type JSX, type RefObject } from "react";
import { useTranslation } from "react-i18next";

import type { LabInputEvent, NotedEvent } from "./lab-events.ts";

/** What the full-screen test asks to go full screen: the video's box, or the bare video. */
export type FullscreenTarget = NonNullable<LabInputEvent["fullscreenTarget"]>;

/** When an event came, on the page's clock, in whole milliseconds. */
function nowMs(): number {
  return Math.round(performance.now());
}

/**
 * The test video's picture: small, since a TV scales it up, at a TV's 16 : 9. A justified constant,
 * the test's own picture.
 */
const pictureWidth = 320;
const pictureHeight = 180;

/** The frames a second the canvas's stream asks for: a video's usual rate. A justified constant. */
const framesPerSecond = 30;

/** The picture's colours: the skin's, written out, since a canvas reads no custom property. */
const colours = { ground: "#000000", text: "#f3eee4", sweep: "#ffa891" } as const;

/**
 * The element full screen, or null: also where the browser has no full screen at all (an iPhone's
 * Safari, a page in a test), whose document has no fullscreenElement.
 */
export function fullscreenElementOf(): Element | null {
  const element: unknown = Reflect.get(document, "fullscreenElement");
  return element instanceof Element ? element : null;
}

/** An error's name as the log takes it: letters, 40 at most. */
function errorNameOf(cause: unknown): string {
  const name = cause instanceof Error ? cause.name : "Unknown";
  return name.replaceAll(/[^A-Za-z]/gu, "").slice(0, 40);
}

/**
 * The test video, generated in the page so it needs no media from the server: a canvas drawing the
 * seconds since it started and a sweep across each second, streamed into the video by captureStream.
 * Where the browser has no captureStream, the canvas itself is shown in the video's box instead.
 */
class TestVideo {
  private readonly canvas = document.createElement("canvas");
  private frame = 0;
  private readonly startedMs = performance.now();
  private readonly stream: MediaStream | undefined;

  constructor(
    box: HTMLElement,
    private readonly video: HTMLVideoElement,
  ) {
    this.canvas.width = pictureWidth;
    this.canvas.height = pictureHeight;
    this.canvas.className = "lab__screen-video";
    if ("captureStream" in HTMLCanvasElement.prototype) {
      this.stream = this.canvas.captureStream(framesPerSecond);
      this.video.srcObject = this.stream;
      void this.video.play().catch(() => undefined);
    } else {
      box.prepend(this.canvas);
    }
    this.frame = requestAnimationFrame(this.draw);
  }

  stop(): void {
    cancelAnimationFrame(this.frame);
    for (const track of this.stream?.getTracks() ?? []) {
      track.stop();
    }
    this.video.srcObject = null;
    this.canvas.remove();
  }

  private readonly draw = (nowAtMs: number): void => {
    const drawing = this.canvas.getContext("2d");
    if (drawing !== null) {
      const seconds = (nowAtMs - this.startedMs) / 1000;
      drawing.fillStyle = colours.ground;
      drawing.fillRect(0, 0, pictureWidth, pictureHeight);
      drawing.fillStyle = colours.sweep;
      drawing.fillRect((seconds % 1) * pictureWidth, pictureHeight * 0.8, 6, pictureHeight * 0.1);
      drawing.fillStyle = colours.text;
      drawing.textAlign = "center";
      drawing.font = "64px system-ui, sans-serif";
      drawing.fillText(seconds.toFixed(1), pictureWidth / 2, pictureHeight / 2);
    }
    this.frame = requestAnimationFrame(this.draw);
  };
}

/**
 * Leaves full screen at a Back or an Escape the page hears while full screen, and sends that it
 * asked: what the browser itself does with them is in the events around it (H7).
 */
export function leaveFullscreenAt(
  noteEvent: (event: NotedEvent) => void,
  button: NonNullable<LabInputEvent["button"]>,
): void {
  if (fullscreenElementOf() === null || !("exitFullscreen" in document)) {
    return;
  }
  noteEvent({ kind: "fullscreenExitAsked", button, inFullscreen: true, atMs: nowMs() });
  void document.exitFullscreen().catch(() => undefined);
}

/**
 * Full screen's entering and leaving, sent as they happen, with what went full screen: the bare
 * video, else its box; onLeft told once the page leaves it, and as the test closes.
 */
function useFullscreenChanges(
  videoRef: RefObject<HTMLVideoElement | null>,
  noteEvent: (event: NotedEvent) => void,
  onLeft: () => void,
): void {
  useEffect(() => {
    let target: FullscreenTarget | undefined;
    const changed = (): void => {
      const element = fullscreenElementOf();
      if (element !== null) {
        target = element === videoRef.current ? "video" : "container";
        noteEvent({ kind: "fullscreenEnter", fullscreenTarget: target, atMs: nowMs() });
        return;
      }
      noteEvent({
        kind: "fullscreenExit",
        ...(target === undefined ? {} : { fullscreenTarget: target }),
        atMs: nowMs(),
      });
      onLeft();
    };
    document.addEventListener("fullscreenchange", changed);
    return () => {
      document.removeEventListener("fullscreenchange", changed);
      onLeft();
    };
  }, [videoRef, noteEvent, onLeft]);
}

/**
 * The test's video in its box, and its opening full screen by target: the video started, then full
 * screen asked at once, inside the press, which a browser requires; a refusal sent with its error's
 * name. Full screen's entering and leaving are sent as they happen, with what went full screen; the
 * video stops once the page leaves it.
 */
function useTestScreen(noteEvent: (event: NotedEvent) => void): {
  readonly boxRef: RefObject<HTMLDivElement | null>;
  readonly videoRef: RefObject<HTMLVideoElement | null>;
  readonly open: (target: FullscreenTarget) => void;
} {
  const boxRef = useRef<HTMLDivElement>(null);
  const videoRef = useRef<HTMLVideoElement>(null);
  const playingRef = useRef<TestVideo | undefined>(undefined);
  const stopPlaying = useCallback(() => {
    playingRef.current?.stop();
    playingRef.current = undefined;
  }, []);
  useFullscreenChanges(videoRef, noteEvent, stopPlaying);
  const open = useCallback(
    (target: FullscreenTarget) => {
      const box = boxRef.current;
      const video = videoRef.current;
      if (box === null || video === null) {
        return;
      }
      playingRef.current ??= new TestVideo(box, video);
      const refused = (cause: unknown): void => {
        noteEvent({
          kind: "fullscreenRefused",
          fullscreenTarget: target,
          errorName: errorNameOf(cause),
          atMs: nowMs(),
        });
        if (fullscreenElementOf() === null) {
          stopPlaying();
        }
      };
      const element = target === "video" ? video : box;
      try {
        element.requestFullscreen().catch(refused);
      } catch (cause) {
        refused(cause);
      }
    },
    [noteEvent, stopPlaying],
  );
  return { boxRef, videoRef, open };
}

/**
 * The full-screen test (plans/tv-device-tests.md H7): a small generated video, which one button
 * puts full screen by its box, so the page keeps hearing the keys, and the other by the bare video,
 * which a TV's browser may treat otherwise; entering, leaving and a refusal sent as they happen.
 */
export function FullscreenTest(props: {
  readonly noteEvent: (event: NotedEvent) => void;
}): JSX.Element {
  const { t } = useTranslation();
  const { boxRef, videoRef, open } = useTestScreen(props.noteEvent);
  return (
    <>
      <button
        type="button"
        className="lab__button"
        onClick={() => {
          open("container");
        }}
      >
        {t("lab.input.fullscreen.box")}
      </button>
      <button
        type="button"
        className="lab__button"
        onClick={() => {
          open("video");
        }}
      >
        {t("lab.input.fullscreen.video")}
      </button>
      <div ref={boxRef} className="lab__screen">
        <video
          ref={videoRef}
          className="lab__screen-video"
          muted={true}
          playsInline={true}
          aria-label={t("lab.input.fullscreen.label")}
        />
        <p className="lab__screen-hint">{t("lab.input.fullscreen.hint")}</p>
      </div>
    </>
  );
}

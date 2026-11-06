// SPDX-License-Identifier: MPL-2.0

import "./lab.css";

import "./lab-words.ts";

import { useEffect, useRef, type JSX } from "react";
import { useTranslation } from "react-i18next";

import {
  extendChannelName,
  extendMessageOf,
  positionAfter,
  type ExtendMessage,
} from "./lab-extend-channel.ts";

/** How long each second's flash and beep last, in seconds: long enough to see, short to time. */
const flashS = 0.1;

/** The beep's pitch, Hz: a tone a TV's speakers carry. */
const beepHz = 1000;

/** The pattern's colours: the skin's, written out, since a canvas reads no custom property. */
const colours = {
  ground: "#000000",
  flash: "#ffffff",
  text: "#f3eee4",
  sweep: "#ffa891",
} as const;

/**
 * The test pattern a canvas plays: its position large, a sweep across each second, and a flash and a
 * beep at each whole second, so picture and sound can be timed against each other. Driven by the
 * remote's commands; tells the remote its state.
 */
class TestPattern {
  private positionS = 0;
  private isPlaying = false;
  private lastFrameMs = performance.now();
  private audio: AudioContext | undefined;
  private frame = 0;
  private readonly channel =
    "BroadcastChannel" in globalThis ? new BroadcastChannel(extendChannelName) : undefined;
  private readonly telling: ReturnType<typeof setInterval>;

  constructor(
    private readonly canvas: HTMLCanvasElement,
    private readonly drawing: CanvasRenderingContext2D,
    private readonly hint: string,
  ) {
    this.frame = requestAnimationFrame(this.draw);
    this.channel?.addEventListener("message", this.heard);
    canvas.addEventListener("click", this.pressed);
    this.telling = setInterval(this.tell, 1000);
  }

  stop(): void {
    cancelAnimationFrame(this.frame);
    clearInterval(this.telling);
    this.canvas.removeEventListener("click", this.pressed);
    this.channel?.removeEventListener("message", this.heard);
    this.channel?.close();
    void this.audio?.close();
  }

  private readonly tell = (): void => {
    const state: ExtendMessage = {
      kind: "state",
      positionS: Math.round(this.positionS * 10) / 10,
      isPlaying: this.isPlaying,
      isFullscreen: document.fullscreenElement !== null,
    };
    this.channel?.postMessage(state);
  };

  private readonly heard = (event: MessageEvent): void => {
    const message = extendMessageOf(event.data);
    if (message?.kind !== "command") {
      return;
    }
    if (message.command === "toggle") {
      this.isPlaying = !this.isPlaying;
    } else {
      this.positionS = positionAfter(message.command, this.positionS);
    }
    this.tell();
  };

  /** The first press: sound, which needs one, and full screen, which needs one too. */
  private readonly pressed = (): void => {
    if ("AudioContext" in globalThis) {
      this.audio ??= new AudioContext();
    }
    void this.audio?.resume();
    if (document.fullscreenElement === null) {
      void document.documentElement
        .requestFullscreen()
        .catch(() => undefined)
        .then(this.tell);
    }
    this.isPlaying = true;
    this.tell();
  };

  private beep(): void {
    if (this.audio === undefined) {
      return;
    }
    const oscillator = this.audio.createOscillator();
    oscillator.frequency.value = beepHz;
    oscillator.connect(this.audio.destination);
    oscillator.start();
    oscillator.stop(this.audio.currentTime + flashS);
  }

  private readonly draw = (nowMs: number): void => {
    const before = this.positionS;
    if (this.isPlaying) {
      this.positionS += (nowMs - this.lastFrameMs) / 1000;
    }
    this.lastFrameMs = nowMs;
    if (this.isPlaying && Math.floor(this.positionS) !== Math.floor(before)) {
      this.beep();
    }
    this.paint();
    this.frame = requestAnimationFrame(this.draw);
  };

  private paint(): void {
    const { canvas, drawing } = this;
    canvas.width = canvas.clientWidth;
    canvas.height = canvas.clientHeight;
    const { width, height } = canvas;
    const isFlash = this.isPlaying && this.positionS % 1 < flashS;
    drawing.fillStyle = isFlash ? colours.flash : colours.ground;
    drawing.fillRect(0, 0, width, height);
    drawing.fillStyle = colours.sweep;
    drawing.fillRect((this.positionS % 1) * width, height * 0.8, width * 0.01, height * 0.1);
    drawing.fillStyle = isFlash ? colours.ground : colours.text;
    drawing.textAlign = "center";
    drawing.font = `${String(Math.round(height / 5))}px system-ui, sans-serif`;
    drawing.fillText(this.positionS.toFixed(1), width / 2, height / 2);
    drawing.font = `${String(Math.round(height / 24))}px system-ui, sans-serif`;
    drawing.fillText(this.hint, width / 2, height * 0.7);
  }
}

/**
 * The extend probe's player (plans/tv-device-tests.md), in a window of its own on the TV's screen:
 * the test pattern, full screen from its first press, driven by the remote in the window that opened
 * it. It needs no server, and so no sign-in.
 */
export function LabExtendPlayer(): JSX.Element {
  const { t } = useTranslation();
  const canvasRef = useRef<HTMLCanvasElement>(null);
  const hint = t("lab.extend.playerHint");
  useEffect(() => {
    const canvas = canvasRef.current;
    const drawing = canvas?.getContext("2d") ?? null;
    if (canvas === null || drawing === null) {
      return undefined;
    }
    const pattern = new TestPattern(canvas, drawing, hint);
    return () => {
      pattern.stop();
    };
  }, [hint]);
  return (
    <main className="lab-player">
      <canvas
        ref={canvasRef}
        className="lab-player__canvas"
        aria-label={t("lab.extend.playerLabel")}
      />
    </main>
  );
}

// SPDX-License-Identifier: MPL-2.0

import "./pill.css";

import type { JSX, ReactNode } from "react";

/**
 * How a notice pill speaks: "said", read out once as it shows (an `<output>`), a centre notice's
 * or a bottom-left note's; "volume", the same with digits that keep their width as the level moves;
 * "silent", never read out, a double-tap seek's "+10", which the person made and sees.
 */
export type PillVoice = "said" | "volume" | "silent";

/** Each voice's class, written out whole, so a search for a stylesheet's class finds it. */
const voiceClass: Readonly<Record<PillVoice, string>> = {
  said: "pill--said",
  volume: "pill--volume",
  silent: "pill--silent",
};

/**
 * A rounded note over a film that tells the viewer something (standards/ui.md, over the film, rule
 * 15; D538 §1): "Seeked to 21:41", "Volume 70%", "Sam joined". Only over video, in the player and a
 * cast's remote. It takes no press and no focus; its place (the centre, the tapped third, the bottom
 * left) and how long it stays are its user's, which lays it on the film's layer for notes.
 */
export function Pill(props: {
  readonly children: ReactNode;
  /** An `Icon` drawing before the words. */
  readonly icon?: ReactNode;
  /** How it speaks; "said" where not given. */
  readonly voice?: PillVoice | undefined;
}): JSX.Element {
  const voice = props.voice ?? "said";
  const content = (
    <>
      {props.icon === undefined ? null : <span className="pill__icon">{props.icon}</span>}
      <span className="pill__words">{props.children}</span>
    </>
  );
  // An <output> is a status, read out as it shows; a silent pill is a plain line.
  return voice === "silent" ? (
    <p className={`pill ${voiceClass[voice]}`}>{content}</p>
  ) : (
    <output className={`pill ${voiceClass[voice]}`}>{content}</output>
  );
}

/**
 * A pill over a film that offers one action, its words the action's ("Stop casting here", "Tap to
 * turn the sound on"): a button a fingertip tall, a focus stop with the ring every control has.
 */
export function ActionPill(props: {
  readonly children: ReactNode;
  readonly icon?: ReactNode;
  readonly onPress: () => void;
}): JSX.Element {
  return (
    <button type="button" className="pill pill--action" onClick={props.onPress}>
      {props.icon === undefined ? null : <span className="pill__icon">{props.icon}</span>}
      <span className="pill__words">{props.children}</span>
    </button>
  );
}

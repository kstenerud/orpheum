// SPDX-License-Identifier: MPL-2.0

import "./volume-row.css";

import { useEffect, useRef, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { VolumeButtonsNote } from "./film-volume-slider.tsx";
import { Icon } from "./icon.tsx";
import { volumeAfter } from "./volume-step.ts";
import { SoundControls, type PlayerSound } from "./player-controls.tsx";
import type { OneControl } from "./popover.tsx";
import {
  partnerGainAfter,
  partnerGainOf,
  peopleOf,
  PersonVoiceRange,
  type VoiceInUse,
  type VoicePerson,
} from "./together-voice.tsx";

/**
 * How many colours a person's slider may carry (volume-row.css, tokens.css's --color-person-1 to
 * --color-person-6), a small fixed palette.
 */
export const personColourCount = 6;

/**
 * The colour, 1 to personColourCount, a person's slider carries: by their member, whose id the
 * session gives in the order people join it, so the same on every device and for the whole session,
 * whoever comes or goes, and different for each of six people who joined one after another.
 */
export function personColourOf(memberId: number): number {
  return ((((memberId - 1) % personColourCount) + personColourCount) % personColourCount) + 1;
}

/** A name's first letters, as many as count, the first a capital: by what a reader sees as letters. */
function firstLetters(name: string, count: number): string {
  const letters = Array.from(name.trim()).slice(0, count);
  const [first = "?", ...rest] = letters;
  return first.toLocaleUpperCase() + rest.join("").toLocaleLowerCase();
}

/**
 * What each person's slider shows of their name, in the order of names: its first letter, or its
 * first two where another in the row starts with the same letter (D440 §2).
 */
export function lettersOf(names: readonly string[]): readonly string[] {
  const firsts = names.map((name) => firstLetters(name, 1));
  return names.map((name, at) =>
    firsts.filter((first) => first === firsts[at]).length > 1
      ? firstLetters(name, 2)
      : (firsts[at] ?? "?"),
  );
}

/** The film's mark in the row: a strip of film. */
function FilmIcon(): JSX.Element {
  return (
    <Icon className="volume-row__film-icon">
      <path
        fillRule="evenodd"
        d="M4 3h16v18H4zm2 2v2h2V5zm10 0v2h2V5zM6 9v2h2V9zm10 0v2h2V9zM6 13v2h2v-2zm10 0v2h2v-2zM6 17v2h2v-2zm10 0v2h2v-2zM10 5v14h4V5z"
      />
    </Icon>
  );
}

/** 🔈's row as the overlay's menu holds it: which slider has focus, and what Up, Down, Left and Right do. */
export interface VolumeRowState {
  /** Where in the row focus is: 0 the film's slider, then each person's in the roster's order. */
  readonly focusedAt: number;
  readonly setFocusedAt: (at: number) => void;
  readonly people: readonly VoicePerson[];
  /** Up and Down step the focused slider; Left and Right move along the row (Popover's OneControl). */
  readonly oneControl: OneControl;
}

/**
 * 🔈's row of sliders (D440 §2): the film's, then one per other person in the Watch Together
 * session, voice given, focus on the film's as it opens. Up and Down step the focused slider, the
 * film's by the volume's step as + and − do, a person's by the same step over their voice's span;
 * Left and Right move focus along the row, and, past its end, close it, as they close a pop-up of
 * one control.
 */
export function useVolumeRow(sound: PlayerSound, voice: VoiceInUse | undefined): VolumeRowState {
  const [focusedAt, setFocusedAt] = useState(0);
  const people = voice === undefined ? [] : peopleOf(voice.snapshot);
  // People gone from the row: focus is on a slider still in it.
  const at = Math.min(focusedAt, people.length);
  const focusedPerson = people[at - 1];
  const step = (direction: -1 | 1): void => {
    if (focusedPerson === undefined || voice === undefined) {
      stepFilm(sound, direction);
      return;
    }
    const gain = partnerGainOf(voice.snapshot, focusedPerson.memberId);
    voice.voice.setPartnerGain(focusedPerson.memberId, partnerGainAfter(gain, direction));
  };
  return {
    focusedAt: at,
    setFocusedAt,
    people,
    oneControl: {
      up: () => {
        step(1);
      },
      down: () => {
        step(-1);
      },
      across: (direction) => {
        const next = at + direction;
        if (next < 0 || next > people.length) {
          return false;
        }
        setFocusedAt(next);
        return true;
      },
    },
  };
}

/** A step of the film's volume (volumeAfter), Up turning a muted film's sound on, as + and − do. */
function stepFilm(sound: PlayerSound, direction: -1 | 1): void {
  sound.setVolume(volumeAfter(sound.volume, direction));
  if (direction === 1 && sound.isMuted) {
    sound.setMuted(false);
  }
}

/**
 * The row of sliders 🔈 opens (D440 §2): the film's, marked by a film icon, mute under it, upright
 * as before (SoundControls), or, where the page cannot set the film's volume, the word that the
 * phone's buttons do; then each other person's in the session, upright, carrying their colour and
 * their first letter (two where first letters collide), the focused one their full name above it.
 * A finger or a mouse drags each slider. The browser's focus stays on the film's mute, where it
 * opened: the row marks the slider Up and Down move (data-focused), as the one control it is.
 */
export function VolumeRow(props: {
  readonly sound: PlayerSound;
  readonly voice: VoiceInUse | undefined;
  readonly row: VolumeRowState;
  /** What the film's mark says of its slider, where not the film's volume here (FilmSlider). */
  readonly filmTitle?: string | undefined;
}): JSX.Element {
  const { sound, voice, row } = props;
  const letters = lettersOf(row.people.map((person) => person.name));
  return (
    <div className="volume-row">
      <FilmSlider
        sound={sound}
        title={props.filmTitle}
        isFocused={row.focusedAt === 0}
        isFocusable={false}
      />
      {voice === undefined
        ? null
        : row.people.map((person, index) => (
            <PersonSlider
              key={person.memberId}
              voice={voice}
              person={person}
              letters={letters[index] ?? "?"}
              isFocused={row.focusedAt === index + 1}
              isFocusable={false}
            />
          ))}
    </div>
  );
}

/**
 * The slider row on a Watch Together guest's page while it casts the group's film (D440 §5): each
 * other person's in the session, as 🔈's row draws them (VolumeRow). The cast's own remote has the
 * overlay's 🔈 instead, the film's level on the receiver first (D444). Not a pop-up but part of the
 * page, so each slider takes the browser's focus as any control does, a finger or a mouse dragging
 * it; the one focused shows the person's name above it. Nothing where the session has no voices.
 */
export function RemoteVolumeRow(props: {
  readonly voice: VoiceInUse | undefined;
}): JSX.Element | null {
  const { voice } = props;
  const [focusedId, setFocusedId] = useState<number | undefined>(undefined);
  const people = voice === undefined ? [] : peopleOf(voice.snapshot);
  if (people.length === 0) {
    return null;
  }
  const letters = lettersOf(people.map((person) => person.name));
  return (
    <div className="volume-row volume-row--on-the-page">
      {voice === undefined
        ? null
        : people.map((person, index) => (
            <PersonSlider
              key={person.memberId}
              voice={voice}
              person={person}
              letters={letters[index] ?? "?"}
              isFocused={focusedId === person.memberId}
              isFocusable
              onFocusChange={(isFocused) => {
                setFocusedId((now) => {
                  if (isFocused) {
                    return person.memberId;
                  }
                  return now === person.memberId ? undefined : now;
                });
              }}
            />
          ))}
    </div>
  );
}

/**
 * The film's slider in a row, marked by a film icon, mute under it, upright (SoundControls), or,
 * where the page cannot set the film's volume, the word that the phone's buttons do. Focusable where
 * the row is part of a page; over the film, the row marks it focused instead. Its mark says what it
 * sets (title): the film's volume, or, on the cast's remote, the film's level on the receiver under
 * the TV's own (D440 §5, D444).
 */
function FilmSlider(props: {
  readonly sound: PlayerSound;
  readonly title?: string | undefined;
  readonly isFocused: boolean;
  readonly isFocusable: boolean;
}): JSX.Element {
  const { t } = useTranslation();
  const { sound } = props;
  return (
    <div
      className="volume-row__slider volume-row__film"
      data-volume-slider=""
      data-focused={props.isFocused ? "" : undefined}
    >
      <span className="volume-row__mark" title={props.title ?? t("playerControls.filmVolume")}>
        <FilmIcon />
      </span>
      <SoundControls
        sound={sound}
        isOverVideo={false}
        isUpright
        isRangeFocusable={props.isFocusable}
      />
      {sound.canSetVolume ? null : <VolumeButtonsNote />}
    </div>
  );
}

/**
 * A person's slider in a row: their name above it while focused, their colour and letters under it.
 * Where isFocusable, its slider takes the browser's focus, told onFocusChange.
 */
function PersonSlider(props: {
  readonly voice: VoiceInUse;
  readonly person: VoicePerson;
  readonly letters: string;
  readonly isFocused: boolean;
  readonly isFocusable: boolean;
  readonly onFocusChange?: ((isFocused: boolean) => void) | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  const { voice, person, onFocusChange } = props;
  const label = t("together.voice.personVolume", { name: person.name });
  const sliderRef = useRef<HTMLDivElement>(null);
  // The slider's focus, heard as it bubbles from the range inside.
  useEffect(() => {
    const slider = sliderRef.current;
    if (slider === null || onFocusChange === undefined) {
      return undefined;
    }
    const focused = (): void => {
      onFocusChange(true);
    };
    const blurred = (): void => {
      onFocusChange(false);
    };
    slider.addEventListener("focusin", focused);
    slider.addEventListener("focusout", blurred);
    return () => {
      slider.removeEventListener("focusin", focused);
      slider.removeEventListener("focusout", blurred);
    };
  }, [onFocusChange]);
  return (
    <div
      ref={sliderRef}
      className="volume-row__slider volume-row__person"
      data-volume-slider=""
      data-colour={personColourOf(person.memberId)}
      data-focused={props.isFocused ? "" : undefined}
    >
      <span className="volume-row__name" aria-hidden="true">
        {props.isFocused ? person.name : ""}
      </span>
      <PersonVoiceRange
        voice={voice.voice}
        snapshot={voice.snapshot}
        tap={voice.tap}
        memberId={person.memberId}
        label={label}
        isUpright
        isFocusable={props.isFocusable}
      />
      <span className="volume-row__mark volume-row__letters" aria-hidden="true">
        {props.letters}
      </span>
    </div>
  );
}

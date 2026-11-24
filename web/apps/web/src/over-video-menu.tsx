// SPDX-License-Identifier: MPL-2.0

import "./play-words.ts";

import type { JSX, ReactNode } from "react";
import { useTranslation } from "react-i18next";

import { DropDown } from "./drop-down.tsx";
import { ListEntry, ListOption, ListSubmenu } from "./list-box.tsx";
import { AudioIcon, SettingsIcon, SubtitlesIcon } from "./player-icons.tsx";
import { currentLabelOf, type ActionChoice } from "./route-chooser.tsx";

/**
 * The menus over a film (D407 §8, D538 §8) and their parts, the player's (player-menus.tsx,
 * sound-menu.tsx) and the cast's remote's over the receiver (cast-remote-choices.tsx, D444) alike
 * (the owner, 2026-11-23: "I want the interfaces to look as similar as possible"): each an icon
 * button over the film whose list grows out of it, opening from its top.
 */

/** A menu over the film: its icon button, named label, and its list (children). */
function OverVideoMenu(props: {
  readonly label: string;
  readonly icon: JSX.Element;
  readonly onOpenChange: (isOpen: boolean) => void;
  readonly children: ReactNode;
}): JSX.Element {
  return (
    <DropDown
      label={props.label}
      opener={props.icon}
      openerClassName="icon-button icon-button--over-video"
      openerNaming={{ "aria-label": props.label }}
      isFromItsTop
      isGrownFromOpener
      onOpenChange={props.onOpenChange}
    >
      {props.children}
    </DropDown>
  );
}

/** 💬 Subtitles over its entries: the player's tracks, timing and Find, or the receiver's tracks. */
export function SubtitlesMenuOf(props: {
  readonly onOpenChange: (isOpen: boolean) => void;
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <OverVideoMenu
      label={t("play.subtitles.label")}
      icon={<SubtitlesIcon />}
      onOpenChange={props.onOpenChange}
    >
      {props.children}
    </OverVideoMenu>
  );
}

/** 🎧 Sound over its entries: the player's sound (SoundMenu), or the receiver's. */
export function SoundMenuOf(props: {
  readonly onOpenChange: (isOpen: boolean) => void;
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <OverVideoMenu
      label={t("play.audio.label")}
      icon={<AudioIcon />}
      onOpenChange={props.onOpenChange}
    >
      {props.children}
    </OverVideoMenu>
  );
}

/** ⁝ Settings over its places: the player's (MoreMenu), or the receiver's. */
export function MoreMenuOf(props: {
  readonly onOpenChange: (isOpen: boolean) => void;
  readonly children: ReactNode;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <OverVideoMenu
      label={t("playerControls.settings")}
      icon={<SettingsIcon />}
      onOpenChange={props.onOpenChange}
    >
      {props.children}
    </OverVideoMenu>
  );
}

/**
 * A menu's submenu of choices, named by the one current (a sound track, a subtitle track); nothing
 * where there is no other to choose (the owner, 2026-11-23: "Any button that would have nothing in
 * its submenu should not display at all").
 */
export function ChoicesSubmenu(props: {
  readonly name: string;
  readonly choices: readonly ActionChoice[];
}): JSX.Element | null {
  const { choices } = props;
  if (choices.length < 2) {
    return null;
  }
  return (
    <ListSubmenu name={props.name} value={currentLabelOf(choices)}>
      {choices.map((choice) => (
        <ListOption key={choice.key} isCurrent={choice.isCurrent} onChoose={choice.onChoose}>
          {choice.label}
        </ListOption>
      ))}
    </ListSubmenu>
  );
}

/**
 * A place of a menu whose buttons step a value, a stepper's, which keep the list open for the next
 * press: − and + beside what they move, not each a whole place as a menu's choice is (D407 §8), set
 * in as the choices' text is.
 */
export function KeptOpen(props: { readonly children: ReactNode }): JSX.Element {
  return (
    <ListEntry isKeptOpen isInsetAsChoices>
      {props.children}
    </ListEntry>
  );
}

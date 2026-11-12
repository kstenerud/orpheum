// SPDX-License-Identifier: MPL-2.0

import "./app-shell.css";
import "./cast.css";
import "./listing-tools.css";
import "./more-actions.css";
import "./poster.css";
import "./route-chooser.css";
import "./settings.css";
import "./status-tray.css";
import "./still-reading.css";
import "./together-voice.css";
import "./uncertain-match.css";
import "./volume-row.css";
import "./warning-sign.css";
import "./watched-mark.css";

import type { JSX } from "react";

import { Icon } from "./icon.tsx";

/*
 * Every icon of the client outside the player, in one file (the owner, 2026-11-11: "One icon
 * library"), each drawn by Icon as it was drawn beside the screen that first used it: a second
 * screen takes the drawing from here and does not draw a near copy of its own with other line
 * widths. The player's icons are player-icons.tsx; the QR code draws its own SVG (icon.tsx). Each
 * icon's class is its screen's stylesheet's, which sizes and colours it, so this file loads the
 * stylesheets of those classes; the UI gallery shows the whole set side by side
 * (ui-gallery.tsx, ui-gallery.test.tsx).
 */

// ---- The banner and the menu ----

/*
 * The places for watching, drawn as icons in the banner so they fit one row with the tray and the
 * menu on a phone (the owner's drawings, 2026-10-28: "let's try using just icons for the top bar
 * links so that they can all go on one line").
 */

/** Films: a strip of film, its sprocket holes down both sides and two frames between. */
export function FilmsIcon(props: { readonly className: string }): JSX.Element {
  return (
    <Icon className={props.className}>
      <path
        fillRule="evenodd"
        d="M5 2h14a2 2 0 0 1 2 2v16a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2z M4.6 4h1.8v2H4.6z M4.6 8.6h1.8v2H4.6z M4.6 13.4h1.8v2H4.6z M4.6 18h1.8v2H4.6z M17.6 4h1.8v2h-1.8z M17.6 8.6h1.8v2h-1.8z M17.6 13.4h1.8v2h-1.8z M17.6 18h1.8v2h-1.8z M8.6 4.6h6.8a.6.6 0 0 1 .6.6v6H8v-6a.6.6 0 0 1 .6-.6z M8 12.8h8v6a.6.6 0 0 1-.6.6H8.6a.6.6 0 0 1-.6-.6z"
      />
    </Icon>
  );
}

/** Series: a television set, its aerial above. */
export function SeriesIcon(props: { readonly className: string }): JSX.Element {
  return (
    <Icon className={props.className}>
      <path
        fillRule="evenodd"
        d="M4.5 7h15a2.5 2.5 0 0 1 2.5 2.5v9a2.5 2.5 0 0 1-2.5 2.5h-15A2.5 2.5 0 0 1 2 18.5v-9A2.5 2.5 0 0 1 4.5 7z M5.5 9.5h9a1 1 0 0 1 1 1v7a1 1 0 0 1-1 1h-9a1 1 0 0 1-1-1v-7a1 1 0 0 1 1-1z M17.6 11.6a1.2 1.2 0 1 0 2.4 0a1.2 1.2 0 1 0-2.4 0z M17.6 15.4a1.2 1.2 0 1 0 2.4 0a1.2 1.2 0 1 0-2.4 0z"
      />
      <path d="M5.5 20.5h2.2v2H5.5z M16.3 20.5h2.2v2h-2.2z" />
      <path
        d="M8 2.2l4 4.2 4-4.2"
        fill="none"
        stroke="currentColor"
        strokeWidth="1.8"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </Icon>
  );
}

/** Libraries: books on a shelf, one leaning. */
export function LibrariesIcon(props: { readonly className: string }): JSX.Element {
  return (
    <Icon className={props.className}>
      <path d="M2.6 5h2.4a.6.6 0 0 1 .6.6V20H2V5.6a.6.6 0 0 1 .6-.6z" />
      <path d="M7.2 3h1.8a.6.6 0 0 1 .6.6V20h-3V3.6a.6.6 0 0 1 .6-.6z" />
      <path d="M11.2 9h2.2a.6.6 0 0 1 .6.6V20h-3.4V9.6a.6.6 0 0 1 .6-.6z" />
      <path
        d="M17.8 20 21.08 19.12 17.59 6.08 14.31 6.96z"
        stroke="currentColor"
        strokeWidth=".6"
        strokeLinejoin="round"
      />
      <path d="M1.5 20.8h21a.9.9 0 0 1 0 1.8h-21a.9.9 0 0 1 0-1.8z" />
    </Icon>
  );
}

/** Three lines: the menu's mark, its name for a screen reader. */
export function MenuIcon(): JSX.Element {
  return (
    <Icon className="app-shell__menu-icon">
      <path d="M3 6h18v2H3zm0 5h18v2H3zm0 5h18v2H3z" />
    </Icon>
  );
}

/** Three dots one above the other: the mark of a page's menu of rarely used actions. */
export function MoreIcon(): JSX.Element {
  return (
    <Icon className="more-actions__icon">
      <circle cx="12" cy="5" r="2" />
      <circle cx="12" cy="12" r="2" />
      <circle cx="12" cy="19" r="2" />
    </Icon>
  );
}

/** A chooser's mark: a chevron pointing down, to the list it opens. */
export function ChevronDownIcon(): JSX.Element {
  return (
    <Icon className="route-chooser__chevron">
      <path d="M5.6 8.3 12 14.7l6.4-6.4 1.4 1.4-7.8 7.8-7.8-7.8z" />
    </Icon>
  );
}

// ---- A listing's tools ----

// The listing tools' icons, as their designer drew them (the owner's upload, 2026-10-30).

/** A gear: configure. */
export function GearIcon(): JSX.Element {
  return (
    <Icon className="listing-tools__icon">
      <path
        fillRule="evenodd"
        stroke="currentColor"
        strokeWidth=".8"
        strokeLinejoin="round"
        d="M9.28 4.90L9.39 1.52A10.8 10.8 0 0 1 14.61 1.52L14.72 4.90A7.6 7.6 0 0 1 16.78 6.09L19.77 4.50A10.8 10.8 0 0 1 22.38 9.02L19.51 10.81A7.6 7.6 0 0 1 19.51 13.19L22.38 14.98A10.8 10.8 0 0 1 19.77 19.50L16.78 17.91A7.6 7.6 0 0 1 14.72 19.10L14.61 22.48A10.8 10.8 0 0 1 9.39 22.48L9.28 19.10A7.6 7.6 0 0 1 7.22 17.91L4.23 19.50A10.8 10.8 0 0 1 1.62 14.98L4.49 13.19A7.6 7.6 0 0 1 4.49 10.81L1.62 9.02A10.8 10.8 0 0 1 4.23 4.50L7.22 6.09A7.6 7.6 0 0 1 9.28 4.90z M12 8.3a3.7 3.7 0 1 0 0 7.4a3.7 3.7 0 1 0 0-7.4z"
      />
    </Icon>
  );
}

/** Two arrows, one up and one down: sort, before what the list's pop-up is set to. */
export function SortIcon(): JSX.Element {
  return (
    <Icon className="listing-tools__icon">
      <path
        d="M7.5 20V4.5 M3.5 8.5 7.5 4.5 11.5 8.5 M16.5 4v15.5 M12.5 15.5 16.5 19.5 20.5 15.5"
        fill="none"
        stroke="currentColor"
        strokeWidth="2.4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </Icon>
  );
}

/** An arrow up, a list running ascending (A to Z, oldest first), or down, descending. */
export function SortArrowIcon(props: { readonly isAscending: boolean }): JSX.Element {
  return (
    <Icon className="listing-tools__arrow">
      <path
        d={
          props.isAscending
            ? "M12 20V4.5 M7 9.5 12 4.5 17 9.5"
            : "M12 4v15.5 M7 14.5 12 19.5 17 14.5"
        }
        fill="none"
        stroke="currentColor"
        strokeWidth="2.4"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </Icon>
  );
}

/** An arrow's chevron, pointing the way it scrolls. */
export function RowArrowIcon(props: { readonly direction: "back" | "on" }): JSX.Element {
  return (
    <Icon className="poster-row__arrow-icon">
      <path
        d={props.direction === "back" ? "M15 4.5 7.5 12l7.5 7.5" : "M9 4.5 16.5 12 9 19.5"}
        fill="none"
        stroke="currentColor"
        strokeWidth="2.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      />
    </Icon>
  );
}

// ---- The status tray and its cues ----

/**
 * A screen playing, two people before it: Watch Together, in the colour of the text it sits in. The
 * owner's choice, drawn by Claude Design (2026-10-12).
 */
export function TogetherIcon(): JSX.Element {
  return (
    <Icon className="status-tray__icon">
      <path
        fillRule="evenodd"
        d="M4 2h16a2 2 0 0 1 2 2v7a2 2 0 0 1-2 2H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2zm6 2.8v5.4l4.6-2.7z"
      />
      <circle cx="7.5" cy="17.3" r="2.3" />
      <circle cx="16.5" cy="17.3" r="2.3" />
      <path d="M3.2 23a4.3 3.6 0 0 1 8.6 0z" />
      <path d="M12.2 23a4.3 3.6 0 0 1 8.6 0z" />
    </Icon>
  );
}

/**
 * A screen playing, a phone beside it: casting to another device (D364), the tray's Cast. The owner's
 * choice (2026-10-27), in the colour of the text it sits in.
 */
export function CastIcon(): JSX.Element {
  return (
    <Icon className="status-tray__icon">
      <path
        fillRule="evenodd"
        d="M7 2h14a2 2 0 0 1 2 2v9a2 2 0 0 1-2 2H10.5a1 1 0 0 1-1-1v-1.4a3.1 3.1 0 0 0-3.1-3.1H6a1 1 0 0 1-1-1V4a2 2 0 0 1 2-2zM12.8 5.8v5.4l4.6-2.7z"
      />
      <path d="M13 15h2v4.5h-2z" />
      <path d="M10.5 19.5h7a1 1 0 0 1 0 2h-7a1 1 0 0 1 0-2z" />
      <path
        fillRule="evenodd"
        d="M2.6 11h3.8A1.6 1.6 0 0 1 8 12.6v8.8A1.6 1.6 0 0 1 6.4 23H2.6A1.6 1.6 0 0 1 1 21.4v-8.8A1.6 1.6 0 0 1 2.6 11zM3.3 20.4h2.4v1.1H3.3z"
      />
    </Icon>
  );
}

/** The warning triangle itself, in the warning yellow (D278 §4): decoration beside words that say it. */
export function WarningIcon(): JSX.Element {
  return (
    <Icon className="warning-icon">
      <path className="warning-icon__triangle" d="M12 2.2 23 21.3H1z" />
      <rect className="warning-icon__mark" x="10.9" y="8.2" width="2.2" height="7.2" rx="1.1" />
      <circle className="warning-icon__mark" cx="12" cy="18.1" r="1.3" />
    </Icon>
  );
}

/**
 * The uncertain match's icon, the owner's (2026-10-15): a question mark on the caution yellow.
 * Ringed, it is drawn over a poster, the dark ring keeping it apart from any art; unringed, beside
 * words on a page. Decoration beside words that say it.
 */
export function UncertainMatchIcon(props: { readonly isRinged: boolean }): JSX.Element {
  const size = props.isRinged ? 30 : 26;
  const centre = size / 2;
  const inset = props.isRinged ? 4.5 : 2.5;
  return (
    <Icon className="uncertain-match-icon" viewBox={`0 0 ${String(size)} ${String(size)}`}>
      {props.isRinged ? (
        <circle className="uncertain-match-icon__ring" cx={centre} cy={centre} r={15} />
      ) : null}
      <circle className="uncertain-match-icon__disc" cx={centre} cy={centre} r={13} />
      <g transform={`translate(${String(inset)} ${String(inset)}) scale(0.875)`}>
        <path
          className="uncertain-match-icon__curve"
          d="M8.6 8.6a3.4 3.4 0 1 1 4.9 3.05c-.95.48-1.5 1.15-1.5 2.1v.5"
        />
        <circle className="uncertain-match-icon__dot" cx="12" cy="18.8" r="1.7" />
      </g>
    </Icon>
  );
}

/**
 * The still-reading icon: an hourglass on the status blue, its sand run into the lower bulb and
 * drawn heavy so it reads as one at a poster's corner (the owner, 2026-10-17: the thin one did not), for a title shown from its listing
 * whose file its machine has not read yet (D304 §4). Ringed, it is drawn over a poster; unringed,
 * beside words on a page. Decoration beside words that say it.
 */
export function StillReadingIcon(props: { readonly isRinged: boolean }): JSX.Element {
  const size = props.isRinged ? 30 : 26;
  const centre = size / 2;
  const inset = props.isRinged ? 4.5 : 2.5;
  return (
    <Icon className="still-reading-icon" viewBox={`0 0 ${String(size)} ${String(size)}`}>
      {props.isRinged ? (
        <circle className="still-reading-icon__ring" cx={centre} cy={centre} r={15} />
      ) : null}
      <circle className="still-reading-icon__disc" cx={centre} cy={centre} r={13} />
      <g transform={`translate(${String(inset)} ${String(inset)}) scale(0.875)`}>
        <path className="still-reading-icon__caps" d="M4.6 4h14.8M4.6 20h14.8" />
        <path
          className="still-reading-icon__glass"
          d="M6.4 4.6c0 4.4 4.3 5.2 4.3 7.4s-4.3 3-4.3 7.4M17.6 4.6c0 4.4-4.3 5.2-4.3 7.4s4.3 3 4.3 7.4"
        />
        <path
          className="still-reading-icon__sand"
          d="M6.4 19.4c0-4.4 4.3-5.2 4.3-7.4h2.6c0 2.2 4.3 3 4.3 7.4z"
        />
      </g>
    </Icon>
  );
}

/**
 * The watched check (D370 §2): a white tick, drawn by Icon, less intrusive than a colour ("Maybe a
 * white checkmark is less intrusive and would stop people complaining?", the owner). Decoration beside
 * words that say it.
 */
export function WatchedIcon(): JSX.Element {
  return (
    <Icon className="watched-icon">
      <path className="watched-icon__tick" d="M5 12.5l4.5 4.5L19 7.5" />
    </Icon>
  );
}

// ---- Cast's menu ----

/*
 * The marks in Cast's menu of the kind of cast each entry makes (D465 §3; plans/google-cast.md §1):
 * the laurel for a device that takes it as one of Orpheum's own clients, Google's Cast icon for a
 * Chromecast, and a magnifying glass before a browser's own picker. The Cast icon and the magnifying
 * glass are Material Symbols' `cast` and `search` (Outlined, weight 400, 24 px; © Google, Apache-2.0,
 * github.com/google/material-design-icons), their paths inlined as the client's other icons are, so
 * no package is added.
 */

/** Google's Cast icon: a screen, the waves of a cast in its corner (Material Symbols' `cast`). */
export function GoogleCastIcon(): JSX.Element {
  return (
    <Icon className="cast-kind cast-kind--chromecast" viewBox="0 -960 960 960">
      <path d="M480-480Zm320 320H600q0-20-1.5-40t-4.5-40h206v-480H160v46q-20-3-40-4.5T80-680v-40q0-33 23.5-56.5T160-800h640q33 0 56.5 23.5T880-720v480q0 33-23.5 56.5T800-160Zm-720 0v-120q50 0 85 35t35 85H80Zm200 0q0-83-58.5-141.5T80-360v-80q117 0 198.5 81.5T360-160h-80Zm160 0q0-75-28.5-140.5t-77-114q-48.5-48.5-114-77T80-520v-80q91 0 171 34.5T391-471q60 60 94.5 140T520-160h-80Z" />
    </Icon>
  );
}

/** A magnifying glass, before a browser's own picker of devices (Material Symbols' `search`). */
export function FindDevicesIcon(): JSX.Element {
  return (
    <Icon className="cast-kind cast-kind--find" viewBox="0 -960 960 960">
      <path d="M784-120 532-372q-30 24-69 38t-83 14q-109 0-184.5-75.5T120-580q0-109 75.5-184.5T380-840q109 0 184.5 75.5T640-580q0 44-14 83t-38 69l252 252-56 56ZM380-400q75 0 127.5-52.5T560-580q0-75-52.5-127.5T380-760q-75 0-127.5 52.5T200-580q0 75 52.5 127.5T380-400Z" />
    </Icon>
  );
}

// ---- Posters, Watch Together's Talk button and the volume row ----

/**
 * A clock, faded and still, where a poster is still to come: behind a tile's image until it is
 * drawn, and on a blank whose entry has not been read (the owner, 2026-11-11: "a faded,
 * non-animating clock").
 */
export function PosterClock(): JSX.Element {
  return (
    <Icon className="poster__clock">
      <g
        fill="none"
        stroke="currentColor"
        strokeWidth="1.5"
        strokeLinecap="round"
        strokeLinejoin="round"
      >
        <circle cx="12" cy="12" r="9" />
        {/* The hands meet a little left of and below the middle, so their weight, up and to the
            right, looks centred (the owner, 2026-11-11). */}
        <path d="M11.5 6.6v5.9l3.5 2.7" />
      </g>
    </Icon>
  );
}

/**
 * A film's clapperboard, open, drawn in the text's colour: the poster of a title that has none
 * yet, as one not identified has (the owner, 2026-10-06: "a subdued clapboard").
 */
export function Clapperboard(): JSX.Element {
  return (
    <Icon className="poster__clapperboard" viewBox="0 0 64 64">
      <g fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinejoin="round">
        <rect x="9" y="30" width="46" height="26" rx="3" />
        <path d="M9 38h46" />
        <path d="M9 29 52.5 17.5l2 7.2L11 36.2" />
      </g>
      <g fill="currentColor">
        <path d="M14 30h7l-4 8h-7z" />
        <path d="M28 30h7l-4 8h-7z" />
        <path d="M42 30h7l-4 8h-7z" />
        <path d="m20 27.4 6.8-1.8-1.5 8.6-6.8 1.8z" />
        <path d="m34 23.7 6.8-1.8-1.5 8.6-6.8 1.8z" />
        <path d="m48 20 4.4-1.2 1.4 5-5.3 1.4z" />
      </g>
    </Icon>
  );
}

/** The film's mark in the volume row: a strip of film. */
export function FilmStripIcon(): JSX.Element {
  return (
    <Icon className="volume-row__film-icon">
      <path
        fillRule="evenodd"
        d="M4 3h16v18H4zm2 2v2h2V5zm10 0v2h2V5zM6 9v2h2V9zm10 0v2h2V9zM6 13v2h2v-2zm10 0v2h2v-2zM6 17v2h2v-2zm10 0v2h2v-2zM10 5v14h4V5z"
      />
    </Icon>
  );
}

/** A microphone, struck through while off. */
export function MicrophoneIcon(props: { readonly isOn: boolean }): JSX.Element {
  return (
    <Icon className="together-voice__icon">
      {props.isOn ? (
        <path d="M12 14a3 3 0 0 0 3-3V5a3 3 0 0 0-6 0v6a3 3 0 0 0 3 3zm5-3a5 5 0 0 1-10 0H5a7 7 0 0 0 6 6.9V21h2v-3.1A7 7 0 0 0 19 11z" />
      ) : (
        <path d="M19 11h-2a5 5 0 0 1-.6 2.4l1.5 1.5A7 7 0 0 0 19 11zM15 5a3 3 0 0 0-6 0v.2l6 6V5zM4.3 3 3 4.3l6 6V11a3 3 0 0 0 4.4 2.6l1.6 1.6A5 5 0 0 1 7 11H5a7 7 0 0 0 6 6.9V21h2v-3.1a7 7 0 0 0 3.4-1.3l3.3 3.3 1.3-1.3z" />
      )}
    </Icon>
  );
}

// ---- Settings ----

/** A circled i: the mark of the button that shows what a setting is for. */
export function AboutIcon(): JSX.Element {
  return (
    <Icon className="settings__icon">
      <path d="M12 2a10 10 0 1 0 0 20 10 10 0 0 0 0-20zm0 2a8 8 0 1 1 0 16 8 8 0 0 1 0-16zm-1 5h2V7h-2zm0 8h2v-6h-2z" />
    </Icon>
  );
}

/** A bin: the mark of the button that returns a setting to its starting value. */
export function ResetIcon(): JSX.Element {
  return (
    <Icon className="settings__icon">
      <path d="M9 3v1H4v2h16V4h-5V3zM5 8l1 13h12l1-13zm4 2h2v9H9zm4 0h2v9h-2z" />
    </Icon>
  );
}

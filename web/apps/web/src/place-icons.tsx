// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";

import { Icon } from "./icon.tsx";

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

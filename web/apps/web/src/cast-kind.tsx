// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";

import { Icon } from "./icon.tsx";
import { laurelMark } from "./laurel.ts";

/*
 * The marks in Cast's menu of the kind of cast each entry makes (D465 §3; plans/google-cast.md §1):
 * the laurel for a device that takes it as one of Orpheum's own clients, Google's Cast icon for a
 * Chromecast, and a magnifying glass before a browser's own picker. The Cast icon and the magnifying
 * glass are Material Symbols' `cast` and `search` (Outlined, weight 400, 24 px; © Google, Apache-2.0,
 * github.com/google/material-design-icons), their paths inlined as the client's other icons are, so
 * no package is added.
 */

/** The platform a Chromecast is paired under (D288; web/apps/cast-receiver's pairThisDevice). */
export const chromecastPlatform = "chromecast";

/** The kind of cast a device of the person's takes: as a Chromecast, or as Orpheum's own client. */
export type CastKind = "chromecast" | "orpheum";

/** The kind of cast a device paired under platform takes. */
export function castKindOf(platform: string): CastKind {
  return platform === chromecastPlatform ? "chromecast" : "orpheum";
}

/** The text of an image that is decoration alone: none, so a screen reader passes it by. */
const decorativeText = "";

/**
 * The mark before a device in Cast's menu for the kind of cast it takes: decoration, its name beside
 * it saying which device it is, so hidden from a screen reader as every icon is (icon.tsx).
 */
export function CastKindMark(props: { readonly kind: CastKind }): JSX.Element {
  return props.kind === "chromecast" ? (
    <GoogleCastIcon />
  ) : (
    <img className="cast-kind cast-kind--orpheum" src={laurelMark} alt={decorativeText} />
  );
}

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

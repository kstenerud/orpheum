// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";

import { GoogleCastIcon } from "./icons.tsx";
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

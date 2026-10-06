// SPDX-License-Identifier: MPL-2.0

import "./poster.css";

import { artUrl } from "@orpheum/sdk";
import type { JSX } from "react";
import { useTranslation } from "react-i18next";

/** A poster, or a named blank where there is none; the card's own text names the item. */
export function Poster(props: { readonly sourceSha256: string | undefined }): JSX.Element {
  const { t } = useTranslation();
  if (props.sourceSha256 === undefined) {
    return (
      <span className="poster poster--blank" aria-hidden="true">
        <Clapperboard />
        <span className="poster__caption">{t("common.noPoster")}</span>
      </span>
    );
  }
  return (
    <img
      className="poster"
      src={artUrl(props.sourceSha256, "portrait-m")}
      alt={""}
      loading="lazy"
      width={240}
      height={360}
    />
  );
}

/**
 * A film's clapperboard, open, drawn in the text's colour: the poster of a title that has none
 * yet, as one not identified has (the owner, 2026-10-06: "a subdued clapboard").
 */
function Clapperboard(): JSX.Element {
  return (
    <svg className="poster__clapperboard" viewBox="0 0 64 64" focusable="false">
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
    </svg>
  );
}

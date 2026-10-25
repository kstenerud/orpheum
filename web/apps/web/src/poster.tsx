// SPDX-License-Identifier: MPL-2.0

import "./poster.css";

import { artUrl } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

/**
 * A poster, or a named blank where there is none, or where its image does not load: one not yet
 * made in the sizes for screens answers "no such image" (the owner, Fire TV, 2026-10-25: Copper Kettle's
 * seasons showed broken images while their art waited its turn). The card's own text names the item.
 */
export function Poster(props: { readonly sourceSha256: string | undefined }): JSX.Element {
  const { t } = useTranslation();
  const [failedSha256, setFailedSha256] = useState<string | undefined>(undefined);
  if (props.sourceSha256 === undefined || failedSha256 === props.sourceSha256) {
    return (
      <span className="poster poster--blank" aria-hidden="true">
        <Clapperboard />
        <span className="poster__caption">{t("common.noPoster")}</span>
      </span>
    );
  }
  const sha256 = props.sourceSha256;
  return (
    <img
      className="poster"
      src={artUrl(sha256, "portrait-m")}
      alt={""}
      loading="lazy"
      width={240}
      height={360}
      onError={() => {
        setFailedSha256(sha256);
      }}
    />
  );
}

/**
 * A season's or an episode's still, or a blank of its size where there is none or its image does
 * not load, as a poster's (Poster).
 */
export function Still(props: {
  readonly sourceSha256: string | undefined;
  readonly className: string;
}): JSX.Element {
  const [failedSha256, setFailedSha256] = useState<string | undefined>(undefined);
  if (props.sourceSha256 === undefined || failedSha256 === props.sourceSha256) {
    return <span className={props.className} aria-hidden="true" />;
  }
  const sha256 = props.sourceSha256;
  return (
    <img
      className={props.className}
      src={artUrl(sha256, "landscape-s")}
      alt={""}
      loading="lazy"
      onError={() => {
        setFailedSha256(sha256);
      }}
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

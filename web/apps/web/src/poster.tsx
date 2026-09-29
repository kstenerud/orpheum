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
        {t("common.noPoster")}
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

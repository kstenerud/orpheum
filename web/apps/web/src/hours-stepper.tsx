// SPDX-License-Identifier: MPL-2.0

import "./libraries-words.ts";

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { mostHoursBetweenChecks } from "./library-draft.ts";
import { Stepper } from "./stepper.tsx";

/**
 * The hours between a library's checks, stepped an hour at a time from 1 to a week: as a library
 * is created, and on its page (D541 §1; the owner, 2026-11-23: the Stepper everywhere).
 */
export function HoursStepper(props: {
  readonly hours: number;
  readonly onChange: (hours: number) => void;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <Stepper
      label={t("libraries.checkEvery")}
      fieldLabel={t("libraries.checkEveryField")}
      valueText={t("libraries.checkEveryWords", { count: props.hours })}
      value={props.hours}
      min={1}
      max={mostHoursBetweenChecks}
      step={1}
      unit={t("libraries.checkEveryUnit")}
      stepDownLabel={t("libraries.checkEveryLess")}
      stepUpLabel={t("libraries.checkEveryMore")}
      onChange={props.onChange}
    />
  );
}

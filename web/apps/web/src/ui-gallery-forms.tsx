// SPDX-License-Identifier: MPL-2.0

import "./gallery-words.ts";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { Checkbox, RadioRows } from "./checkbox.tsx";
import { Switch } from "./switch.tsx";

/**
 * The gallery's form parts (D538; ui-gallery.tsx): each control a form or a setting is made of, as
 * it is pressed, so a look is checked in one place.
 */
export function FormParts(): JSX.Element {
  const { t } = useTranslation();
  return (
    <section aria-labelledby="gallery-forms">
      <h2 id="gallery-forms">{t("gallery.forms")}</h2>
      <GallerySwitches />
      <GalleryChoices />
    </section>
  );
}

/** Two switches, one off and one on with its status. */
function GallerySwitches(): JSX.Element {
  const { t } = useTranslation();
  const [isFirstOn, setIsFirstOn] = useState(false);
  const [isSecondOn, setIsSecondOn] = useState(true);
  return (
    <div className="ui-gallery__forms">
      <Switch label={t("gallery.switch")} isOn={isFirstOn} onSwitch={setIsFirstOn} />
      <Switch
        label={t("gallery.switchWithStatus")}
        status={t("gallery.switchStatus")}
        isOn={isSecondOn}
        onSwitch={setIsSecondOn}
      />
    </div>
  );
}

/** A checkbox and a radio group, as a form's Save confirms them. */
function GalleryChoices(): JSX.Element {
  const { t } = useTranslation();
  const [isChecked, setIsChecked] = useState(true);
  const [chosen, setChosen] = useState<"films" | "series">("films");
  return (
    <div className="ui-gallery__forms">
      <Checkbox
        label={t("gallery.checkbox")}
        hint={t("gallery.checkboxHint")}
        isChecked={isChecked}
        onChange={setIsChecked}
      />
      <RadioRows
        legend={t("gallery.radios")}
        name="gallery-kind"
        choices={[
          { value: "films", label: t("gallery.radioFilms") },
          { value: "series", label: t("gallery.radioSeries"), hint: t("gallery.radioSeriesHint") },
        ]}
        chosen={chosen}
        onChoose={setChosen}
      />
    </div>
  );
}

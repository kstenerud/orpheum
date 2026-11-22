// SPDX-License-Identifier: MPL-2.0

import "./gallery-words.ts";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

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

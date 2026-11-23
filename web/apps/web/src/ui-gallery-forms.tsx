// SPDX-License-Identifier: MPL-2.0

import "./gallery-words.ts";

import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { Checkbox, RadioRows } from "./checkbox.tsx";
import { missingText, TextField, useFormFields } from "./field.tsx";
import { HelpButton } from "./help-button.tsx";
import { PasswordField } from "./password-field.tsx";
import { submitWith } from "./form-row.tsx";
import { SettingControl, type Setting } from "./server-settings.tsx";
import { SubmitButton } from "./submit-button.tsx";
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
      <GalleryFields />
      <GalleryNumbers />
      <GallerySubmit />
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

/** Fields in each state: with a hint, optional, with an error; a password; and a help button. */
function GalleryFields(): JSX.Element {
  const { t } = useTranslation();
  const [name, setName] = useState("");
  const [note, setNote] = useState("");
  const [port, setPort] = useState("70000");
  const [password, setPassword] = useState("");
  return (
    <div className="ui-gallery__forms">
      <TextField
        label={t("gallery.field")}
        hint={t("gallery.fieldHint")}
        value={name}
        onChange={setName}
      />
      <TextField label={t("gallery.fieldOptional")} isOptional value={note} onChange={setNote} />
      <div className="ui-gallery__field-with-help">
        <TextField
          label={t("gallery.fieldError")}
          inputMode="numeric"
          error={t("gallery.fieldErrorText")}
          value={port}
          onChange={setPort}
        />
        <HelpButton label={t("gallery.help")} help={t("gallery.helpText")} />
      </div>
      <PasswordField
        label={t("gallery.password")}
        hint={t("gallery.passwordHint")}
        autoComplete="new-password"
        value={password}
        onChange={setPassword}
      />
    </div>
  );
}

/** The bounds of the gallery's wide number: wider than a few steps, as a setting's can be. */
const galleryNumberBounds = { min: 1, max: 1000 } as const;

/** Where the gallery's wide number starts. */
const galleryNumberStart = 20;

/**
 * A whole number with wide bounds, drawn as Settings draws one (server-settings.tsx's
 * SettingControl): a Stepper, its number typed or stepped, never the browser's number field (the
 * owner, 2026-11-23: "We'll use the one designed by the agent instead"; VQ382).
 */
function GalleryNumbers(): JSX.Element {
  const { t } = useTranslation();
  const [value, setValue] = useState<number>(galleryNumberStart);
  const { min, max } = galleryNumberBounds;
  const label = t("gallery.numberLabel");
  const setting: Setting = {
    key: "galleryMostTitles",
    kind: "integer",
    section: "general",
    description: label,
    isImportant: true,
    isModified: value !== galleryNumberStart,
    minimum: min,
    maximum: max,
    setAt: 0,
    startingValue: galleryNumberStart,
    value,
  };
  return (
    <section aria-labelledby="gallery-numbers">
      <h3 id="gallery-numbers">{t("gallery.numbers")}</h3>
      <div className="ui-gallery__forms">
        <SettingControl
          setting={setting}
          label={label}
          onSave={(_key, saved) => {
            if (typeof saved === "number") {
              setValue(saved);
            }
          }}
        />
      </div>
    </section>
  );
}

/**
 * A submit form as every one works: its button always enabled, a press with a field empty saying so
 * at the field and moving focus there, and one with all filled busy, its spinner turning, until
 * pressed again here, which stands for the answer coming.
 */
function GallerySubmit(): JSX.Element {
  const { t } = useTranslation();
  const [password, setPassword] = useState("");
  const [isBusy, setIsBusy] = useState(false);
  const fields = useFormFields(undefined, []);
  return (
    <form
      className="ui-gallery__forms"
      aria-label={t("gallery.submitForm")}
      noValidate
      onSubmit={submitWith(() => {
        if (fields.check([["password", missingText(t, t("gallery.password"), password)]])) {
          setIsBusy(true);
        }
      })}
    >
      <PasswordField
        {...fields.fieldOf("password")}
        label={t("gallery.password")}
        autoComplete="current-password"
        value={password}
        onChange={fields.editing("password", setPassword)}
      />
      <SubmitButton label={t("gallery.submit")} isBusy={isBusy} />
      <button
        type="button"
        onClick={() => {
          setIsBusy(false);
        }}
      >
        {t("gallery.submitAnswer")}
      </button>
    </form>
  );
}

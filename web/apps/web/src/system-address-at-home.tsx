// SPDX-License-Identifier: MPL-2.0

import "./system-words.ts";

import type { TFunction } from "i18next";
import type { components } from "@orpheum/sdk";
import { useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { dateTimeText } from "./date-time-text.ts";
import { i18n } from "./i18n.ts";
import { problemText, throwIfRefused } from "./problem.ts";
import { useServices } from "./services.tsx";
import { useRequest } from "./use-request.ts";
import { StatusNote } from "./status-note.tsx";
import { missingText, TextField, useFormFields } from "./field.tsx";
import { SubmitButton } from "./submit-button.tsx";

export type AddressAtHome = components["schemas"]["AddressAtHomeBody"];

/**
 * The server's address in the house, which devices at home reach it at (D452 §3, §4): the address,
 * how the server came to know it, and, unless the configuration names it, a form for the admin to
 * set it.
 */
export function SystemAddressAtHome(props: {
  readonly addressAtHome: AddressAtHome;
  /** The admin set the address: read the report again. */
  readonly onChanged?: (() => void) | undefined;
}): JSX.Element {
  const { t } = useTranslation();
  return (
    <section aria-labelledby="system-address-at-home">
      <h3 id="system-address-at-home">{t("system.addressAtHome.heading")}</h3>
      <AddressFacts addressAtHome={props.addressAtHome} />
      <AddressForm onChanged={props.onChanged} />
    </section>
  );
}

/** The address, or that none is known yet, and how it was known. */
function AddressFacts(props: { readonly addressAtHome: AddressAtHome }): JSX.Element {
  const { t } = useTranslation();
  const { address } = props.addressAtHome;
  if (address === "") {
    return <p>{t("system.addressAtHome.none")}</p>;
  }
  return (
    <p>
      {t("system.addressAtHome.address", { address })}{" "}
      {originText(t, props.addressAtHome, i18n.language)}
    </p>
  );
}

/** How the address was known, in words: learned from a device, set by an admin, guessed. */
function originText(t: TFunction, known: AddressAtHome, language: string): string {
  const at = known.at === undefined ? "" : dateTimeText(known.at, language);
  switch (known.origin) {
    case "learned":
      return t("system.addressAtHome.learned", { device: known.learnedFrom ?? "", at });
    case "set":
      return setText(t, known.setByName, at);
    case "guessed":
      return t("system.addressAtHome.guessed");
    case "":
      return "";
  }
}

/** Who set the address and when; a removed admin's name is no longer known. */
function setText(t: TFunction, name: string | undefined, at: string): string {
  return name === undefined || name === ""
    ? t("system.addressAtHome.setByRemoved", { at })
    : t("system.addressAtHome.set", { name, at });
}

/** The admin's form setting the address, saying in plain words why the server refused one. */
function AddressForm(props: { readonly onChanged?: (() => void) | undefined }): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [address, setAddress] = useState("");
  const { isBusy, problem, runRequest } = useRequest((cause) =>
    problemText(t, cause, {
      conflict: t("system.addressAtHome.refusedNotKept"),
      invalidRequest: t("system.addressAtHome.refusedNotPrivate"),
    }),
  );
  const save = (): void => {
    runRequest(async () => {
      throwIfRefused(
        await api.PUT("/system/address-at-home", { body: { address: address.trim() } }),
      );
      setAddress("");
      props.onChanged?.();
    });
  };
  const fields = useFormFields(undefined, []);
  return (
    <form
      noValidate
      onSubmit={(event) => {
        event.preventDefault();
        if (
          fields.check([
            ["address", missingText(t, t("system.addressAtHome.field"), address.trim())],
          ])
        ) {
          save();
        }
      }}
    >
      <TextField
        {...fields.fieldOf("address")}
        label={t("system.addressAtHome.field")}
        inputMode="decimal"
        value={address}
        onChange={fields.editing("address", setAddress)}
      />
      <SubmitButton label={t("system.addressAtHome.save")} isBusy={isBusy} />
      {problem === undefined ? null : <StatusNote told={{ kind: "problem", text: problem }} />}
    </form>
  );
}

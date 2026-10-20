// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { QrCode } from "./qr-code.tsx";

/**
 * The server's home address, which reaches it directly in the house (D292 §6): a link, and a QR
 * code a phone's camera opens; at home it is the way an admin administers (D297 §6). Nothing when
 * the server has none ready.
 */
export function HomeAddress(props: { readonly addresses: readonly string[] }): JSX.Element | null {
  const { t } = useTranslation();
  const [address] = props.addresses;
  if (address === undefined) {
    return null;
  }
  return (
    <div className="home-address">
      <p>
        {t("homeAddress.text")} <a href={address}>{address}</a>
      </p>
      <QrCode text={address} label={t("homeAddress.qr")} />
    </div>
  );
}

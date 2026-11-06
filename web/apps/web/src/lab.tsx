// SPDX-License-Identifier: MPL-2.0

import "./lab.css";

import "./lab-words.ts";

import { use, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { LabCast } from "./lab-cast.tsx";
import { LabDevice } from "./lab-device.tsx";
import { LabExtend } from "./lab-extend.tsx";
import { LabInput } from "./lab-input.tsx";
import { LabRemote } from "./lab-remote.tsx";
import { LabSound } from "./lab-sound.tsx";
import { LabThroughput } from "./lab-throughput.tsx";
import { Link } from "./link.tsx";
import { DeviceTestsOfferedContext } from "./lab-offered.ts";
import { routeHref, type LabPage } from "./router.ts";

/**
 * The device lab (plans/tv-device-tests.md): a menu of probes, each run on a device to settle how
 * casting, extending, a phone's remote and a TV's controller input are built, the sound timing test,
 * which finds and keeps the device's sound offset, and a way to the UI
 * gallery, to see the toolkit's widgets on the device. Reached at /lab, a
 * few presses on a TV's remote, or by Device tests in the menu. Only while the server offers the
 * tests (should_offer_device_tests): off, every page says so in place of its probe, and nothing
 * while the client settings are still read.
 */
export function Lab(props: { readonly probe: LabPage | undefined }): JSX.Element | null {
  const { t } = useTranslation();
  const isOffered = use(DeviceTestsOfferedContext);
  if (isOffered === undefined) {
    return null;
  }
  if (!isOffered) {
    return (
      <section className="lab" aria-labelledby="lab-heading">
        <h1 id="lab-heading">{t("lab.heading")}</h1>
        <p className="lab__lead">{t("lab.turnedOff")}</p>
      </section>
    );
  }
  return <LabProbe probe={props.probe} />;
}

/** The probe the address names, or the lab's menu. */
function LabProbe(props: { readonly probe: LabPage | undefined }): JSX.Element {
  switch (props.probe) {
    case "device":
      return <LabDevice />;
    case "input":
      return <LabInput />;
    case "sound":
      return <LabSound />;
    case "remote":
      return <LabRemote />;
    case "extend":
      return <LabExtend />;
    case "throughput":
      return <LabThroughput />;
    case "cast":
      return <LabCast />;
    case undefined:
      return <LabMenu />;
  }
}

/**
 * The lab's menu: a vertical list of big links, which a browser's own spatial navigation, Tab and a
 * pointer all reach. The cast probe is opened by its own load, never in place, since only its page
 * may load Google's Cast SDK (the core's policy for /lab/cast).
 */
function LabMenu(): JSX.Element {
  const { t } = useTranslation();
  const inPlace: readonly LabPage[] = [
    "device",
    "input",
    "sound",
    "remote",
    "extend",
    "throughput",
  ];
  return (
    <section className="lab" aria-labelledby="lab-heading">
      <h1 id="lab-heading">{t("lab.heading")}</h1>
      <p className="lab__lead">{t("lab.explain")}</p>
      <ul className="lab__menu">
        {inPlace.map((probe) => (
          <li key={probe}>
            <Link to={{ screen: "lab", probe }} className="lab__menu-link">
              <span className="lab__menu-name">{t(`lab.menu.${probe}`)}</span>
              <span className="lab__menu-what">{t(`lab.menu.${probe}What`)}</span>
            </Link>
          </li>
        ))}
        <li>
          <a href={routeHref({ screen: "lab", probe: "cast" })} className="lab__menu-link">
            <span className="lab__menu-name">{t("lab.menu.cast")}</span>
            <span className="lab__menu-what">{t("lab.menu.castWhat")}</span>
          </a>
        </li>
        <li>
          {/* The toolkit's gallery, a few presses away on a TV, where its address is a pain to type
              (the owner, 2026-11-03). */}
          <Link to={{ screen: "ui-gallery" }} className="lab__menu-link">
            <span className="lab__menu-name">{t("lab.menu.uiGallery")}</span>
            <span className="lab__menu-what">{t("lab.menu.uiGalleryWhat")}</span>
          </Link>
        </li>
      </ul>
    </section>
  );
}

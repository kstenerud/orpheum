// SPDX-License-Identifier: MPL-2.0

import { use, useState, type JSX } from "react";
import { useTranslation } from "react-i18next";

import { listMyDevices, renameMyDevice } from "./accounts.ts";
import { chooseOk, okChoices, useOkChoice } from "./controller-choice.ts";
import { arrivalMark } from "./focus-navigation.ts";
import { DeviceTestsOfferedContext } from "./lab-offered.ts";
import { Link } from "./link.tsx";
import { problemText } from "./problem.ts";
import { useServices } from "./services.tsx";
import { Loaded } from "./loaded.tsx";
import { useRequest } from "./use-request.ts";
import { useResource } from "./use-resource.ts";
import { DeviceName } from "./your-devices.tsx";

/**
 * Which face button of a game controller is OK in this browser (controller-choice.ts): automatic, by
 * the controller's make, unless the person chose; kept in the browser, so shown whether or not the
 * browser is one of their devices.
 */
function ControllerChoice(): JSX.Element {
  const { t } = useTranslation();
  const chosen = useOkChoice();
  return (
    <>
      <h2>{t("thisDevice.controller.heading")}</h2>
      <p>{t("thisDevice.controller.explain")}</p>
      <fieldset>
        <legend>{t("thisDevice.controller.legend")}</legend>
        {okChoices.map((choice) => (
          <label key={choice}>
            <input
              type="radio"
              name="controller-ok"
              value={choice}
              checked={chosen === choice}
              onChange={() => {
                chooseOk(choice);
              }}
            />
            {t(`thisDevice.controller.${choice}`)}
          </label>
        ))}
      </fieldset>
    </>
  );
}

/**
 * The device lab's tests, while the server offers them (DeviceTestsOfferedContext): a test is of the
 * device in hand, so they are this page's, not the menu's, which they crowded for everyone else
 * (ui-audit-2026-11-04.md Q8, the owner's answer: "For testers"). The sound timing test is a link
 * of its own, as it sets this device's sound offset (the owner, 2026-11-06).
 */
function DeviceTests(): JSX.Element | null {
  const { t } = useTranslation();
  if (use(DeviceTestsOfferedContext) !== true) {
    return null;
  }
  return (
    <>
      <h2>{t("thisDevice.deviceTests.heading")}</h2>
      <p>
        <Link to={{ screen: "lab" }}>{t("thisDevice.deviceTests.open")}</Link>
      </p>
      <p>
        <Link to={{ screen: "lab", probe: "sound" }}>
          {t("thisDevice.deviceTests.soundTiming")}
        </Link>
      </p>
    </>
  );
}

/**
 * The settings of the device the person is using, in one place, each kept at once with no Save
 * (ui-audit-2026-11-04.md Q5): its name, kept on Enter or on leaving its field and then said Saved,
 * and which button of a game controller is OK, kept as it is chosen. How it makes sound is asked of
 * no one (D336 §4); its sound offset is set under the video. The person's other devices are on their account page.
 */
export function ThisDevice(): JSX.Element {
  const { t } = useTranslation();
  const { api } = useServices();
  const [devices] = useResource("my-devices", () => listMyDevices(api));
  const { problem, runRequest } = useRequest((cause) => problemText(t, cause));
  // The name last kept, shown in the one read's place: reading the list again would draw it anew,
  // taking focus from Rename. Said Saved, as the page has no Save (ui-audit-2026-11-04.md Q5).
  const [savedName, setSavedName] = useState<string | undefined>(undefined);
  return (
    <section className="this-device" aria-labelledby="this-device-heading">
      <h1 id="this-device-heading">{t("thisDevice.heading")}</h1>
      <p>{t("thisDevice.explain")}</p>
      <Loaded
        resource={devices}
        render={(list) => {
          const device = list.find((each) => each.isThisDevice);
          if (device === undefined) {
            return <p>{t("thisDevice.notListed")}</p>;
          }
          return (
            <>
              <h2>{t("thisDevice.name")}</h2>
              <div {...arrivalMark}>
                <DeviceName
                  device={savedName === undefined ? device : { ...device, name: savedName }}
                  onRename={(_, name) => {
                    setSavedName(undefined);
                    runRequest(async () => {
                      await renameMyDevice(api, device.id, name);
                      setSavedName(name);
                    });
                  }}
                />
                {savedName === undefined ? null : <output>{t("thisDevice.nameSaved")}</output>}
              </div>
              {problem === undefined ? null : <p role="alert">{problem}</p>}
            </>
          );
        }}
      />
      <ControllerChoice />
      <DeviceTests />
      <p>
        <Link to={{ screen: "account" }}>{t("thisDevice.elsewhere")}</Link>
      </p>
    </section>
  );
}

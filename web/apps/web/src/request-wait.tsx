// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import type { RequestState } from "./use-request.ts";

/**
 * What a person's request to an outside provider waits for, in words (D231 §12): the provider asked
 * us to wait, counting down to the try the request makes on its own; the check of the provider after
 * it was not answering; or a request past a couple of seconds. Nothing while it is not held back.
 */
export function RequestWait(props: {
  readonly request: RequestState;
  /** The provider's name: TMDB, OpenSubtitles. */
  readonly provider: string;
}): JSX.Element | null {
  const { t } = useTranslation();
  const { request, provider } = props;
  if (request.retryInSeconds !== undefined) {
    return (
      <output>
        {t("problem.wait.askedToWait", { provider, seconds: request.retryInSeconds })}
      </output>
    );
  }
  if (request.isBusy && request.isChecking) {
    return <output>{t("problem.wait.checking", { provider })}</output>;
  }
  if (request.isBusy && request.isSlow) {
    return <output>{t("problem.wait.stillWaiting", { provider })}</output>;
  }
  return null;
}

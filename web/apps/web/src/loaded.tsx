// SPDX-License-Identifier: MPL-2.0

import type { JSX } from "react";
import { useTranslation } from "react-i18next";

import { problemText } from "./problem.ts";
import type { Resource } from "./use-resource.ts";

/**
 * Shows what a resource holds, or that it is loading, or why it failed: in problemText's words,
 * or in describeProblem's for a screen that knows more about its failures, and that it is asked
 * for again by itself when its poller will.
 */
export function Loaded<T>(props: {
  readonly resource: Resource<T>;
  readonly render: (value: T) => JSX.Element;
  readonly describeProblem?: (cause: unknown) => string;
}): JSX.Element {
  const { t } = useTranslation();
  const { resource } = props;
  if (resource.state === "loading") {
    return <p className="title__loading">{t("common.loading")}</p>;
  }
  if (resource.state === "failed") {
    const describe = props.describeProblem ?? ((cause: unknown) => problemText(t, cause));
    const why = describe(resource.cause);
    return (
      <p role="alert">
        {resource.willAskAgain === true ? `${why} ${t("problem.askingAgain")}` : why}
      </p>
    );
  }
  return props.render(resource.value);
}

// SPDX-License-Identifier: MPL-2.0

import type { JSX, MouseEvent, ReactNode } from "react";

import { routeHref, type Route } from "./router.ts";
import { useServices } from "./services.tsx";

/** A link to a route: a real address, followed in place on a plain click. */
export function Link(props: {
  readonly to: Route;
  readonly className?: string;
  /** Its name for a screen reader and a pointer's hover, when its content is only an icon. */
  readonly label?: string;
  /** Whether it leads to the page shown now, marked for the eye and a screen reader alike. */
  readonly isCurrent?: boolean;
  /**
   * Whether following it replaces the current entry in the history rather than adding one: a view
   * of the same page, a season or a sort (D367 §9).
   */
  readonly isReplacing?: boolean;
  readonly children: ReactNode;
}): JSX.Element {
  const { navigation } = useServices();
  const follow = (event: MouseEvent<HTMLAnchorElement>): void => {
    const isPlain =
      event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey;
    if (isPlain) {
      event.preventDefault();
      if (props.isReplacing === true) {
        navigation.replace(props.to);
      } else {
        navigation.go(props.to);
      }
    }
  };
  return (
    <a
      href={routeHref(props.to)}
      className={props.className}
      aria-label={props.label}
      title={props.label}
      aria-current={props.isCurrent === true ? "page" : undefined}
      onClick={follow}
    >
      {props.children}
    </a>
  );
}

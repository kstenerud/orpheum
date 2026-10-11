// SPDX-License-Identifier: MPL-2.0

import type { JSX, MouseEvent, ReactNode } from "react";

import { routeHref, type Route } from "./router.ts";
import { useServices } from "./services.tsx";

/** A link to a route: a real address, followed in place on a plain click. */
export function Link(props: {
  readonly to: Route;
  readonly className?: string;
  /** Its name for a screen reader, when its content is only an icon. */
  readonly label?: string;
  readonly children: ReactNode;
}): JSX.Element {
  const { navigation } = useServices();
  const follow = (event: MouseEvent<HTMLAnchorElement>): void => {
    const isPlain =
      event.button === 0 && !event.metaKey && !event.ctrlKey && !event.shiftKey && !event.altKey;
    if (isPlain) {
      event.preventDefault();
      navigation.go(props.to);
    }
  };
  return (
    <a
      href={routeHref(props.to)}
      className={props.className}
      aria-label={props.label}
      onClick={follow}
    >
      {props.children}
    </a>
  );
}

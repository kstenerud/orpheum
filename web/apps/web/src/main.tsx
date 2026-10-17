// SPDX-License-Identifier: MPL-2.0

import { loaderConfigOf } from "./loader-config.ts";
import { loaderOf } from "./loader-interface.ts";

// One build, two roles (D308): every core serves it as its own client, and the discovery serves it
// as the loader, which finds a core and runs that core's own client in its place. The page says which
// it is: a discovery's page names its loader's configuration, and a client a loader runs finds the
// loader on the window; each loads only its own part.
if (loaderOf(window) === undefined && loaderConfigOf(document) !== undefined) {
  void import("./loader-main.tsx");
} else {
  void import("./client-main.tsx");
}

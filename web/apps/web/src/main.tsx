// SPDX-License-Identifier: MPL-2.0

import { tabStorage } from "./browser-storage.ts";
import { loaderConfigOf } from "./loader-config.ts";
import { loaderOf } from "./loader-interface.ts";
import { loadPart, waitMs } from "./part-load.ts";

const recovery = {
  tab: tabStorage(),
  reload: () => {
    window.location.reload();
  },
  wait: waitMs,
};

// One build, two roles (D308): every core serves it as its own client, and the discovery serves it
// as the loader, which finds a core and runs that core's own client in its place. The page says which
// it is: a discovery's page names its loader's configuration, and a client a loader runs finds the
// loader on the window; each loads only its own part, asked again when it does not come (part-load.ts).
if (loaderOf(window) === undefined && loaderConfigOf(document) !== undefined) {
  void loadPart("loader", async () => await import("./loader-main.tsx"), recovery);
} else {
  void loadPart("client", async () => await import("./client-main.tsx"), recovery);
}

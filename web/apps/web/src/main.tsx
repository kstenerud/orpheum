// SPDX-License-Identifier: MPL-2.0

import { tabStorage } from "./browser-storage.ts";
import { loadOwnPart, shellWaitMs } from "./part-load.ts";

// The page loads only its own part, the loader or the core's client, asked again when it does not
// come (part-load.ts).
void loadOwnPart(
  { window, document },
  {
    loader: async () => await import("./loader-main.tsx"),
    client: async () => await import("./client-main.tsx"),
  },
  {
    tab: tabStorage(),
    reload: () => {
      window.location.reload();
    },
    wait: shellWaitMs,
  },
);

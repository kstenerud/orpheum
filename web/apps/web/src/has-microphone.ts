// SPDX-License-Identifier: MPL-2.0

import { useEffect, useState } from "react";

/** What of the browser's media devices tells whether there is a microphone. */
export type MicrophoneFinder = Partial<
  Pick<
    MediaDevices,
    "enumerateDevices" | "getUserMedia" | "addEventListener" | "removeEventListener"
  >
>;

/**
 * Whether the device has a microphone, as the browser lists its devices: an audio input among them,
 * listed before any permission is asked, its name hidden till then; where the browser cannot list
 * them, whether it can open a microphone at all (getUserMedia). A television's browser lists none,
 * so Talk does not show on one playing alone (D440 §3).
 */
export async function findsMicrophone(media: MicrophoneFinder | undefined): Promise<boolean> {
  if (media === undefined) {
    return false;
  }
  if (media.enumerateDevices === undefined) {
    return media.getUserMedia !== undefined;
  }
  try {
    const listed = await media.enumerateDevices();
    return listed.some((device) => device.kind === "audioinput");
  } catch {
    return false;
  }
}

/** The page's media devices, undefined where it has none (an insecure page, a test's DOM). */
function pageMedia(): MicrophoneFinder | undefined {
  // An insecure page's navigator has no mediaDevices, whatever its type says.
  return typeof navigator === "undefined" || !("mediaDevices" in navigator)
    ? undefined
    : navigator.mediaDevices;
}

/**
 * Whether the device has a microphone (findsMicrophone), false until the browser has said, and read
 * again as a device is plugged in or out.
 */
export function useHasMicrophone(media: MicrophoneFinder | undefined = pageMedia()): boolean {
  const [hasMicrophone, setHasMicrophone] = useState(false);
  useEffect(() => {
    let isCurrent = true;
    const look = (): void => {
      void findsMicrophone(media).then((isFound) => {
        if (isCurrent) {
          setHasMicrophone(isFound);
        }
      });
    };
    look();
    media?.addEventListener?.("devicechange", look);
    return () => {
      isCurrent = false;
      media?.removeEventListener?.("devicechange", look);
    };
  }, [media]);
  return hasMicrophone;
}

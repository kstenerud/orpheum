// SPDX-License-Identifier: MPL-2.0

/**
 * The English catalogue's device part: the This device page (thisDevice) and the person's list of
 * devices (devices), kept apart so the catalogue fits in one read (en.ts).
 */
export const enDevices = {
  thisDevice: {
    heading: "This device",
    explain:
      "How this device plays, kept for this device alone. A change applies from the next film you start.",
    name: "Its name",
    elsewhere: "Your other devices are on your account page.",
    notListed: "This browser is not one of your devices: sign in again to choose how it plays.",
  },
  devices: {
    heading: "Your devices",
    thisDevice: "This device",
    signedIn: "Signed in",
    signedOut: "Signed out",
    addedAndUsed: "Added {{added}}, last used {{used}}",
    codeToSignBackIn: "Make a code to sign it back in",
    rename: "Rename",
    save: "Save",
    remove: "Remove",
    removeLabel: "Remove {{name}}; it is signed out",
    signOutElsewhere: "Sign out everywhere else",
  },
};

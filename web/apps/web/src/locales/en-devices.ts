// SPDX-License-Identifier: MPL-2.0

/**
 * The English catalogue's device part: the This device page (thisDevice) and the person's list of
 * devices (devices), kept apart so the catalogue fits in one read (en.ts).
 */
export const enDevices = {
  thisDevice: {
    heading: "This device",
    explain: "This device's own settings, kept for it alone.",
    name: "Its name",
    elsewhere: "Your other devices are on your account page.",
    notListed: "This browser is not one of your devices: sign in again to name it.",
    nameSaved: "Saved",
    soundOffsetNotKept: "This device could not keep its sound timing: {{problem}}",
    controller: {
      heading: "Game controller",
      explain:
        "Which face button of a game controller is OK; the other one is Back. Kept in this browser.",
      legend: "OK is",
      automatic: "Automatic, by the controller's make",
      bottom: "The bottom button",
      right: "The right button",
    },
  },
  devices: {
    heading: "Your devices",
    all: "All devices ({{count}})",
    thisDevice: "This device",
    signedIn: "Signed in",
    signedOut: "Signed out",
    addedAndUsed: "Added {{added}}, last used {{used}}",
    rename: "Rename",
    remove: "Remove",
    removeLabel: "Remove {{name}}; it is signed out",
    removeConsequence:
      "{{name}} is signed out, and signs in again only with a password or a code it shows.",
    removeConfirm: "Remove it",
    byCode: "Activated by a code",
    byCodeNotice:
      "A device activated by a code watches and hosts as you. If you don't know one of those below, remove it.",
    signOutElsewhere: "Sign out everywhere else",
    signOutElsewhereConsequence: "Every other device must sign in again.",
    signOutElsewhereConfirm: "Sign them out",
  },
};

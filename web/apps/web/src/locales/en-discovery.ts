// SPDX-License-Identifier: MPL-2.0

/**
 * The English catalogue's discovery part: the page a discovery's origin opens on, until the server
 * is reached, by a link or a realm sign-in (discovery), kept apart so the catalogue fits in one read
 * (en.ts).
 */
export const enDiscovery = {
  discovery: {
    link: {
      heading: "Your server's link",
      text: "This link leads to a server someone runs for you. Connect to it to sign in or join.",
      connect: "Connect",
    },
    connecting: "Connecting to the server…",
    none: {
      heading: "Which server?",
      text: "Open the link you were sent, or ask the person who runs your server for your link.",
    },
    notFound: {
      heading: "This link no longer finds its server",
      text: "Ask the person who sent it for a new one.",
    },
    changed: {
      heading: "Your household's server has changed",
      text: "The server this realm leads to is not the one this browser reached before.",
      since:
        "The server this realm leads to is not the one this browser reached before. The new one was connected on {{at}}.",
      ifNot:
        "If someone in your household set up a new server, go on. If no one did, stop here and tell whoever runs your server: someone else may have connected theirs to your account.",
      continue: "Go on to the new server",
      cancel: "Stop",
    },
    deadEnd: {
      heading: "Can't reach the server from this network",
      text: "Try your phone's mobile data, or another Wi-Fi. A VPN or a travel eSIM often can't reach a home server: turn it off, or use the phone's own mobile data. If it keeps happening, copy what happened and send it to the person who sent you this link.",
    },
    realm: {
      heading: "Sign in",
      realm: "Realm",
      realmHint: "The name of the server you were given, such as karlnas or karlnas.live.",
      name: "Name",
      password: "Password",
      remember: "This is my own device: remember my realm",
      submit: "Sign in",
      signingIn: "Signing in…",
      wrong: "The realm, name or password is wrong, or the server can't be reached right now.",
      unreached:
        "Your server was found, but can't be reached from this network right now. A VPN or a travel eSIM often can't reach a home server: turn it off, or try the phone's own mobile data or another Wi-Fi. If it keeps happening, copy what happened and send it to whoever runs your server.",
      again: "Your server couldn't be reached as this device last reached it. Sign in again.",
      orLink: "Or open the link you were sent.",
      useCode: "Get a code",
    },
  },
  loader: {
    notConfigured: "This page was served without what it needs to find a server.",
    activate: {
      heading: "Activate a TV",
      whichRealm: "Activate the TV in which realm?",
      notSignedIn:
        "This browser is not signed in to a household. Open this link on a phone or computer that is, or sign in here and then activate the TV.",
      copy: "Copy link",
      copied: "Copied.",
      refused: "This browser would not copy it; select the link above instead.",
      signInHere: "Sign in here",
    },
    households: {
      heading: "Your households",
      forget: "Forget",
      forgetNamed: "Forget {{name}}",
      forgetConsequence:
        "This browser will forget {{name}}. You can sign in to it again by its realm.",
      forgetConfirm: "Forget it",
      join: "Sign in to another household",
    },
    code: {
      heading: "Sign in with a code",
      scan: "Scan this with a phone where you are signed in, and tap Activate.",
      qr: "QR code that activates this TV",
      explain:
        "Or, on a phone or computer where you are signed in, open Your account and enter this code under Activate a device.",
      code: "Code",
      checkCode: "Check code",
      checkExplain:
        "Your other device now shows a check code too. If it is this same code, say so there.",
      scannedExplain: "Waiting for them to activate it…",
      renewed: "The last code ended, so here is a new one.",
      enteredBy: "Someone in the household {{household}} entered this code.",
      confirm: "Sign this TV in to the household {{household}}?",
      confirmAs: "Sign this TV in to the household {{household}} as {{person}}?",
      join: "Yes, sign in",
      notMine: "No, that's not mine",
      refused: "Nothing was signed in. Go back to ask for a new code.",
      signingIn: "Activated. Opening your server…",
      failed: "No code could be had just now. Go back and try again in a minute.",
      back: "Sign in with a realm, name and password instead",
    },
    refused:
      "This server's client needs version {{needs}} of the page that reaches it, which serves version {{has}}. Ask the person who runs your server to update it.",
  },
};

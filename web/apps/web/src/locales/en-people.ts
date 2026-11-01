// SPDX-License-Identifier: MPL-2.0

/**
 * The English catalogue's people part: setting up a server's first admin (setUp), the admin's People
 * page (people), a person's page (person) and the roles (roles), kept apart so the catalogue fits in
 * one read (en.ts).
 */
export const enPeople = {
  setUp: {
    heading: "Set up this server",
    explain:
      "This server has no admin yet. Enter the setup code from its log (or run `orpheum setup-code` on it), and choose the first admin's name and password.",
    code: "Setup code",
    submit: "Create the admin",
    back: "Back",
    wrongCode:
      "That setup code is wrong. It is in the server's log, or run `orpheum setup-code` on it.",
    alreadySetUp: "This server already has an admin. Go back and sign in.",
  },
  people: {
    heading: "People",
    nameTaken: "Someone already has that name. Choose another.",
    qrLabel: "{{name}}'s link as a QR code",
    add: "Add a person",
    name: "Name",
    role: "Role",
    roleUser: "Can watch",
    roleAdmin: "Administrator",
    setUp: "Set up an account",
    password: "Password",
    create: "Add the account",
    invite: "Invite someone",
    inviteExplain:
      "They choose their own name and password from the link. Your note is for you alone, so you can tell who is who once they have.",
    adminNote: "Who it is for (only you see this)",
    adminNoteExample: "Uncle Joe",
    makeLink: "Make their link",
    createRole: "Create Role",
    createAccount: "Create Account",
    inviteSomeone: "Invite Someone",
    invitations: "Invitations",
    accounts: "Accounts",
    issuedOn: "Issued {{at}}",
    issuedLabel: "Issued",
    invitationFor: "Invitation for {{note}}",
    invitationEnded: "Its links have ended. Withdraw it, and invite them again.",
    withdraw: "Withdraw",
    withdrawConsequence: "Every link sent for it stops working.",
    withdrawConfirm: "Withdraw it",
    registered: "Has a password",
    notRegistered: "Has not used their link yet",
    newLink: "New link",
    newLinkConsequence:
      "Any link sent to {{name}} before keeps working until one of them is used. Using one sets a new password, ends the others and signs {{name}} out everywhere else.",
    newLinkConfirm: "Make a new link",
    issued: "{{name}}'s link: send it to them",
  },
  person: {
    devices: "Their devices",
    rename: "Rename",
    saveNote: "Keep the note",
    changeRole: "Give this role",
    watched: "Watched {{hours}} hours of {{titles}} titles; last {{last}}",
    neverWatched: "never",
    remove: "Remove {{name}}…",
    removeConsequence:
      "{{name}} is signed out everywhere at once, and what they watched, their places in films and their lists are deleted. This cannot be undone.",
    removeConfirm: "Remove them",
    ownLink:
      "The link they were sent leads them back to this server until {{date}}; each visit renews it.",
    revokeOwnLink: "End that link…",
    revokeOwnLinkConsequence:
      "The link {{name}} was sent stops leading back to this server. They stay signed in on their devices, and a new link from you gives them another.",
    revokeOwnLinkConfirm: "End it",
  },
  roles: {
    heading: "Roles",
    explain:
      "Each person has one role, their whole access: the libraries they see and play, and what else they may do. A library taken from a role is gone for its people at once.",
    name: "Name",
    newTitle: "New role",
    dialogLabel: "Role",
    create: "Add the role",
    holders: "{{count}} people",
    notFound: "There is no such role.",
    capabilities: "What its people may do",
    libraries: "The libraries its people see",
    everyLibrary: "Every library: administrators see everything.",
    noLibrary:
      "No library is ticked for this role, so its people see no films or series. Tick the libraries they may watch.",
    noLibraryChosen:
      "This role has no library yet, so they will see no films or series until you tick some for it under Roles.",
    delete: "Delete the role",
    deleteConsequence: "No one can be given this role any more.",
    deleteConfirm: "Delete it",
    refused: "That cannot be done: the name is taken, someone holds the role, or it is built in.",
    capability: {
      administer: "Run the server (sees every library)",
      deleteMedia: "Delete media",
      download: "Download",
      editMetadata: "Fix identification and information",
      hostWatchTogether: "Host Watch Together",
      remoteAccess: "Watch from outside the house",
    },
  },
};

// SPDX-License-Identifier: MPL-2.0

/**
 * The English catalogue's people part: the admin's People page (people), a person's page (person)
 * and the roles (roles), kept apart so the catalogue fits in one read (en.ts).
 */
export const enPeople = {
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
    invitations: "Invitations not yet used",
    invitation: "{{note}}, as {{role}}, until {{until}}",
    withdraw: "Withdraw",
    withdrawConsequence: "Their link stops working.",
    withdrawConfirm: "Withdraw it",
    noted: "({{note}})",
    registered: "Has a password",
    notRegistered: "Has not used their link yet",
    newLink: "New link",
    newLinkConsequence: "{{name}}'s current link stops working.",
    newLinkConfirm: "Make a new link",
    issued: {
      link: "{{name}}'s link: send it to them",
      code: "A code for {{name}}'s device",
    },
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
  },
  roles: {
    heading: "Roles",
    explain:
      "Each person has one role, their whole access: the libraries they see and play, and what else they may do. A library taken from a role is gone for its people at once.",
    name: "Name",
    newName: "A new role's name",
    create: "Add the role",
    holders: "{{count}} people",
    capabilities: "What its people may do",
    libraries: "The libraries its people see",
    everyLibrary: "Every library: administrators see everything.",
    noLibrary:
      "No library is ticked for this role, so its people see no films or series. Tick the libraries they may watch.",
    noLibraryChosen:
      "This role has no library yet, so they will see no films or series until you tick some for it under Roles.",
    save: "Save",
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

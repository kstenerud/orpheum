> **ABOUTME:** Granular sharing — giving one person access to part of a library — which nobody
> in this field has solved and which is worth solving.

# Sharing model

- **Status:** UNSPECIFIED — needs design.
- **Depends on:** library-model.md

## The evidence that it is unsolved

- Immich: `Better sharing in Immich` — **1,020 reactions, 259 comments**, under a declared
  feature freeze. The most-reacted issue in the entire competitive survey.
- PhotoPrism: `Sharing: Multi-user / multi-library support` — **844 reactions, open since
  2019-02**.
- Jellyfin: users symlink subsets of their media into separate directories and register each as
  its own library, per user, to fake it. Named on HN as the #1 missing feature.
- Plex: solved it, via the account system everyone resents. That is not a coincidence — the
  account exists *because* sharing needs an identity for the other person.

## What needs deciding

What the unit of sharing is (library, collection, item, saved filter); how a shared user is
identified without a central account (D13); whether shared users get their own playback state
and where it lives; what happens to a share when the underlying item moves or is deleted; and
whether shares are transitive.

This is a data-model question as much as a product one, which is why it depends on
`library-model.md` — getting the model wrong here means a migration later.

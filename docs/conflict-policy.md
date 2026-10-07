# ClusterStor conflict policy

ClusterStor resolves conflicts in the provider-neutral sync layer, immediately before a pending local operation is sent to a storage provider.

The filesystem watcher does not decide conflicts, and Google Drive, OneDrive, Dropbox, and Box adapters must not invent their own user-facing conflict rules.

## Core safety rule

ClusterStor must not silently discard user data.

Before executing a queued local operation, the sync worker compares:

- the provider item identity the local operation was based on;
- the base ClusterStor/provider version;
- the base local path;
- the provider item's current identity, version, and path.

Each pending operation therefore records its base provider item ID, base version ID, base path, and base modified time when available.

## Conflict copy naming

When both copies must be preserved, the local copy is renamed to:

```text
<name> (<device> conflict YYYY-MM-DD HHMMSS)<extension>
```

Example:

```text
Report (LAPTOP-SRNCLDLL conflict 2026-10-07 093015).docx
```

The original extension is preserved. Windows-reserved characters are removed from the device name.

If that exact conflict filename already exists, the future worker must choose another non-destructive sibling name rather than overwrite it.

## Decision matrix

| Local intent | Remote state since base | Result |
| --- | --- | --- |
| New file | no remote item at target | apply local create |
| New file | unrelated remote item already at target | preserve remote original; upload local as conflict copy |
| Edit file | remote content unchanged | apply local edit |
| Edit file | remote renamed only, content unchanged | apply local content to the same remote identity at its new remote location; reconcile local path afterward |
| Edit file | remote content also changed | preserve remote original; upload local as conflict copy |
| Edit/move | remote item deleted | preserve local bytes as a recovered conflict copy; do not silently resurrect the deleted identity |
| Move/rename | remote content changed only | apply local move to the same provider identity, preserving the newer remote bytes |
| Move/rename | remote path also changed | keep remote location; suppress the local move and surface a conflict |
| Delete | remote unchanged | apply recoverable provider delete/trash |
| Delete | remote already deleted | no-op |
| Delete | remote edited or moved | suppress delete, keep remote item, surface conflict |
| Create folder | target unused | create folder |
| Create folder | target occupied by unrelated item | do not overwrite; preserve both with a non-colliding local name |
| Unknown/unsafe operation | any | keep remote and require explicit recovery rather than guessing |

## Edit/edit conflict

This is the most important case.

Given a common base version:

1. device A edits `Report.docx` locally;
2. device B or the provider edits the same remote file before A uploads;
3. A's worker sees that the current remote version no longer equals A's recorded base version;
4. A does **not** overwrite the remote bytes;
5. the remote version remains `Report.docx`;
6. A's local bytes become a sibling conflict copy;
7. both files remain available to the user;
8. Activity should eventually record the conflict and its resolution.

ClusterStor does not attempt automatic document merging in V1.

## Delete conflicts

Deletes receive more protection than normal edits because a mistaken conflict resolution would be destructive.

If the remote item changed in content or location after the local base version, a queued local delete is suppressed. The remote item remains intact and ClusterStor surfaces the conflict.

If the remote item was already deleted, the local delete becomes a no-op.

Permanent deletion is not part of V1 conflict resolution.

## Remote deletion versus local work

If a known remote item is deleted while a device has unsynchronized local edits or a local move, ClusterStor preserves the local bytes as a new recovered conflict copy.

The deleted provider identity is not silently resurrected. This respects the remote deletion while still preserving the local user's work.

## Move conflicts

Path changes and content changes are treated separately.

A local move can safely coexist with a remote content edit because moving the provider item does not require replacing its bytes.

Two concurrent moves are different: if both local and remote paths changed from the same base path, ClusterStor keeps the remote path, suppresses the local move, and surfaces the conflict. V1 does not guess which destination was more intentional.

## Identity before filename

Provider identity is authoritative.

ClusterStor must not decide that two items are the same merely because their filenames match. A path that now points to a different provider item is treated as a name collision and both sides are preserved.

## Version comparison

Preferred conflict comparison order:

1. provider/ClusterStor item identity;
2. provider-neutral version ID or provider revision mapped to a ClusterStor version;
3. modified timestamp only as a fallback when a stable version token is unavailable.

Provider adapters should expose enough metadata for the sync worker to produce a provider-neutral current snapshot.

## Offline behavior

Conflict checks happen when the worker is actually ready to execute an operation, not when the watcher first journals it.

This is critical because a device may remain offline for hours or days. The original base version stays attached to the pending operation, so reconnecting cannot make an old local change look current.

## Pause behavior

Paused sync continues collecting local operations and preserving their base metadata.

When sync resumes, every queued operation is conflict-checked against current remote state before execution.

## Provider worker contract

Before any destructive or overwriting provider call, the future sync worker must:

1. load the pending local operation;
2. fetch current provider-neutral remote metadata;
3. call the ClusterStor conflict policy;
4. execute only the returned action;
5. preserve conflict copies before acknowledging the original operation;
6. update local journal metadata only after provider success;
7. record an Activity event for conflicts and recovery copies.

No provider adapter may bypass this check for an operation that has a known remote base.

## Current implementation status

Implemented now:

- base provider item ID captured in pending operations;
- base ClusterStor/provider version captured;
- base local path captured;
- base modified time captured when available;
- provider-neutral remote snapshot type;
- provider-neutral conflict decision engine;
- deterministic conflict filename generation;
- tests for edit/edit, delete/edit, remote deletion, concurrent moves, name collision, and conflict filenames.

Not implemented until the sync-worker phase:

- fetching current Google Drive metadata immediately before execution;
- physically creating/uploading the conflict copy;
- reconciling the local path after a remote-only rename;
- Activity/UI conflict notifications;
- retry and recovery workflow around provider failures.

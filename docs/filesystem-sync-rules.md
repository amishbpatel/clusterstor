# ClusterStor filesystem sync rules

This document defines the Windows-local behavior implemented by the desktop watcher and sync journal before provider operations are allowed. The watcher is active in the developer agent; provider execution remains intentionally disabled.

## Core principle

The filesystem watcher never talks directly to Google Drive or another provider. It observes local filesystem activity, normalizes that activity into ClusterStor operations, and writes durable operations to the local journal. A separate sync worker will later execute those operations against the selected storage backend.

This separation prevents transient Windows events from immediately becoming remote destructive actions.

## Path model

- The user works through the selected mapped drive, for example `S:\`.
- The watcher observes the private backing root for that drive.
- Journal paths are stored relative to the backing root.
- Local path comparison on Windows is case-insensitive.
- A path outside the configured backing root is never journaled.
- ClusterStor internal state is stored outside the mapped drive and can never be interpreted as customer content.

## Files that are not user content

ClusterStor ignores known operating-system metadata:

- `desktop.ini`
- `Thumbs.db`
- `$RECYCLE.BIN`
- `System Volume Information`
- macOS `.DS_Store` if a drive is later shared across platforms

Office owner/lock files beginning with `~$` are also ignored.

Normal dotfiles are **not** ignored. A user-created file such as `.env.example` is legitimate content and must sync.

Temporary download names such as `.crdownload`, `.download`, and `.partial` are treated as transient writes. ClusterStor waits for the application to rename the completed file rather than uploading every intermediate partial state.

## Create

### Folder

A stable new directory becomes:

```text
create_folder
```

The operation includes the relative local path. Nested folders are ordered parent-first by the future sync worker.

### File

A new file becomes an `upsert_file` operation only after the write has settled.

Creating a file and then immediately writing it must result in **one logical upload**, not separate create and modify uploads.

## Edit

File write events are debounced for **2 seconds** by default.

Before an `upsert_file` operation becomes ready, the watcher should verify that file size and modification time remain unchanged across a **1-second stability window**.

If another write arrives during the debounce/stability period, the timer resets.

This prevents Office, browsers, editors, and large copy operations from creating a stream of unnecessary versions.

## Rename and move

A rename or move **within the ClusterStor drive** should become one logical `move` operation whenever the watcher can pair the old and new path.

This applies to:

- rename within one folder;
- moving a file to another folder;
- renaming a folder;
- moving a folder subtree.

The operation records both `old_local_path` and `local_path`.

If Windows delivers events that cannot safely be paired, ClusterStor waits up to **3 seconds** for the matching path event before falling back to reconciliation. The reconciliation layer may represent an unpaired move as delete + create, but must never guess identity solely from filename.

## Move into the drive

A file or folder moved from outside ClusterStor into the mapped drive is treated exactly like newly created content.

Files still receive the normal write-stability checks before becoming upload operations.

## Move out of the drive

Moving content out of the mapped drive is treated as a ClusterStor delete after the rename grace period expires.

The provider worker will later translate that delete to the provider's recoverable trash/delete mechanism rather than immediately performing irreversible destruction.

## Delete

A stable local removal becomes:

```text
delete
```

Deletes are durable journal operations. The sync worker must be idempotent so a crash/restart cannot issue multiple destructive provider operations.

Permanent provider deletion is **not** part of the initial desktop sync behavior. Initial deletes should use provider trash/recoverable semantics.

## Open or locked files

ClusterStor does not force access to files another application has locked.

If a stable file cannot be opened for upload:

1. leave the operation pending;
2. record a retryable local error;
3. retry with backoff;
4. surface a tray/UI error only after repeated failure.

A locked file must never cause the previous remote version to be deleted.

## Large files

Large files use the same logical `upsert_file` operation as small files.

The future provider worker decides whether the operation uses simple, resumable, or multipart upload based on provider capability and size. The watcher must not split large files into provider-specific operations.

## Event coalescing

Before a queued operation is executed, newer local events may collapse earlier events:

- create + repeated writes => one `upsert_file`
- write + write => one `upsert_file`
- create + rename => create at final path when no remote identity exists yet
- rename + rename => one move from the known original path to the latest path
- create + delete before upload => no remote operation
- write + delete => delete supersedes the pending write when the remote file already exists

Coalescing occurs locally and must preserve the latest user-visible filesystem state.

## Pause behavior

**Pause sync** means:

- continue observing filesystem changes;
- continue journaling/coalescing operations;
- do not execute provider transfers.

This is important because pausing ClusterStor must not cause changes made during the pause to disappear.

When sync resumes, the worker drains the durable journal in dependency-safe order.

## Crash and restart behavior

Filesystem events themselves are not the source of truth after a crash.

On startup, ClusterStor will eventually reconcile:

1. the current filesystem state;
2. the durable local journal;
3. the last known ClusterStor/provider metadata.

This catches changes that occurred while the agent was stopped and protects against watcher event loss.

A full provider rescan should not be the normal recovery mechanism.

## Initial operation vocabulary

The Windows watcher normalizes local activity into only four provider-neutral operation types:

- `create_folder`
- `upsert_file`
- `move`
- `delete`

Provider-specific concepts do not belong in the watcher.

Version creation, provider revision IDs, resumable upload sessions, quota checks, and remote trash behavior are responsibilities of the later sync worker/provider adapter.

## Conflict boundary

The watcher records what happened locally but does not decide whether a remote/local conflict exists.

Conflict detection happens immediately before a provider operation by comparing the local operation's known base version against current ClusterStor/provider metadata.

That keeps local filesystem event handling deterministic and leaves conflict policy in one provider-neutral synchronization layer.


## Implementation status

The developer Windows agent now implements this local pipeline:

- recursive watch registration for the private mapped-drive backing tree;
- startup identity snapshot without treating pre-existing files as new uploads;
- Windows file identity tracking for safe rename/move pairing;
- 2-second write debounce plus a 1-second stability check;
- recursive handling when folders are created or moved into ClusterStor;
- durable journal persistence after every coalesced operation change;
- rename grace before an unmatched rename/removal becomes a delete;
- ignored Windows metadata, Office lock files, and transient browser downloads;
- create/write/move/delete coalescing;
- watcher activity continues while Sync is paused.

The provider worker is still disabled, so pending operations remain in the local journal for inspection rather than being sent to Google Drive.


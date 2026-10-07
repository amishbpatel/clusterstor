# Google Drive desktop sync

This document describes the current ClusterStor developer-preview desktop sync path for Google Drive.

## Scope

The Windows agent now synchronizes normal files and folders between the local ClusterStor mapped drive and the dedicated Google Drive `ClusterStor` managed root.

The desktop sync layer uses the paired device credential. Google OAuth access and refresh tokens remain encrypted on the ClusterStor backend and are never returned to the desktop agent.

## Initial bootstrap

Desktop sync requires a complete provider tree rather than the web UI's lazy index.

Migration `000011_desktop_sync` adds:

- `provider_accounts.desktop_tree_bootstrapped_at`
- `provider_items.provider_revision_id`

On the first desktop snapshot for a Google account:

1. ClusterStor establishes a Google Drive change cursor if one is not already present.
2. ClusterStor recursively crawls the entire managed `ClusterStor` root.
3. Provider mappings are updated with Google `headRevisionId` values.
4. ClusterStor marks the desktop tree bootstrap complete.
5. Drive changes since the captured cursor are replayed.
6. The agent receives a complete provider-neutral snapshot.

The change cursor is captured before the crawl so changes made during the initial scan are not silently missed.

## Local to Google

The filesystem watcher writes provider-neutral operations into the durable journal.

The Google worker executes:

- `create_folder`
- `upsert_file`
- `move`
- `delete`

New files use Google resumable upload sessions. The file body travels from the desktop directly to the Google resumable-session URL instead of being proxied through the ClusterStor API.

Updates to known files use the existing provider identity and Google revision/version upload flow.

Deletes use Google Drive's recoverable trash behavior.

Renames and moves preserve provider identity.

## Google to local

On each sync pass, the backend advances the Google change cursor and returns current managed-root metadata.

The desktop agent:

- creates new remote folders locally;
- downloads new remote files;
- downloads new revisions when Google content changes;
- applies remote renames and moves;
- removes local resident copies when the remote item is deleted or moved outside the managed root;
- preserves watcher suppression around ClusterStor-generated local filesystem changes so those changes do not upload back as false local edits.

Normal resident-file downloads are staged to a temporary file and then replaced atomically enough to preserve the old local file if the replacement fails.

## Conflict safety

Every queued local operation stores the remote identity and revision it was based on.

Immediately before an operation executes, the agent compares that base to the current Google snapshot and uses the provider-neutral conflict policy.

Key behaviors include:

- edit/edit => preserve both;
- local delete + remote edit/move => suppress delete and restore current remote state locally;
- local move + remote content edit => apply the move, then hydrate the newer remote bytes;
- local move + remote move => preserve the remote location;
- remote delete + unsynced local edit => upload the local work as a recovered conflict copy.

Conflict filenames include the device name and timestamp.

## Changes while the agent is stopped

At startup, ClusterStor reconciles the current local filesystem against the durable journal before the watcher starts.

It queues:

- files/folders created while the agent was stopped;
- resident files edited while the agent was stopped;
- known resident files deleted while the agent was stopped.

Those operations retain the prior remote base revision so reconnecting still performs a normal conflict check.

Remote-only unavailable items are not mistaken for local deletions.

## Queue resilience

One failed operation does not stall the entire queue.

Failures remain pending and record:

- retry attempt count;
- last error.

Independent local operations and remote reconciliation continue when safe.

## Duplicate Google names

Google Drive permits multiple sibling items with the same name; Windows does not.

ClusterStor maps duplicates deterministically. One item keeps the original name and additional items receive a local alias containing a short Google item identifier, while retaining the original extension.

This prevents one duplicate from silently hiding another.

## Local storage behavior

This phase uses fully resident local files.

The Files On-Demand policy from `local-storage-files-on-demand.md` is enforced conceptually, but physical Windows Cloud Files placeholders/dehydration are not enabled yet.

Remote content is considered safe for later dehydration only after ClusterStor has verified the provider copy.

## Current limitation: Google-native Workspace files

Google-native Docs, Sheets, Slides, and similar `application/vnd.google-apps.*` items are included in metadata but are not downloaded as ordinary desktop files in this first sync slice.

They are marked locally unavailable rather than exported into a lossy alternate format.

A later policy can add explicit export/shortcut behavior without conflating native Google document identity with a binary file revision.

## Polling

The developer agent currently performs a sync pass approximately every 20 seconds.

The backend uses Google change cursors, so steady-state polling does not require a full tree crawl.

Future production work can replace or supplement polling with push/event mechanisms without changing the local journal contract.

## Security boundaries

The desktop agent stores only its ClusterStor device credential, protected with Windows DPAPI.

The agent does not store:

- ClusterStor account passwords;
- browser session tokens;
- Google OAuth access tokens;
- Google OAuth refresh tokens.

A Google resumable upload-session URL is short-lived transfer capability and must never be logged.

## Validation checklist

For the current Windows developer build:

1. apply migration `000011_desktop_sync`;
2. start the API with the existing Google OAuth environment;
3. start the agent;
4. confirm initial Google files appear under the mapped ClusterStor drive;
5. create/edit/rename/move/delete a normal local file and verify Drive changes;
6. create/edit/rename/move/delete a normal file inside the Google `ClusterStor` folder and verify local changes;
7. stop the agent, edit a resident local file, restart the agent, and verify startup reconciliation queues and syncs it;
8. inspect `sync-journal.json` and confirm successful operations leave no stale pending entries.

The developer build should be considered end-to-end validated only after these checks are exercised against a real connected Google account.

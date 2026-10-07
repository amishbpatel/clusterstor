# ClusterStor local storage and Files On-Demand

ClusterStor presents one logical filesystem while allowing each device to decide how much file content remains physically resident on that device.

This policy is provider-neutral. Google Drive, OneDrive, Dropbox, Box, ClusterStor Cloud, and future Peer Storage all use the same local-availability semantics.

## V1 local model

The first Windows sync implementation uses normal local files in the private ClusterStor backing directory. Files are fully resident when downloaded.

Windows Cloud Files placeholders are a later presentation/runtime layer. The policy in this document is implemented before that API integration so the same behavior can be reused by Windows and macOS.

The current SUBST-based mapped drive is not treated as a placeholder filesystem.

## User-visible availability choices

ClusterStor has three availability modes:

### Automatic

This is the default.

- Newly created local files remain local.
- Recently hydrated/downloaded content remains local while disk space is healthy.
- Once a verified remote copy exists, ClusterStor may reclaim local bytes under disk pressure.
- A future placeholder remains visible in the ClusterStor filesystem and rehydrates when opened.

### Always keep on this device

The file or folder must remain fully resident on the device.

If it is currently a placeholder, ClusterStor hydrates it.

This setting is suitable for files needed offline.

### Online-only

The file or folder remains visible in ClusterStor but local bytes may be removed after the remote copy is verified.

Opening an online-only placeholder hydrates it.

A file with unsynchronized local changes or an unresolved conflict cannot be dehydrated even if the user selected Online-only.

## Selective sync is different from Online-only

Selective sync controls whether a subtree participates on a specific device.

An excluded subtree is hidden from that device once it is safe to do so.

Online-only content remains visible as part of the filesystem but normally does not retain local bytes.

ClusterStor must never use exclusion as an excuse to discard pending local work. If a user excludes a folder that contains unsynchronized or conflicted content, the local bytes remain protected until those changes are safely resolved.

## Rule inheritance

Availability rules are device-specific and inherit by path.

For example:

```text
Projects                  Always keep
Projects\Archive          Online-only
Projects\Archive\2024    Excluded on this device
```

The most specific matching path wins.

A folder rule applies to descendants unless overridden by a deeper rule.

## Safety rule for removing local bytes

ClusterStor may dehydrate/remove resident file bytes only when all of the following are true:

1. a remote copy has been verified;
2. there is no pending local create/edit operation;
3. there is no unresolved conflict;
4. the file is not open;
5. the effective availability mode permits dehydration;
6. the item is not explicitly pinned by Always keep on this device.

If any of these conditions fail, the local bytes remain resident.

This rule is intentionally stricter than simple cache eviction because the local copy may contain the only version of the user's work.

## Disk pressure

Automatic mode can reclaim verified remote content when local storage drops below the configured reserve.

The default reserve is the larger of:

- 10 GB free; or
- 10 percent of the local volume.

These are per-device defaults and can be made configurable in the desktop Settings UI.

Automatic cleanup should prefer older, larger, non-pinned resident files before recently used content.

Automatic cleanup must never select:

- files with pending local work;
- conflict copies not yet preserved remotely;
- open files;
- files whose remote version is not verified;
- Always keep content.

## Placeholder states

The provider-neutral local state vocabulary is:

- `resident` — full file bytes are present locally;
- `placeholder` — metadata/name are visible, bytes must be hydrated before normal access;
- `unavailable` — content cannot currently be accessed locally, for example because a required provider is offline and no bytes are resident.

The storage provider does not own these states. They describe this device's local representation.

## Opening a placeholder

When the operating system/user requests access to a placeholder:

1. ClusterStor resolves the item identity and provider;
2. the provider worker verifies the item/version;
3. the file is hydrated to local storage;
4. the local state becomes resident;
5. the application is allowed to continue opening the file.

If the device is offline and the bytes are not resident, ClusterStor surfaces an offline/unavailable error. It does not create an empty or stale file that could overwrite the remote version later.

## Editing hydrated content

Once a placeholder is hydrated and an application modifies it, the normal filesystem watcher applies.

The edit becomes a durable local journal operation and the content is automatically protected from dehydration until the operation has successfully synchronized and the resulting remote version is verified.

## Rename and delete without hydration

Future placeholder support should not download file bytes merely to rename, move, or delete an item.

Metadata-only operations can be journaled and executed by provider identity, subject to the normal conflict checks.

## Offline behavior

Resident files continue to work normally while offline.

Always keep content is intended to remain usable offline.

Online-only placeholders that have not been hydrated are unavailable while offline.

Local changes made while offline remain durable in the sync journal and are conflict-checked against current remote metadata when connectivity returns.

## Multi-device behavior

Availability is per-device.

A file can therefore be:

- Always keep on a desktop;
- Automatic on a laptop;
- Online-only on another computer.

Changing local availability does not change the provider copy and does not force other devices to make the same caching decision.

Conflict/version semantics remain shared across devices.

## Windows implementation boundary

The current developer agent uses a private backing directory plus a SUBST mapped drive.

This is sufficient for normal resident-file synchronization but cannot provide production-quality Files On-Demand placeholders.

The production Windows placeholder implementation should use the Windows Cloud Files API (CFAPI) or an equivalent supported Windows virtualization mechanism.

That future layer must implement the provider-neutral actions already defined by ClusterStor:

- hydrate;
- dehydrate;
- keep local;
- hide due to selective sync;
- expose availability state.

The Cloud Files layer must not contain provider-specific Google/OneDrive/Dropbox/Box conflict or routing logic.

## macOS boundary

The future macOS agent should map the same provider-neutral policy onto Apple's supported file-provider/filesystem integration rather than invent a separate availability model.

The same Automatic, Always keep, Online-only, and selective-sync semantics should apply.

## Current implementation status

Implemented now:

- provider-neutral availability modes;
- provider-neutral resident/placeholder/unavailable states;
- per-device default availability;
- inherited per-path availability/selective-sync rules;
- 10 GB / 10 percent default free-space reserve policy;
- safety decision engine for hydrate/dehydrate/keep/hide actions;
- local availability metadata fields in the sync journal;
- tests preventing dehydration of pending, conflicted, unverified, or open content.

Not yet implemented:

- Windows Cloud Files placeholders;
- hydration/download execution;
- physical dehydration of resident files;
- filesystem context-menu commands such as Always keep and Free up space;
- automatic LRU/size-based eviction ordering;
- Settings API/UI persistence beyond the local agent configuration;
- macOS File Provider integration.

Those execution pieces follow the Google/provider sync worker because ClusterStor must be able to verify and retrieve remote content before it can safely remove local bytes.

# Provider synchronization strategy

ClusterStor V1 must not scan a user's entire connected cloud provider to render My Files.

## Managed-root rule

Each connected provider has a dedicated ClusterStor root. User-facing file operations are scoped to that managed root and its descendants.

## My Files reads

- `GET /api/v1/files` is the provider-neutral file listing endpoint.
- The current provider adapter resolves the requested ClusterStor folder to its provider item ID.
- Only the direct children of that managed folder are queried from the provider.
- Results are cursor-paginated and cached in the web client by folder.
- Expanding or opening a folder loads that folder on demand.
- A ZIP download recursively loads only the selected subtree before building the archive.
- The legacy Google Drive-wide listing endpoint remains for compatibility but must not be used by My Files.

## Incremental provider changes

Google Drive uses the Changes API with a persisted `provider_accounts.change_cursor`.

- The first incremental sync stores Google's start page token.
- Subsequent syncs process only changes since the saved cursor.
- Changed files are indexed only when they are within the managed ClusterStor tree.
- Items moved outside the managed tree become unavailable to ClusterStor without being shown as Trash.
- Provider-trashed items are marked deleted.
- Dashboard statistics traverse only active nodes under managed roots.

This complements, rather than replaces, on-demand folder reads. Folder reads remain authoritative for the folder currently visible to the user.

## ClusterStor realtime

ClusterStor-originated mutations continue to emit monotonic account events. WebSocket notifications refresh only the managed root and current folder instead of reloading an entire provider.

## Upload capacity

Before starting a batch upload, the client calls the provider-neutral upload-capacity preflight endpoint with the combined selected byte count. Google Drive quota is refreshed from the provider and the batch is rejected before upload sessions are created when insufficient space is available. Individual upload-session creation also performs a server-side capacity guard.

## Future providers

OneDrive, Dropbox, Box, and ClusterStor storage should implement the same logical contract:

1. managed root
2. direct-child cursor listing
3. provider change cursor or equivalent delta feed
4. provider/source identity on every item
5. quota/capacity preflight when the provider exposes capacity

Do not reintroduce whole-account scans as a shortcut for new adapters.

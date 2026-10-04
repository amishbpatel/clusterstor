# Provider root-folder policy

Status: approved architecture decision as of 2026-10-04.

## Rule

When ClusterStor creates files or folders inside a connected third-party cloud provider, all ClusterStor-managed content must live underneath a dedicated top-level folder named `ClusterStor`.

Examples:

- Google Drive: `My Drive / ClusterStor / ...`
- OneDrive: `My files / ClusterStor / ...`
- Dropbox: `Dropbox / ClusterStor / ...`
- Box: `All Files / ClusterStor / ...`

## Identity

ClusterStor must identify the provider root folder by the provider's immutable item/folder ID, not by name alone. The display name may be changed by a user without breaking ClusterStor's reference to the folder.

## Existing provider content

Files that already exist elsewhere in a connected provider account are not automatically moved into the ClusterStor folder. Provider discovery/indexing may still read existing files when explicitly designed to do so, but ClusterStor-created content must stay inside the dedicated root.

## Database impact

The existing logical-node and `provider_items` mapping architecture remains valid. If a provider root identifier needs to be stored explicitly, that should be added as a small additive metadata field or related record rather than redesigning the node model.

## Upload rule

Before the first ClusterStor-managed upload or folder creation for a provider account:

1. Look up the stored provider root ID.
2. If it is missing, locate or create the top-level `ClusterStor` folder.
3. Persist that provider folder ID.
4. Place all subsequent ClusterStor-created files/folders beneath that root.

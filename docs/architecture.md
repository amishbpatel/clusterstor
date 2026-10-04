# ClusterStor architecture baseline

## Core database
ClusterStor V1 uses one managed PostgreSQL database for users, logical files, versions, provider accounts, billing, entitlements, devices, events, jobs, and core metadata.

## Future Peer database
When Peer Storage becomes a production subsystem, high-volume Peer operational state moves to a separate PostgreSQL database/cluster. Core keeps logical ownership, plans/entitlements, purchased/earned Peer capacity, and high-level Peer object/reference IDs. Peer owns peer nodes, contributed capacity, fragment inventory/placements, health/reliability scores, repair jobs, transfer telemetry, and other high-write operational state.

The two systems communicate through stable UUID identifiers and service/API/event contracts rather than cross-database foreign keys.

## Desktop Peer onboarding
Desktop-agent setup explicitly asks the user to opt in or opt out of Peer contribution. If opted in, the user chooses a contribution amount. The setting remains editable later in website settings and desktop tray/menu settings.

## Automatic updates
The desktop agent is designed for signed automatic updates with safe-point installation, post-update health checks, and rollback after repeated failed starts. Peer-capable code may be shipped behind feature flags before Peer is enabled.

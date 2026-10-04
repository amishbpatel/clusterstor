# ClusterStor Pricing

Status: approved product pricing as of 2026-10-04.

## Peer Storage

| Plan | Included storage | Monthly price |
| --- | ---: | ---: |
| Peer 100 | 100 GB | $2.99 |
| Peer 500 | 500 GB | $5.99 |
| Peer 1TB | 1 TB | $8.99 |
| Peer 2TB | 2 TB | $15.99 |

Peer Storage is opt-in. Production Peer Storage remains a later subsystem and must not be enabled merely because pricing exists.

## ClusterStor Cloud

| Plan | Included storage | Monthly price |
| --- | ---: | ---: |
| Cloud 500 | 500 GB | $8.99 |
| Cloud 1TB | 1 TB | $14.99 |
| Cloud 2TB | 2 TB | $24.99 |

ClusterStor Cloud is the premium managed-storage option. Backblaze B2 remains the selected future object-storage backend, but B2 credentials, buckets, domains, and production Cloud storage are intentionally deferred until the ClusterStor domain is registered and the Cloud phase begins.

## Pricing implementation notes

- Prices above are customer-facing recurring monthly prices in USD.
- Billing product/price identifiers should remain configuration data rather than hard-coded Stripe IDs.
- Storage entitlements should be expressed in bytes in the backend.
- Provider-connected storage (Google Drive, OneDrive, Dropbox, Box) is separate from purchased ClusterStor Cloud or Peer Storage capacity.

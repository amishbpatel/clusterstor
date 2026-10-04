# ClusterStor contributor guidance

- Preserve the approved architecture unless a change is explicitly documented.
- Prefer a modular Go monolith for V1.
- Keep REST/JSON APIs under /api/v1.
- Do not proxy large customer file bodies through the backend unless necessary.
- Treat the Go backend as authoritative for routing, entitlements, billing, and logical metadata.
- Never log OAuth tokens, device secrets, encryption keys, or file plaintext.
- ClusterStor-managed Cloud and future Peer bytes are client-side encrypted.
- Use cursor-based pagination and monotonic change sequences.
- Model file identity by UUID, not mutable path.
- Peer contribution is opt-in.
- Peer production operations belong in a separate future database/service boundary.

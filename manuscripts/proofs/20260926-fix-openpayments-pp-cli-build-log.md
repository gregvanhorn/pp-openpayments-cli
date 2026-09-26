Manifest transcendence rows: 16 planned, 0 built. Phase 3 will not pass until all 16 ship.

## Phase 2 notes
- Generated from read-only overlay (9 GET endpoints; harvest/imports/revisions/download/write methods removed; basic_auth scheme dropped; servers set).
- Hand-edit internal/client/client.go default User-Agent: CMS Akamai returns 403 for any UA containing 'cli' (verified: curl -A openpayments-pp-cli/v1 → 403, openpayments-pp/1.0 → 200). OPENPAYMENTS_USER_AGENT override preserved.
- Wrapper commands verified live: datastore datasetindex-query-get, datastore sql, catalog search, metastore get-all.

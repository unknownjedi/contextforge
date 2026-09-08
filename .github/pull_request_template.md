## Summary of Changes
Provide a brief summary of what this PR introduces or fixes.

## Architectural Considerations
- [ ] Preserves strict project-scoped data isolation (enforcing `WHERE project_id = $1` in vector queries).
- [ ] Follows API-first principles with OpenAPI updates in `docs/api/openapi.yaml`.
- [ ] Stored secrets and tokens remain encrypted at rest (AES-256-GCM).
- [ ] No fake or mock implementations in production paths.

## Testing Performed
- [ ] `make test-unit` passed.
- [ ] `make test-integration` passed.
- [ ] `make test-isolation` passed (zero cross-tenant data leakage).
- [ ] `make lint` passed cleanly.
- [ ] `make security` passed cleanly.

## Associated ADR
- [ ] An ADR has been created or updated in `docs/adr/` (if architectural changes occurred).

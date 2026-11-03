# Versioning and configuration control

- Specification uses SemVer `MAJOR.MINOR.PATCH`. MAJOR changes incompatible recovery or security obligations; MINOR adds requirements or API operations; PATCH clarifies without changing behaviour.
- API uses `/api/v1`; breaking changes require `/api/v2`. Recovery manifest `format_version` is independently versioned.
- IDs are immutable. Removed requirements become `retired`, never reused.
- Every PR requires: change summary, impacted IDs, threat assessment, migration effect, test updates, release impact, and approval by product and technical owners.
- Tag accepted baselines `spec/vX.Y.Z`; record commit SHA, reviewers, date and outstanding deviations in release record.
- No changes to crypto, manifest or restore ordering without compatibility fixtures and rollback documentation.
- Status of this package: baseline candidate. No actual Git repository/tag has been created.

# Contributing

Use feature branches and pull requests against `main`. Reference requirement IDs in PR descriptions. Update documentation, API contracts, migrations and tests together with behaviour changes. Architectural changes require an ADR. Do not merge failing checks or bypass security/recovery release gates.

Never upload real backup archives, credentials, service-role keys, customer data or secrets. Use synthetic fixtures. Include threat-model implications for changes to authentication, cloud storage, encryption or restore behaviour.

## PR checklist
- [ ] Linked requirements and acceptance criteria
- [ ] Tests added or updated, including failure paths
- [ ] Security and privacy impact assessed
- [ ] Documentation and changelog updated
- [ ] Recovery compatibility assessed

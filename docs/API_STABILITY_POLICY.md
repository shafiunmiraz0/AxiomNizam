# API Stability Policy

> **"We do not break userspace. Period."** — Linus Torvalds

This document defines AxiomNizam's absolute commitment to API stability. Inspired by the Linux kernel's most sacred rule, this policy ensures that any client that works with a given version of AxiomNizam will continue to work with all future versions.

## Core Principle

**No existing API endpoint, response format, or behavior may be broken by any change. Ever.**

This is not a guideline. This is not a recommendation. This is an absolute rule that overrides all other considerations including performance, code cleanliness, and developer convenience.

## What Constitutes a Breaking Change

A **breaking change** is any modification that could cause an existing client to fail. The following are explicitly breaking:

### Route-Level Breaking Changes
- Removing an endpoint (DELETE `/api/v1/foo` no longer responds)
- Changing an endpoint's HTTP method (PUT becomes POST)
- Changing an endpoint's URL path (`/api/v1/users` → `/api/v1/accounts`)
- Adding required query parameters to an existing endpoint
- Changing the authentication/authorization requirements (public → authenticated)

### Response-Level Breaking Changes
- Removing a field from a JSON response
- Changing a field's type (string → number, object → array)
- Changing a field's name (`user_id` → `userId`)
- Changing the response envelope structure
- Changing HTTP status codes for existing conditions (200 → 201 for creation)
- Changing error response format

### Behavior-Level Breaking Changes
- Changing the semantics of an existing operation
- Adding validation that rejects previously accepted input
- Changing default values for optional parameters
- Changing rate limiting behavior (tighter limits)
- Changing pagination behavior (page size, cursor format)

## What Is Always Safe (Additive-Only Changes)

The following changes are **always safe** and do not require versioning:

- Adding a new endpoint
- Adding an optional field to a response
- Adding an optional query parameter
- Adding a new HTTP method to an existing resource
- Adding a new error code (as long as existing codes remain)
- Adding new response headers
- Increasing rate limits (more permissive)
- Fixing a bug that restores intended behavior

## Deprecation Process

When an endpoint or feature must be replaced:

1. **Announce** — Add deprecation notice in response headers (`Deprecation: true`, `Sunset: <date>`)
2. **Wait** — Minimum 90 days between deprecation announcement and sunset
3. **Redirect** — Keep old endpoint as an alias that forwards to the new one
4. **Never Remove** — Old endpoints are kept indefinitely as aliases

### Deprecation Headers

All deprecated endpoints MUST include these response headers:

```
Deprecation: true
Sunset: 2027-03-15T00:00:00Z
Link: </api/v2/users>; rel="successor-version"
```

### Sunset Enforcement

After the sunset date, deprecated endpoints return `410 Gone` with a response body explaining the migration path. They are never removed.

## Versioning Strategy

- **URL path versioning**: `/api/v1/`, `/api/v2/`, etc.
- **Additive only**: New versions add features; old versions remain functional
- **No version negotiation**: Clients target a specific version in the URL
- **Version lifecycle**: active → deprecated → sunset (but never removed)

## Breaking Change Detection

The `internal/platform/apistability/` module provides automated breaking change detection:

- `CaptureSnapshot()` — captures the current API surface (all routes, methods, parameters)
- `CompareSnapshots()` — compares two snapshots to detect breaking changes
- Integrated into CI/CD pipeline — PRs that introduce breaking changes are blocked

## Error Response Contract

All API errors MUST use the standard format from `internal/errors/`:

```json
{
  "error": "human-readable message",
  "code": "MACHINE_READABLE_CODE",
  "detail": "additional context"
}
```

HTTP status codes MUST be consistent:
- `400` — Bad Request (validation errors)
- `401` — Unauthorized (missing/invalid auth)
- `403` — Forbidden (insufficient permissions)
- `404` — Not Found
- `409` — Conflict
- `429` — Rate Limited
- `500` — Internal Server Error

## Enforcement

This policy is enforced through:

1. **Code review** — All PRs are reviewed for breaking changes
2. **Conformance tests** — Automated tests verify API stability invariants
3. **API surface snapshots** — Automated comparison of route registrations
4. **CI/CD pipeline** — Automated build, test, and lint gates
5. **This document** — The source of truth for what is and isn't allowed

## References

- Linux kernel's "don't break userspace" rule (Linus Torvalds, multiple LKML posts)
- `internal/versioning/manager.go` — Deprecation framework
- `internal/contracts/validator.go` — Data schema breaking change detection
- `internal/errors/` — Standard error types and HTTP mapping
- `internal/platform/apistability/` — API surface snapshot and comparison
- `docs/CODING_PRACTICES.md` — Coding standards and review checklist

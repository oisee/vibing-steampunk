# ADR-006: Transport Organizer Tree — Ask for Status and Type Explicitly

**Date:** 2026-09-08
**Status:** ACCEPTED
**Context:** `GetUserTransports` / `ListTransports` / `vsp transport list` on systems where the organizer tree answered with released requests only

---

## Summary

vsp reads transport listings from `GET /sap/bc/adt/cts/transportrequests` and now always sends
`requestType` and `requestStatus`, defaulting to `KWT` and `DR`. A `source` parameter selects the
organizer tree with explicit parameters, the organizer tree through the saved Transport Organizer
search configuration (what Eclipse ADT does), or the E070/E07T tables; `auto` tries them in that
order and reports which one answered.

## Problem

On an ABAP 7.57 system the listing showed one released request and none of the user's modifiable
ones, while Eclipse showed all of them. The old E070/E07T fallback had masked this: the previous
parser found no request in a tree without a `modifiable` bucket, fell back to SQL, and the SQL
listed the modifiable ones. The generic tree parser (#111, #140) finds the released request, skips
the fallback, and exposes the gap.

## Findings (backend, verified on 7.57)

`CL_CTS_ADT_RES_APP` registers the collection, `CL_CTS_ADT_TM_RES_COLL_CONT` answers `GET`.
ADT discovery advertises only `{?targets}`, but the handler reads:

- `requestType` — letters of K (workbench), W (customizing), T (transport of copies)
- `requestStatus` — letters of D (modifiable), R (released)
- `user` (default sy-uname), `targetuser`, `targets`, `releasedFromDate`, `releasedToDate` (YYYYMMDD)
- deprecated `req_cat`, `status`

or, when `configUri` is present, everything from the saved search configuration
(`/searchconfiguration/configurations/<id>`, properties WorkbenchRequests, CustomizingRequests,
TransportOfCopies, Modifiable, Released, User, DateFilter, FromDate, ToDate); `user` is then ignored.

Without `requestStatus` the handler assembles the selection criteria for modifiable requests but
processes the hits only inside a block that requires a `D` in the status list. The block is skipped,
and only released requests of the last 14 days come back. Sending `requestType=KWT&requestStatus=DR`
returns the full tree for any user.

## Decision

1. `TransportQuery` in `pkg/adt` carries user, request types, statuses, released window, targets,
   source and config URI. `withDefaults` fills `KWT` / `DR`; both tools and the CLI use it.
2. `source`: `auto` (default) → `params` → `config` → `sql`, first source with requests wins.
   Fallbacks and deviations are returned as `notes`; the source used is part of every answer.
3. `config` picks the configuration whose `User` property matches the requested user. When none
   matches, the first configuration is used and the answer says whose it is — the listing then
   follows that configuration, not the user asked for.
4. `ListTransports` lists modifiable and released rows by default (`request_status=D` restores the
   old modifiable-only behaviour). Rows carry `bucket` and `project`.
5. The SQL fallback filters by the requested types and statuses and reads E07T in the session
   language instead of English only.
6. The endpoint contract — parameters, the pitfall, the sibling resources — is documented in
   `pkg/adt/transport_query.go`, the tool descriptions, `MCP_USAGE.md`, `README_TOOLS.md` and the
   CLI help.

## Consequences

- One request answers the common case; the configuration round trip happens only on demand or as
  a fallback.
- Consumers of `ListTransports` see released rows too; anything that picks a transport to write
  into must filter on `bucket == modifiable` (or ask with `request_status=D`).
- The user-defaulting behaviour of the backend (sy-uname) is kept: a connection without a
  configured user name still gets its own transports from the tree.

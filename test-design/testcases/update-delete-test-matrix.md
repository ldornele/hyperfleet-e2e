# Update and Delete Lifecycle -- Test Matrix

Consolidated test matrix covering positive, negative, and edge case scenarios for the Update (PATCH) and Delete lifecycle across cluster and nodepool resources. Out of scope: creation lifecycle, RBAC (no implementation exists), performance/load testing.

**Key concepts:** [Cluster lifecycle](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/components/api-service/), [Sentinel](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/components/sentinel/), [Adapter framework](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/components/adapter/), [Hard-delete design](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/components/api-service/hard-delete-design.md).

**Notation:** Shorthand `Reconciled=True` means `Reconciled` condition `status: "True"`. Same convention applies to other conditions (`Applied`, `Finalized`, `Available`, `Health`).

**Design assumptions exercised by this matrix:**

- **Sentinel event publishing:** Every update and delete test relies on Sentinel publishing events on `Reconciled` condition transitions. If Sentinel did not watch this condition, adapters would never reconcile to new generations (#1, #3, #6, #7, #15, #16 would fail).
- **Adapter deletion mode:** Every delete test exercises the adapter's deletion mode switch (triggered by the `lifecycle.delete.when` CEL expression detecting `deleted_time`). If adapters did not switch to deletion mode, they would apply spec instead of finalizing, and `Finalized=True` would never be reported.
- **Adapter delete ordering:** The `lifecycle.delete.when` CEL expression ordering is exercised by delete happy-path tests (#1, #3). Incorrect ordering would result in stuck or failed finalization.
- **Hard-delete mechanism:** Hard-delete executes inline within the `POST /adapter_statuses` request that computes `Reconciled=True` — no separate endpoint or background process. See [hard-delete-design.md](https://github.com/openshift-hyperfleet/architecture/blob/main/hyperfleet/components/api-service/hard-delete-design.md).

## Test Matrix

| # | Test Case | Resource | Pos/Neg | Priority | File | Ticket Area |
|---|-----------|----------|---------|----------|------|-------------|
| 1 | Cluster deletion happy path -- soft-delete through hard-delete | Cluster | Positive | Tier0 | [delete-cluster.md](delete-cluster.md#test-title-cluster-deletion-happy-path----soft-delete-through-hard-delete) | DELETE happy path |
| 2 | Cluster deletion cascades to child nodepools | Cluster + Nodepool | Positive | Tier0 | [delete-cluster.md](delete-cluster.md#test-title-cluster-deletion-cascades-to-child-nodepools) | DELETE hierarchical |
| 3 | Nodepool deletion happy path -- soft-delete through hard-delete | Nodepool | Positive | Tier0 | [delete-nodepool.md](delete-nodepool.md#test-title-nodepool-deletion-happy-path----soft-delete-through-hard-delete) | DELETE happy path |
| 4 | PATCH to soft-deleted cluster returns 409 Conflict | Cluster | Negative | Tier1 | [delete-cluster.md](delete-cluster.md#test-title-patch-to-soft-deleted-cluster-returns-409-conflict) | DELETE API behavior |
| 5 | PATCH to soft-deleted nodepool returns 409 Conflict | Nodepool | Negative | Tier1 | [delete-nodepool.md](delete-nodepool.md#test-title-patch-to-soft-deleted-nodepool-returns-409-conflict) | DELETE API behavior |
| 6 | Cluster update via PATCH triggers reconciliation and reaches Reconciled | Cluster | Positive | Tier0 | [update-cluster.md](update-cluster.md#test-title-cluster-update-via-patch-triggers-reconciliation-and-reaches-reconciled) | UPDATE happy path |
| 7 | Nodepool update via PATCH triggers reconciliation and reaches Reconciled | Nodepool | Positive | Tier0 | [update-nodepool.md](update-nodepool.md#test-title-nodepool-update-via-patch-triggers-reconciliation-and-reaches-reconciled) | UPDATE happy path |
| 8 | Soft-deleted cluster remains visible via GET and LIST | Cluster | Positive | Tier1 | [delete-cluster.md](delete-cluster.md#test-title-soft-deleted-cluster-remains-visible-via-get-and-list) | DELETE API behavior |
| 9 | Re-DELETE on already-deleted cluster is idempotent | Cluster | Positive | Tier1 | [delete-cluster.md](delete-cluster.md#test-title-re-delete-on-already-deleted-cluster-is-idempotent) | DELETE edge cases |
| 10 | Create nodepool under soft-deleted cluster returns 409 Conflict | Cluster | Negative | Tier1 | [delete-cluster.md](delete-cluster.md#test-title-create-nodepool-under-soft-deleted-cluster-returns-409-conflict) | DELETE API behavior |
| 11 | DELETE non-existent cluster returns 404 | Cluster | Negative | Tier1 | [delete-cluster.md](delete-cluster.md#test-title-delete-non-existent-cluster-returns-404) | DELETE edge cases |
| 12 | Nodepool deletion does not affect sibling nodepools | Nodepool | Positive | Tier1 | [delete-nodepool.md](delete-nodepool.md#test-title-nodepool-deletion-does-not-affect-sibling-nodepools) | DELETE hierarchical |
| 13 | Re-DELETE on already-deleted nodepool is idempotent | Nodepool | Positive | Tier1 | [delete-nodepool.md](delete-nodepool.md#test-title-re-delete-on-already-deleted-nodepool-is-idempotent) | DELETE edge cases |
| 14 | DELETE non-existent nodepool returns 404 | Nodepool | Negative | Tier1 | [delete-nodepool.md](delete-nodepool.md#test-title-delete-non-existent-nodepool-returns-404) | DELETE edge cases |
| 15 | Adapter statuses transition during update reconciliation | Cluster | Positive | Tier1 | [update-cluster.md](update-cluster.md#test-title-adapter-statuses-transition-during-update-reconciliation) | UPDATE happy path |
| 16 | Multiple rapid updates coalesce to latest generation | Cluster | Positive | Tier1 | [update-cluster.md](update-cluster.md#test-title-multiple-rapid-updates-coalesce-to-latest-generation) | UPDATE edge cases |
| 17 | Stuck deletion -- adapter unable to finalize prevents hard-delete | Cluster | Negative | Tier2 | [delete-cluster.md](delete-cluster.md#test-title-stuck-deletion----adapter-unable-to-finalize-prevents-hard-delete) | DELETE error cases |
| 19 | Simultaneous DELETE requests produce a single soft-delete record | Cluster | Positive | Tier1 | [delete-cluster.md](delete-cluster.md#test-title-simultaneous-delete-requests-produce-a-single-soft-delete-record) | DELETE edge cases |
| 20 | Adapter treats externally-deleted K8s resources as finalized | Cluster | Positive | Tier1 | [delete-cluster.md](delete-cluster.md#test-title-adapter-treats-externally-deleted-k8s-resources-as-finalized) | DELETE edge cases |
| 21 | DELETE during update reconciliation before adapters converge | Cluster | Positive | Tier1 | [delete-cluster.md](delete-cluster.md#test-title-delete-during-update-reconciliation-before-adapters-converge) | DELETE edge cases |
| 22 | Recreate cluster with same name after hard-delete | Cluster | Positive | Tier1 | [delete-cluster.md](delete-cluster.md#test-title-recreate-cluster-with-same-name-after-hard-delete) | DELETE edge cases |
| 23 | Labels-only PATCH bumps generation and triggers reconciliation (cluster) | Cluster | Positive | Tier1 | [update-cluster.md](update-cluster.md#test-title-labels-only-patch-bumps-generation-and-triggers-reconciliation) | UPDATE edge cases |
| 24 | Labels-only PATCH bumps generation and triggers reconciliation (nodepool) | Nodepool | Positive | Tier1 | [update-nodepool.md](update-nodepool.md#test-title-labels-only-patch-bumps-generation-and-triggers-reconciliation) | UPDATE edge cases |
| 25 | LIST returns soft-deleted clusters alongside active clusters | Cluster | Positive | Tier1 | [delete-cluster.md](delete-cluster.md#test-title-list-returns-soft-deleted-clusters-alongside-active-clusters) | DELETE API behavior |
| 28 | Soft-deleted nodepool remains visible via GET and LIST | Nodepool | Positive | Tier1 | [delete-nodepool.md](delete-nodepool.md#test-title-soft-deleted-nodepool-remains-visible-via-get-and-list) | DELETE API behavior |
| 29 | No-op PATCH does not increment generation | Cluster | Positive | Tier1 | [update-cluster.md](update-cluster.md#test-title-no-op-patch-does-not-increment-generation) | UPDATE edge cases |

## Summary

| Category | Tier0 | Tier1 | Tier2 | Total |
|----------|-------|-------|-------|-------|
| Positive | 5 | 15 | 0 | 20 |
| Negative | 0 | 5 | 1 | 6 |
| **Total** | **5** | **20** | **1** | **26** |

## Coverage by Ticket Area

| Ticket Area | Test Cases | Status |
|-------------|-----------|--------|
| DELETE happy path (soft-delete -> Finalized -> Reconciled -> hard-delete) | #1, #3 | Covered |
| DELETE hierarchical (subresource cleanup before parent hard-delete) | #2, #12 | Covered |
| DELETE edge cases (idempotent re-DELETE, concurrent DELETEs, non-existent resource, NotFound-as-success, DELETE during update, name reuse after hard-delete) | #9, #11, #13, #14, #19, #20, #21, #22 | Covered |
| DELETE error cases (stuck adapter, unable to finalize) | #17 | Covered |
| DELETE API behavior (409 on mutations, GET/LIST still allowed) | #4, #5, #8, #10, #25, #28 | Covered |
| UPDATE happy path (PATCH -> generation -> reconciliation -> Reconciled) | #6, #7, #15 | Covered |
| UPDATE edge cases (rapid updates, coalescing, labels-only PATCH, no-op PATCH) | #16, #23, #24, #29 | Covered |
| UPDATE negative (E2E-scoped) | — | Not applicable in E2E scope (API payload validation belongs in integration tests) |

## Deferred / Not Applicable

Items considered for this matrix but deliberately not covered as standalone test cases:

| Item | Status | Rationale |
|------|--------|-----------|
| RBAC denied on DELETE | N/A | No RBAC implementation exists in the API. Authentication is bearer-token only with no role/permission model. Revisit when RBAC is added. |
| Concurrent PATCH + DELETE race condition | Deferred | Non-deterministic test — both outcomes (PATCH-first or DELETE-first) are acceptable. Hard to assert on reliably in E2E. Revisit if the team adds a deterministic ordering guarantee. |
| PATCH payload validation errors (malformed JSON, schema/type violations) | Out of E2E scope | API-boundary validation happens before lifecycle business logic; cover in API integration tests rather than cross-component E2E. |
| Cascade DELETE while child nodepool is already deleting (deleted_time preservation) | Out of E2E scope | `deleted_time` preservation is a service-layer invariant (`if np.DeletedTime == nil` guard in `CascadeSoftDelete`). Already covered by unit test (`cluster_test.go:1472`), nodepool idempotency test (`node_pool_test.go:941`), and integration test (`clusters_test.go:1077`). E2E assertion is structurally unable to verify itself due to 404 race — adapter may hard-delete the nodepool before the first poll. |
| Cascade DELETE while child nodepool is mid-update-reconciliation (generation handoff) | Out of E2E scope | Adapter generation handling is trivial — precondition phase always re-fetches the resource from the API (`GET /clusters/{id}`) and overwrites the event's `generationId` with the latest value. `CompareGenerations()` is a simple three-way branch (not exists → create, equal → skip, different → update) with no special handling for generation gaps. The adapter mechanically processes at the correct generation regardless of concurrent PATCH + cascade DELETE. Same 404 race weakness as the already-deleting cascade test — E2E assertion structurally unable to verify itself. |
| DELETE during initial creation before cluster reaches Reconciled | Out of E2E scope | Test acknowledges edge case is best-effort (warning when all adapters already Applied=True). Adapter behavior is mechanical — re-fetch always gets latest state including `deleted_time`, no branching on prior Applied state. Same adapter code path covered by Tier1 test #21 (DELETE during update reconciliation) on a more deterministic scenario. |

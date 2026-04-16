# Update and Delete Lifecycle -- Test Matrix

Consolidated test matrix for HYPERFLEET-859. Covers positive, negative, and edge case scenarios for the Update (PATCH) and Delete lifecycle across cluster and nodepool resources. 

## Test Matrix

| # | Test Case | Resource | Pos/Neg | Priority | File | Ticket Area |
|---|-----------|----------|---------|----------|------|-------------|
| 1 | Cluster deletion happy path -- soft-delete through hard-delete | Cluster | Positive | Tier0 | [delete-cluster.md](delete-cluster.md#test-title-cluster-deletion-happy-path----soft-delete-through-hard-delete) | DELETE happy path |
| 2 | Cluster deletion cascades to child nodepools | Cluster + Nodepool | Positive | Tier0 | [delete-cluster.md](delete-cluster.md#test-title-cluster-deletion-cascades-to-child-nodepools) | DELETE hierarchical |
| 3 | Nodepool deletion happy path -- soft-delete through hard-delete | Nodepool | Positive | Tier0 | [delete-nodepool.md](delete-nodepool.md#test-title-nodepool-deletion-happy-path----soft-delete-through-hard-delete) | DELETE happy path |
| 4 | PATCH to soft-deleted cluster returns 409 Conflict | Cluster | Negative | Tier0 | [delete-cluster.md](delete-cluster.md#test-title-patch-to-soft-deleted-cluster-returns-409-conflict) | DELETE API behavior |
| 5 | PATCH to soft-deleted nodepool returns 409 Conflict | Nodepool | Negative | Tier0 | [delete-nodepool.md](delete-nodepool.md#test-title-patch-to-soft-deleted-nodepool-returns-409-conflict) | DELETE API behavior |
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
| 17 | Stuck deletion -- adapter unable to finalize prevents hard-delete | Cluster | Negative | Tier1 | [delete-cluster.md](delete-cluster.md#test-title-stuck-deletion----adapter-unable-to-finalize-prevents-hard-delete) | DELETE error cases |
| 18 | DELETE during initial creation before cluster reaches Ready | Cluster | Positive | Tier2 | [delete-cluster.md](delete-cluster.md#test-title-delete-during-initial-creation-before-cluster-reaches-ready) | DELETE edge cases |
| 19 | PATCH with invalid payload is rejected without changing cluster state | Cluster | Negative | Tier1 | [update-cluster.md](update-cluster.md#test-title-patch-with-invalid-payload-is-rejected-without-changing-cluster-state) | UPDATE negative |
| 20 | PATCH with invalid payload is rejected without changing nodepool state | Nodepool | Negative | Tier1 | [update-nodepool.md](update-nodepool.md#test-title-patch-with-invalid-payload-is-rejected-without-changing-nodepool-state) | UPDATE negative |
| 21 | Simultaneous DELETE requests produce a single tombstone | Cluster | Positive | Tier1 | [delete-cluster.md](delete-cluster.md#test-title-simultaneous-delete-requests-produce-a-single-tombstone) | DELETE edge cases |
| 22 | Adapter treats externally-deleted K8s resources as finalized | Cluster | Positive | Tier1 | [delete-cluster.md](delete-cluster.md#test-title-adapter-treats-externally-deleted-k8s-resources-as-finalized) | DELETE edge cases |
| 23 | DELETE during update reconciliation before adapters converge | Cluster | Positive | Tier1 | [delete-cluster.md](delete-cluster.md#test-title-delete-during-update-reconciliation-before-adapters-converge) | DELETE edge cases |
| 24 | Recreate cluster with same name after hard-delete | Cluster | Positive | Tier1 | [delete-cluster.md](delete-cluster.md#test-title-recreate-cluster-with-same-name-after-hard-delete) | DELETE edge cases |

## Summary

| Category | Tier0 | Tier1 | Tier2 | Total |
|----------|-------|-------|-------|-------|
| Positive | 5 | 10 | 1 | 16 |
| Negative | 2 | 6 | 0 | 8 |
| **Total** | **7** | **16** | **1** | **24** |

## Coverage by Ticket Area

| Ticket Area | Test Cases | Status |
|-------------|-----------|--------|
| DELETE happy path (tombstone -> Finalized -> Reconciled -> hard-delete) | #1, #3 | Covered |
| DELETE hierarchical (subresource cleanup before parent hard-delete) | #2, #12 | Covered |
| DELETE edge cases (idempotent re-DELETE, concurrent DELETEs, non-existent resource, stale pre-tombstone state, NotFound-as-success, DELETE during update, name reuse after hard-delete) | #9, #11, #13, #14, #18, #21, #22, #23, #24 | Covered |
| DELETE error cases (stuck adapter, unable to finalize) | #17 | Covered |
| DELETE API behavior (409 on mutations, GET/LIST still allowed) | #4, #5, #8, #10 | Covered |
| UPDATE happy path (PATCH -> generation -> reconciliation -> Reconciled) | #6, #7, #15 | Covered |
| UPDATE edge cases (rapid updates, coalescing) | #16 | Covered |
| UPDATE negative (invalid/malformed PATCH payloads) | #19, #20 | Covered |

## Deferred / Not Applicable

Items from HYPERFLEET-859 scope that are not covered as standalone test cases, with rationale:

| Item | Status | Rationale |
|------|--------|-----------|
| RBAC denied on DELETE | N/A | No RBAC implementation exists in the API. Authentication is bearer-token only with no role/permission model. Revisit when RBAC is added. |
| Sentinel: events published using Reconciled (not Ready) | Implicitly covered | Every update and delete test case relies on Sentinel publishing events based on Reconciled condition. If Sentinel used Ready instead, adapters would not reconcile to new generations and tests #1, #3, #6, #7, #15, #16 would fail. No standalone test case needed. |
| Adapter: when_deleting mode switch | Implicitly covered | Every delete test case (#1, #2, #3, #17, #18) exercises the adapter's when_deleting mode. If adapters did not switch to deletion mode, they would apply spec instead of finalizing, and Finalized=True would never be reported. |
| Adapter: delete_options.when ordering | Implicitly covered | The delete happy path tests (#1, #3) validate that adapters process deletion in the correct order by confirming all adapters reach Finalized=True. Incorrect ordering would result in stuck or failed finalization. |
| Adapter: propagationPolicy passed to K8s API | Not covered | Internal adapter behavior. Could be validated by inspecting K8s resource state after deletion (e.g., verifying child resources are cleaned up according to the expected propagation policy). Deferred — consider adding when adapter delete_options configuration is finalized. |

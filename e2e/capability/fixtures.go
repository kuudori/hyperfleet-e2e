package capability

import (
	"context"
	"errors"
	"fmt"
	"net/http"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega" //nolint:staticcheck // dot import for test readability

	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/client"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/config"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/descriptor"
	"github.com/openshift-hyperfleet/hyperfleet-e2e/pkg/helper"
)

// fixtureRows reads the static table, not the resolved catalog, so the spec tree
// is the same in every process and environment. A root's parent is the zero row.
func fixtureRows(keep func(row, parent descriptor.EntityDescriptor) bool) []descriptor.EntityDescriptor {
	table := config.EntityDescriptors()
	byKind := make(map[string]descriptor.EntityDescriptor, len(table))
	for _, row := range table {
		byKind[row.Kind] = row
	}
	var rows []descriptor.EntityDescriptor
	for _, row := range table {
		if row.Fixture && keep(row, byKind[row.ParentKind]) {
			rows = append(rows, row)
		}
	}
	return rows
}

func adapterBacked(row descriptor.EntityDescriptor) bool {
	return len(row.RequiredAdapters) > 0
}

// A child runs only under an adapter-backed root: the specs create the parent as a
// root, and a kind with no required adapters never reaches Reconciled=True.
func adapterBackedRoot(row descriptor.EntityDescriptor) bool {
	return adapterBacked(row) && row.ParentKind == ""
}

func lifecycleKind(row, parent descriptor.EntityDescriptor) bool {
	return adapterBacked(row) && (row.ParentKind == "" || adapterBackedRoot(parent))
}

func cascadeChildKind(row, parent descriptor.EntityDescriptor) bool {
	return adapterBacked(row) && row.OnParentDelete == "cascade" && adapterBackedRoot(parent)
}

func recreateKind(row, _ descriptor.EntityDescriptor) bool {
	return adapterBackedRoot(row)
}

// An empty Describe would pass tier0 with the capability untested.
func failWhenEmpty(capability string, rows []descriptor.EntityDescriptor) {
	if len(rows) > 0 {
		return
	}
	ginkgo.It("has a fixture kind to run on", func() {
		ginkgo.Fail(fmt.Sprintf("no fixture kind in the descriptor table has the shape capability %q needs", capability))
	})
}

func resolve(h *helper.Helper, kind string) descriptor.EntityDescriptor {
	ginkgo.GinkgoHelper()
	entity, err := h.Entities.Lookup(kind)
	Expect(err).NotTo(HaveOccurred())
	return entity
}

func createFixture(
	ctx context.Context,
	h *helper.Helper,
	entity descriptor.EntityDescriptor,
	parentID string,
) *client.Resource {
	ginkgo.GinkgoHelper()
	resource, err := h.Client.CreateResourceFromPayloadWith(
		ctx,
		entity.CollectionPath(parentID),
		h.TestDataPath("payloads/fixtures/fixture-request.json"),
		map[string]any{"kind": entity.Kind},
	)
	Expect(err).NotTo(HaveOccurred(), "failed to create %s", entity.Kind)
	Expect(resource.ID).NotTo(BeNil(), "%s ID should be generated", entity.Kind)
	h.DeferCleanup(entity, parentID, *resource.ID)
	ginkgo.GinkgoWriter.Printf("Created %s ID: %s, Name: %s\n", entity.Kind, *resource.ID, resource.Name)
	return resource
}

// The API hard-deletes a resource in the status write that finalizes its last
// adapter, so Finalized=True may never be observable; a 404 ends the check.
func finalizedOrGone(
	ctx context.Context,
	h *helper.Helper,
	entity descriptor.EntityDescriptor,
	parentID, resourceID string,
) func(g Gomega) {
	path := entity.ResourcePath(parentID, resourceID)
	return func(g Gomega) {
		_, err := h.Client.GetResource(ctx, path)
		if isNotFound(err) {
			return
		}
		g.Expect(err).NotTo(HaveOccurred())
		statuses, err := h.Client.GetResourceStatuses(ctx, path)
		if isNotFound(err) {
			return
		}
		g.Expect(err).NotTo(HaveOccurred())
		g.Expect(statuses).To(helper.HaveAllAdaptersWithCondition(
			entity.RequiredAdapters, client.ConditionTypeFinalized, client.AdapterConditionStatusTrue))
	}
}

func isNotFound(err error) bool {
	var httpErr *client.HTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound
}

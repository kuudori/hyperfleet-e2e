package helper

// RemoteNamespace returns the Namespace the remote-transport adapter creates on the target
// cluster for clusterID. The name comes from the adapter example the infra stack deploys
// (hyperfleet-adapter charts/examples/remote-two-resources, discovery by_name
// "{{ .resourceId }}-remote"). It shares the cluster ID prefix with the local adapters'
// namespaces, so prefix lookups by cluster ID find it too.
func RemoteNamespace(clusterID string) string {
	return clusterID + "-remote"
}

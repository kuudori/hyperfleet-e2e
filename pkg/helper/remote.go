package helper

// RemoteNamespace returns the Namespace the remote-transport adapter creates on the target
// cluster for clusterID. The name comes from the remote adapter task the infra stack deploys
// (hyperfleet-infra helmfile/configs/base/adapters/remote/, discovery by_name
// "{{ .resourceId }}-remote"). It shares the cluster ID prefix with the local adapters'
// namespaces, so prefix lookups by cluster ID find it too.
func RemoteNamespace(clusterID string) string {
	return clusterID + "-remote"
}

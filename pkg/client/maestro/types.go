package maestro

// ResourceBundleList represents the response from Maestro API listing resource bundles
type ResourceBundleList struct {
	Items []ResourceBundle `json:"items"`
}

// ResourceBundle represents a Maestro resource bundle (ManifestWork)
type ResourceBundle struct {
	ID              string           `json:"id"`
	ConsumerName    string           `json:"consumer_name"`
	Version         int              `json:"version"`
	Metadata        Metadata         `json:"metadata"`
	Manifests       []Manifest       `json:"manifests"`
	ManifestConfigs []ManifestConfig `json:"manifest_configs"`
	DeleteOption    *DeleteOption    `json:"delete_option,omitempty"`
}

// DeleteOption represents the deletion options for a resource bundle
type DeleteOption struct {
	PropagationPolicy string `json:"propagationPolicy"`
}

// Metadata represents metadata for a Maestro resource
type Metadata struct {
	Labels      map[string]string `json:"labels"`
	Annotations map[string]string `json:"annotations"`
}

// Manifest represents a manifest within a resource bundle
type Manifest struct {
	Metadata ManifestMetadata `json:"metadata"`
}

// ManifestMetadata represents metadata for a manifest
type ManifestMetadata struct {
	Name string `json:"name"`
}

// ManifestConfig represents the manifest configuration including feedback rules
type ManifestConfig struct {
	ResourceIdentifier ResourceIdentifier `json:"resourceIdentifier"`
	FeedbackRules      []FeedbackRule     `json:"feedbackRules"`
}

// ResourceIdentifier identifies a specific resource in the manifest
type ResourceIdentifier struct {
	Name      string `json:"name"`
	Group     string `json:"group"`
	Resource  string `json:"resource"`
	Namespace string `json:"namespace,omitempty"`
}

// FeedbackRule represents a feedback rule for status collection
type FeedbackRule struct {
	Type      string     `json:"type"`
	JSONPaths []JSONPath `json:"jsonPaths,omitempty"`
}

// JSONPath represents a JSONPath expression for extracting values
type JSONPath struct {
	Name string `json:"name"`
	Path string `json:"path"`
}

// FindManifestConfig finds a manifest config by full resource identity.
// Empty fields in the identifier parameter are treated as wildcards (match any value).
func FindManifestConfig(configs []ManifestConfig, identifier ResourceIdentifier) *ManifestConfig {
	for i := range configs {
		rid := configs[i].ResourceIdentifier
		// Match name (required)
		if identifier.Name != "" && rid.Name != identifier.Name {
			continue
		}
		// Match namespace (optional - empty means any namespace)
		if identifier.Namespace != "" && rid.Namespace != identifier.Namespace {
			continue
		}
		// Match group (optional - empty means any group)
		if identifier.Group != "" && rid.Group != identifier.Group {
			continue
		}
		// Match resource (optional - empty means any resource type)
		if identifier.Resource != "" && rid.Resource != identifier.Resource {
			continue
		}
		return &configs[i]
	}
	return nil
}

// ConsumerList represents the response from Maestro API listing consumers
type ConsumerList struct {
	Items []Consumer `json:"items"`
}

// Consumer represents a Maestro consumer (target cluster)
type Consumer struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

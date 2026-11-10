// Package config holds the Upjet configuration of the Apache Superset provider.
package config

import (
	// The provider schema and metadata are embedded into the binary.
	_ "embed"

	ujconfig "github.com/crossplane/upjet/v2/pkg/config"
	superset "github.com/platacard/terraform-provider-superset/provider"

	"github.com/hayone1/crossplane-provider-superset/internal/version"
)

const (
	resourcePrefix = "superset"
	modulePath     = "github.com/hayone1/crossplane-provider-superset"

	rootGroupCluster    = "superset.crossplane.io"
	rootGroupNamespaced = "superset.m.crossplane.io"
)

//go:embed schema.json
var providerSchema string

//go:embed provider-metadata.yaml
var providerMetadata string

// GetProvider returns the configuration of the cluster-scoped managed resources
// (*.superset.crossplane.io).
func GetProvider() *ujconfig.Provider {
	return buildProvider(rootGroupCluster)
}

// GetProviderNamespaced returns the configuration of the namespaced managed resources
// (*.superset.m.crossplane.io) that Crossplane v2 introduced.
func GetProviderNamespaced() *ujconfig.Provider {
	return buildProvider(rootGroupNamespaced,
		ujconfig.WithExampleManifestConfiguration(ujconfig.ExampleManifestConfiguration{
			ManagedResourceNamespace: "default",
		}))
}

func buildProvider(rootGroup string, opts ...ujconfig.ProviderOption) *ujconfig.Provider {
	opts = append([]ujconfig.ProviderOption{
		ujconfig.WithShortName("superset"),
		ujconfig.WithRootGroup(rootGroup),
		ujconfig.WithFeaturesPackage("internal/features"),
		// Every resource runs through the in-process ("no-fork") Terraform plugin
		// framework provider, so the CLI and plugin SDK include lists stay empty.
		ujconfig.WithIncludeList([]string{}),
		ujconfig.WithTerraformPluginSDKIncludeList([]string{}),
		ujconfig.WithTerraformPluginFrameworkIncludeList(ExternalNameConfigured()),
		ujconfig.WithTerraformPluginFrameworkProvider(superset.New(version.Version)()),
		ujconfig.WithDefaultResourceOptions(ExternalNameConfigurations()),
	}, opts...)

	pc := ujconfig.NewProvider([]byte(providerSchema), resourcePrefix, modulePath, []byte(providerMetadata), opts...)
	for _, configure := range configurators {
		configure(pc)
	}
	pc.ConfigureResources()
	return pc
}

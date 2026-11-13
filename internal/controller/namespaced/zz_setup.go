// SPDX-FileCopyrightText: 2024 The Crossplane Authors <https://crossplane.io>
//
// SPDX-License-Identifier: Apache-2.0

package controller

import (
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane/upjet/v2/pkg/controller"

	annotation "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/annotation/annotation"
	annotationlayer "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/annotation/annotationlayer"
	assetsimport "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/assets/assetsimport"
	chartimport "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/assets/chartimport"
	dashboardimport "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/assets/dashboardimport"
	datasetimport "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/assets/datasetimport"
	chart "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/chart/chart"
	csstemplate "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/dashboard/csstemplate"
	dashboard "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/dashboard/dashboard"
	dashboardembedding "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/dashboard/dashboardembedding"
	database "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/database/database"
	databasepermissionssync "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/database/databasepermissionssync"
	metadatabase "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/database/metadatabase"
	dataset "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/dataset/dataset"
	datasetrefresh "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/dataset/datasetrefresh"
	providerconfig "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/providerconfig"
	report "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/report/report"
	group "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/security/group"
	role "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/security/role"
	rolepermissions "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/security/rolepermissions"
	rowlevelsecurity "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/security/rowlevelsecurity"
	user "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/security/user"
	savedquery "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/sqllab/savedquery"
	sqlexecution "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/sqllab/sqlexecution"
	tag "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/tag/tag"
	theme "github.com/hayone1/crossplane-provider-superset/internal/controller/namespaced/theme/theme"
)

// Setup creates all controllers with the supplied logger and adds them to
// the supplied manager.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		annotation.Setup,
		annotationlayer.Setup,
		assetsimport.Setup,
		chartimport.Setup,
		dashboardimport.Setup,
		datasetimport.Setup,
		chart.Setup,
		csstemplate.Setup,
		dashboard.Setup,
		dashboardembedding.Setup,
		database.Setup,
		databasepermissionssync.Setup,
		metadatabase.Setup,
		dataset.Setup,
		datasetrefresh.Setup,
		providerconfig.Setup,
		report.Setup,
		group.Setup,
		role.Setup,
		rolepermissions.Setup,
		rowlevelsecurity.Setup,
		user.Setup,
		savedquery.Setup,
		sqlexecution.Setup,
		tag.Setup,
		theme.Setup,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}

// SetupGated creates all controllers with the supplied logger and adds them to
// the supplied manager gated.
func SetupGated(mgr ctrl.Manager, o controller.Options) error {
	for _, setup := range []func(ctrl.Manager, controller.Options) error{
		annotation.SetupGated,
		annotationlayer.SetupGated,
		assetsimport.SetupGated,
		chartimport.SetupGated,
		dashboardimport.SetupGated,
		datasetimport.SetupGated,
		chart.SetupGated,
		csstemplate.SetupGated,
		dashboard.SetupGated,
		dashboardembedding.SetupGated,
		database.SetupGated,
		databasepermissionssync.SetupGated,
		metadatabase.SetupGated,
		dataset.SetupGated,
		datasetrefresh.SetupGated,
		providerconfig.SetupGated,
		report.SetupGated,
		group.SetupGated,
		role.SetupGated,
		rolepermissions.SetupGated,
		rowlevelsecurity.SetupGated,
		user.SetupGated,
		savedquery.SetupGated,
		sqlexecution.SetupGated,
		tag.SetupGated,
		theme.SetupGated,
	} {
		if err := setup(mgr, o); err != nil {
			return err
		}
	}
	return nil
}

// SetupWebhookWithManager registers conversion webhooks for all resource kinds in the group.
func SetupWebhookWithManager(mgr ctrl.Manager) error {
	for _, setup := range []func(ctrl.Manager) error{
		annotation.SetupWebhookWithManager,
		annotationlayer.SetupWebhookWithManager,
		assetsimport.SetupWebhookWithManager,
		chartimport.SetupWebhookWithManager,
		dashboardimport.SetupWebhookWithManager,
		datasetimport.SetupWebhookWithManager,
		chart.SetupWebhookWithManager,
		csstemplate.SetupWebhookWithManager,
		dashboard.SetupWebhookWithManager,
		dashboardembedding.SetupWebhookWithManager,
		database.SetupWebhookWithManager,
		databasepermissionssync.SetupWebhookWithManager,
		metadatabase.SetupWebhookWithManager,
		dataset.SetupWebhookWithManager,
		datasetrefresh.SetupWebhookWithManager,
		providerconfig.SetupWebhookWithManager,
		report.SetupWebhookWithManager,
		group.SetupWebhookWithManager,
		role.SetupWebhookWithManager,
		rolepermissions.SetupWebhookWithManager,
		rowlevelsecurity.SetupWebhookWithManager,
		user.SetupWebhookWithManager,
		savedquery.SetupWebhookWithManager,
		sqlexecution.SetupWebhookWithManager,
		tag.SetupWebhookWithManager,
		theme.SetupWebhookWithManager,
	} {
		if err := setup(mgr); err != nil {
			return err
		}
	}
	return nil
}

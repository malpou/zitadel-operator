package controller

import (
	"context"

	permissionv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/internal_permission/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/malpou/zitadel-operator/api/v1alpha1"
)

// Test-only handles on the pure functions.
//
//nolint:gochecknoglobals // test export shims.
var (
	Classify        = classify
	OIDCConfig      = oidcConfig
	OIDCEqual       = oidcEqual
	UpdateOIDC      = updateOIDC
	SameSet         = sameSet
	SyncSet         = syncSet[string]
	FlowIDs         = flowIDs
	CreateTarget    = createTarget
	UpdateTarget    = updateTarget
	RotateRequested = rotateRequested
	WriteClientIDs  = writeClientIDs
	ErrDependency   = errDependencyNotReady
	ErrSecret       = errSecretUnavailable
	Resync          = resync
	DependencyRetry = dependencyRetry
)

// NewProjectReconciler wires the shared reconcile driver to a stub syncer.
func NewProjectReconciler(
	kube client.Client, sync func(context.Context, *v1alpha1.Project) error,
) reconcile.Reconciler {
	return &reconciler[*v1alpha1.Project]{
		kube: kube, newObj: func() *v1alpha1.Project { return &v1alpha1.Project{} },
		impl: stubSyncer(sync), finalize: true,
	}
}

type stubSyncer func(context.Context, *v1alpha1.Project) error

func (s stubSyncer) sync(ctx context.Context, p *v1alpha1.Project) error { return s(ctx, p) }

func (stubSyncer) remove(context.Context, *v1alpha1.Project) error { return nil }

// SyncOrgRoles runs the MachineUser org-role reconcile against a permission client.
func SyncOrgRoles(
	ctx context.Context, perm permissionv2.InternalPermissionServiceClient, orgID, userID string, want []string,
) error {
	s := &machineUserSyncer{z: Clients{OrgID: orgID, Permission: perm}}

	return s.syncOrgRoles(ctx, userID, want)
}

// NewTargetReconciler is the Target controller's reconciler.
func NewTargetReconciler(kube client.Client, z Clients) reconcile.Reconciler {
	return &reconciler[*v1alpha1.Target]{
		kube: kube, newObj: func() *v1alpha1.Target { return &v1alpha1.Target{} },
		impl: &targetSyncer{kube: kube, z: z}, finalize: true,
	}
}

// NewExecutionReconciler is the Execution controller's reconciler.
func NewExecutionReconciler(kube client.Client, z Clients) reconcile.Reconciler {
	return &reconciler[*v1alpha1.Execution]{
		kube: kube, newObj: func() *v1alpha1.Execution { return &v1alpha1.Execution{} },
		impl: &executionSyncer{kube: kube, z: z}, finalize: true,
	}
}

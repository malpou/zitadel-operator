package controller_test

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/malpou/zitadel-operator/api/v1alpha1"
	"github.com/malpou/zitadel-operator/internal/controller"
)

// TestReconcileStaleCache replays the first-apply race: the reconcile queued
// by our own finalizer write reads a cached copy whose resourceVersion is
// already behind the server. The status write must still land.
func TestReconcileStaleCache(t *testing.T) {
	t.Parallel()
	scheme := runtime.NewScheme()
	if err := v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	key := types.NamespacedName{Namespace: "ns", Name: "p"}
	var stale *v1alpha1.Project
	kube := fake.NewClientBuilder().WithScheme(scheme).
		WithObjects(&v1alpha1.Project{Namespace: "ns", Name: "p"}).
		WithStatusSubresource(&v1alpha1.Project{}).
		WithInterceptorFuncs(interceptor.Funcs{
			Get: func(ctx context.Context, c client.WithWatch, k client.ObjectKey, obj client.Object,
				opts ...client.GetOption,
			) error {
				if stale != nil {
					stale.DeepCopyInto(obj.(*v1alpha1.Project))

					return nil
				}

				return c.Get(ctx, k, obj, opts...)
			},
		}).Build()

	syncs := 0
	r := controller.NewProjectReconciler(kube, func(_ context.Context, p *v1alpha1.Project) error {
		syncs++
		p.Status.ID = "zid"

		return nil
	})
	req := ctrl.Request{NamespacedName: key}

	if _, err := r.Reconcile(t.Context(), req); err != nil {
		t.Fatalf("first reconcile: %v", err)
	}
	if syncs != 0 {
		t.Fatal("finalizer add must end the pass before sync")
	}
	got := &v1alpha1.Project{}
	if err := kube.Get(t.Context(), key, got); err != nil {
		t.Fatal(err)
	}
	if !controllerutil.ContainsFinalizer(got, "zitadel-operator.io/finalizer") {
		t.Fatal("finalizer not added")
	}

	// Freeze the "cache" here, then move the server object on.
	stale = got.DeepCopy()
	moved := got.DeepCopy()
	moved.Labels = map[string]string{"edited": "elsewhere"}
	if err := kube.Update(t.Context(), moved); err != nil {
		t.Fatal(err)
	}

	if _, err := r.Reconcile(t.Context(), req); err != nil {
		t.Fatalf("reconcile on stale cache: %v", err)
	}
	stale = nil
	if err := kube.Get(t.Context(), key, got); err != nil {
		t.Fatal(err)
	}
	if got.Status.ID != "zid" || got.Labels["edited"] != "elsewhere" {
		t.Fatalf("status %+v labels %v", got.Status, got.Labels)
	}
}

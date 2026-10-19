package controller_test

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/malpou/zitadel-operator/api/v1alpha1"
	"github.com/malpou/zitadel-operator/internal/controller"
)

// TestWriteClientIDs: every clientIDRefs Secret gets client-id and nothing
// else, in its own namespace or the CR's.
func TestWriteClientIDs(t *testing.T) {
	t.Parallel()
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).Build()
	app := &v1alpha1.OIDCApplication{
		Namespace: "ops", Name: "pwa",
		Spec: v1alpha1.OIDCApplicationSpec{
			AuthMethod: v1alpha1.OIDCAuthMethodBasic,
			ClientIDRefs: []v1alpha1.SecretRef{
				{Name: "pwa-oidc", Namespace: "callout"},
				{Name: "pwa-id"},
			},
		},
		Status: v1alpha1.OIDCApplicationStatus{ClientID: "cid"},
	}
	if err := controller.WriteClientIDs(t.Context(), kube, app); err != nil {
		t.Fatal(err)
	}
	for _, nn := range []types.NamespacedName{{Namespace: "callout", Name: "pwa-oidc"}, {Namespace: "ops", Name: "pwa-id"}} {
		s := &corev1.Secret{}
		if err := kube.Get(t.Context(), nn, s); err != nil {
			t.Fatalf("%s: %v", nn, err)
		}
		if len(s.Data) != 1 || string(s.Data["client-id"]) != "cid" {
			t.Fatalf("%s: data %v, want only client-id", nn, s.Data)
		}
		if s.Labels["app.kubernetes.io/managed-by"] != "zitadel-operator" {
			t.Fatalf("%s: labels %v", nn, s.Labels)
		}
	}
}

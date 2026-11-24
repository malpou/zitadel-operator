package controller_test

import (
	"context"
	"fmt"
	"os"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	zclient "github.com/zitadel/zitadel-go/v3/pkg/client"
	actionv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/action/v2"
	"github.com/zitadel/zitadel-go/v3/pkg/zitadel"
	"google.golang.org/grpc"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	"github.com/malpou/zitadel-operator/api/v1alpha1"
	"github.com/malpou/zitadel-operator/internal/controller"
)

// countingAction counts the action.v2 writes the operator sends.
type countingAction struct {
	actionv2.ActionServiceClient

	writes atomic.Int32
}

func (c *countingAction) CreateTarget(
	ctx context.Context, in *actionv2.CreateTargetRequest, opts ...grpc.CallOption,
) (*actionv2.CreateTargetResponse, error) {
	c.writes.Add(1)

	return c.ActionServiceClient.CreateTarget(ctx, in, opts...)
}

func (c *countingAction) UpdateTarget(
	ctx context.Context, in *actionv2.UpdateTargetRequest, opts ...grpc.CallOption,
) (*actionv2.UpdateTargetResponse, error) {
	c.writes.Add(1)

	return c.ActionServiceClient.UpdateTarget(ctx, in, opts...)
}

func (c *countingAction) SetExecution(
	ctx context.Context, in *actionv2.SetExecutionRequest, opts ...grpc.CallOption,
) (*actionv2.SetExecutionResponse, error) {
	c.writes.Add(1)

	return c.ActionServiceClient.SetExecution(ctx, in, opts...)
}

// TestActionsV2AgainstZitadel drives the Target and Execution reconcilers
// against a real, throwaway Zitadel: create, no-op resync, update, key
// rotation, adoption and delete. It runs only when ZITADEL_TEST_PORT (an
// insecure instance on localhost) and ZITADEL_TEST_PAT (an IAM_OWNER's
// token) are set; never point it at an instance that matters. The endpoint
// is never called, but Zitadel refuses one that does not resolve to a
// public address (Errors.Target.DeniedURL).
//
//nolint:paralleltest // one instance-wide execution per function, shared by the steps.
func TestActionsV2AgainstZitadel(t *testing.T) {
	port, pat := os.Getenv("ZITADEL_TEST_PORT"), os.Getenv("ZITADEL_TEST_PAT")
	if port == "" || pat == "" {
		t.Skip("ZITADEL_TEST_PORT and ZITADEL_TEST_PAT not set")
	}
	h := newHarness(t, port, pat)

	// An execution waits for its target.
	reconcileTwice(t, h.er, "preuserinfo")
	if e := getExecution(t, h.kube, "preuserinfo"); e.Status.Conditions[0].Reason != v1alpha1.ReasonDependencyNotReady {
		t.Fatalf("execution before target: %+v", e.Status)
	}

	// Create.
	reconcileTwice(t, h.tr, "claims")
	target := getTarget(t, h.kube)
	id := target.Status.ID
	if id == "" || target.Status.Conditions[0].Status != metav1.ConditionTrue {
		t.Fatalf("target status %+v", target.Status)
	}
	got := h.zitadelTarget(id)
	if got.GetRestCall() == nil || got.GetRestCall().GetInterruptOnError() ||
		got.GetTimeout().AsDuration() != 3*time.Second {
		t.Fatalf("created target %+v", got)
	}
	key := signingKey(t, h.kube)
	if key == "" || key != got.GetSigningKey() {
		t.Fatalf("secret key %q, zitadel key %q", key, got.GetSigningKey())
	}
	for _, fn := range functions {
		reconcileTwice(t, h.er, fn)
		h.waitExecutionTargets(fn, []string{id})
	}

	// A resync of matching objects writes nothing.
	before := h.action.writes.Load()
	reconcileOnce(t, h.tr, "claims")
	reconcileOnce(t, h.er, "preuserinfo")
	if n := h.action.writes.Load() - before; n != 0 {
		t.Fatalf("resync sent %d writes", n)
	}

	h.updateAndRotate(id, key)
	h.adopt(id)

	// Delete: the executions lose their target list, then the target goes.
	for _, fn := range functions {
		deleteAndReconcile(t, h.kube, h.er, getExecution(t, h.kube, fn))
		h.waitExecutionTargets(fn, nil)
	}
	deleteAndReconcile(t, h.kube, h.tr, getTarget(t, h.kube))
	if _, err := h.action.GetTarget(t.Context(), &actionv2.GetTargetRequest{Id: id}); err == nil {
		t.Fatal("target still in zitadel after delete")
	}
}

//nolint:gochecknoglobals // the two functions the club-tools claims run on.
var functions = []string{"preuserinfo", "preaccesstoken"}

type harness struct {
	t      *testing.T
	kube   client.Client
	action *countingAction
	tr, er reconcile.Reconciler
}

func newHarness(t *testing.T, port, pat string) *harness {
	t.Helper()
	ctx := t.Context()
	zc, err := zclient.New(ctx,
		zitadel.New("localhost", zitadel.WithInsecure(port)), zclient.WithAuth(zclient.PAT(pat)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = zc.Close() })
	action := &countingAction{ActionServiceClient: zc.ActionServiceV2()}

	fns, err := action.ListExecutionFunctions(ctx, &actionv2.ListExecutionFunctionsRequest{})
	if err != nil {
		t.Fatal(err)
	}
	for _, fn := range functions {
		if !slices.Contains(fns.GetFunctions(), fn) {
			t.Fatalf("function %q not in %v", fn, fns.GetFunctions())
		}
	}

	scheme := runtime.NewScheme()
	if err = corev1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	if err = v1alpha1.AddToScheme(scheme); err != nil {
		t.Fatal(err)
	}
	objs := []client.Object{&v1alpha1.Target{
		Namespace: "ns", Name: "claims",
		Spec: v1alpha1.TargetSpec{
			Name: fmt.Sprintf("operator-test-%d", time.Now().UnixNano()), Endpoint: "http://example.com/claims",
			Type: v1alpha1.TargetTypeCall, Timeout: metav1.Duration{Duration: 3 * time.Second},
			PayloadType:         v1alpha1.PayloadTypeJSON,
			SigningKeySecretRef: &v1alpha1.SecretRef{Name: "claims-key", Namespace: "api"},
			DeletionPolicy:      v1alpha1.DeletionPolicyDelete,
		},
	}}
	for _, fn := range functions {
		objs = append(objs, &v1alpha1.Execution{
			Namespace: "ns", Name: fn,
			Spec: v1alpha1.ExecutionSpec{
				Condition:  v1alpha1.ExecutionCondition{Function: fn},
				TargetRefs: []string{"claims"}, DeletionPolicy: v1alpha1.DeletionPolicyDelete,
			},
		})
	}
	kube := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).
		WithStatusSubresource(&v1alpha1.Target{}, &v1alpha1.Execution{}).Build()
	z := controller.Clients{Action: action}

	return &harness{
		t: t, kube: kube, action: action,
		tr: controller.NewTargetReconciler(kube, z), er: controller.NewExecutionReconciler(kube, z),
	}
}

// updateAndRotate changes the endpoint and timeout, then rotates the key.
func (h *harness) updateAndRotate(id, key string) {
	t := h.t
	target := getTarget(t, h.kube)
	target.Spec.Endpoint = "http://example.com/v2/claims"
	target.Spec.Timeout = metav1.Duration{Duration: 5 * time.Second}
	updateObj(t, h.kube, target)
	reconcileOnce(t, h.tr, "claims")
	if got := h.zitadelTarget(id); got.GetEndpoint() != "http://example.com/v2/claims" ||
		got.GetTimeout().AsDuration() != 5*time.Second {
		t.Fatalf("updated target %+v", got)
	}

	target = getTarget(t, h.kube)
	target.Annotations = map[string]string{v1alpha1.AnnotationRotate: "1"}
	updateObj(t, h.kube, target)
	reconcileOnce(t, h.tr, "claims")
	rotated := signingKey(t, h.kube)
	if zkey := h.zitadelTarget(id).GetSigningKey(); rotated == key || rotated != zkey {
		t.Fatalf("rotation: old %q, secret %q, zitadel %q", key, rotated, zkey)
	}
	if getTarget(t, h.kube).Status.RotatedFor != "1" {
		t.Fatal("rotatedFor not recorded")
	}
}

// adopt applies a second CR for the same target name, which must find it.
func (h *harness) adopt(id string) {
	t := h.t
	adopt := &v1alpha1.Target{Namespace: "ns", Name: "adopt", Spec: *getTarget(t, h.kube).Spec.DeepCopy()}
	adopt.Spec.SigningKeySecretRef = nil
	adopt.Spec.DeletionPolicy = v1alpha1.DeletionPolicyOrphan
	if err := h.kube.Create(t.Context(), adopt); err != nil {
		t.Fatal(err)
	}
	reconcileTwice(t, h.tr, "adopt")
	if err := h.kube.Get(t.Context(), client.ObjectKeyFromObject(adopt), adopt); err != nil {
		t.Fatal(err)
	}
	if adopt.Status.ID != id {
		t.Fatalf("adopted %q, want %q", adopt.Status.ID, id)
	}
	deleteAndReconcile(t, h.kube, h.tr, adopt)
}

func (h *harness) zitadelTarget(id string) *actionv2.Target {
	h.t.Helper()
	resp, err := h.action.GetTarget(h.t.Context(), &actionv2.GetTargetRequest{Id: id})
	if err != nil {
		h.t.Fatal(err)
	}

	return resp.GetTarget()
}

// executionTargets waits up to 5 s for fn's target list to equal want: the
// list is read from a projection that trails the write.
func (h *harness) waitExecutionTargets(fn string, want []string) {
	h.t.Helper()
	var have []string
	for range 50 {
		if have = h.executionTargets(fn); slices.Equal(have, want) {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	h.t.Fatalf("%s targets %v, want %v", fn, have, want)
}

func (h *harness) executionTargets(fn string) []string {
	h.t.Helper()
	resp, err := h.action.ListExecutions(h.t.Context(), &actionv2.ListExecutionsRequest{
		Filters: []*actionv2.ExecutionSearchFilter{{Filter: &actionv2.ExecutionSearchFilter_InConditionsFilter{
			InConditionsFilter: &actionv2.InConditionsFilter{Conditions: []*actionv2.Condition{{
				ConditionType: &actionv2.Condition_Function{Function: &actionv2.FunctionExecution{Name: fn}},
			}}},
		}}},
	})
	if err != nil {
		h.t.Fatal(err)
	}
	if len(resp.GetExecutions()) == 0 {
		return nil
	}

	return resp.GetExecutions()[0].GetTargets()
}

func reconcileOnce(t *testing.T, r reconcile.Reconciler, name string) {
	t.Helper()
	if _, err := r.Reconcile(t.Context(), ctrl.Request{Namespace: "ns", Name: name}); err != nil {
		t.Fatalf("reconcile %s: %v", name, err)
	}
}

// reconcileTwice covers a new CR: the first pass only adds the finalizer.
func reconcileTwice(t *testing.T, r reconcile.Reconciler, name string) {
	t.Helper()
	reconcileOnce(t, r, name)
	reconcileOnce(t, r, name)
}

func deleteAndReconcile(t *testing.T, kube client.Client, r reconcile.Reconciler, obj client.Object) {
	t.Helper()
	if err := kube.Delete(t.Context(), obj); err != nil {
		t.Fatal(err)
	}
	reconcileOnce(t, r, obj.GetName())
	if err := kube.Get(t.Context(), client.ObjectKeyFromObject(obj), obj); err == nil {
		t.Fatalf("%s still present after delete", obj.GetName())
	}
}

func updateObj(t *testing.T, kube client.Client, obj client.Object) {
	t.Helper()
	if err := kube.Update(t.Context(), obj); err != nil {
		t.Fatal(err)
	}
}

func getTarget(t *testing.T, kube client.Client) *v1alpha1.Target {
	t.Helper()
	obj := &v1alpha1.Target{}
	if err := kube.Get(t.Context(), types.NamespacedName{Namespace: "ns", Name: "claims"}, obj); err != nil {
		t.Fatal(err)
	}

	return obj
}

func getExecution(t *testing.T, kube client.Client, name string) *v1alpha1.Execution {
	t.Helper()
	obj := &v1alpha1.Execution{}
	if err := kube.Get(t.Context(), types.NamespacedName{Namespace: "ns", Name: name}, obj); err != nil {
		t.Fatal(err)
	}

	return obj
}

func signingKey(t *testing.T, kube client.Client) string {
	t.Helper()
	s := &corev1.Secret{}
	if err := kube.Get(t.Context(), types.NamespacedName{Namespace: "api", Name: "claims-key"}, s); err != nil {
		t.Fatal(err)
	}

	return string(s.Data["signing-key"])
}

// Package controller reconciles the zitadel-operator.io kinds against one
// Zitadel instance. Every kind shares the reconcile driver in this file and
// implements the small syncer interface in its own file.
package controller

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"time"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
	applicationv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/application/v2"
	authorizationv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/authorization/v2"
	permissionv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/internal_permission/v2"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	projectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/project/v2"
	userv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/user/v2"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	"github.com/malpou/zitadel-operator/api/v1alpha1"
)

const (
	finalizer       = "zitadel-operator.io/finalizer"
	resync          = 10 * time.Minute
	dependencyRetry = 30 * time.Second

	labelManagedBy = "app.kubernetes.io/managed-by"
	managedByValue = "zitadel-operator"

	// Secret keys written by the operator.
	keyClientID     = "client-id"
	keyClientSecret = "client-secret"
	keyToken        = "token"
)

var (
	errDependencyNotReady = errors.New("dependency not ready")
	errSecretUnavailable  = errors.New(
		"secret value is only known at creation; set the " + v1alpha1.AnnotationRotate + " annotation to regenerate it")
	errVanished = errors.New("resource vanished from zitadel, recreating on next pass")
)

// Object is implemented by every kind in api/v1alpha1.
type Object interface {
	client.Object
	CommonStatus() *v1alpha1.Status
	DeletionPolicy() v1alpha1.DeletionPolicy
}

// Clients bundles the Zitadel service clients the syncers call.
type Clients struct {
	OrgID         string
	Project       projectv2.ProjectServiceClient
	Application   applicationv2.ApplicationServiceClient
	Authorization authorizationv2.AuthorizationServiceClient
	User          userv2.UserServiceClient
	Permission    permissionv2.InternalPermissionServiceClient
	Management    management.ManagementServiceClient
	Admin         admin.AdminServiceClient
}

// syncer is what each kind implements.
type syncer[T Object] interface {
	// sync makes Zitadel match the spec and fills in status. It returns
	// errDependencyNotReady / errSecretUnavailable for the non-error waits.
	sync(ctx context.Context, obj T) error
	// remove deletes the Zitadel resource; NotFound counts as success.
	remove(ctx context.Context, obj T) error
}

type reconciler[T Object] struct {
	kube     client.Client
	newObj   func() T
	impl     syncer[T]
	finalize bool
}

// Setup registers every controller with the manager.
func Setup(mgr ctrl.Manager, z Clients) error {
	kube := mgr.GetClient()
	setups := []func(ctrl.Manager) error{
		(&reconciler[*v1alpha1.Project]{
			kube: kube, newObj: func() *v1alpha1.Project { return &v1alpha1.Project{} },
			impl: &projectSyncer{z: z}, finalize: true,
		}).setup,
		(&reconciler[*v1alpha1.OIDCApplication]{
			kube: kube, newObj: func() *v1alpha1.OIDCApplication { return &v1alpha1.OIDCApplication{} },
			impl: &oidcApplicationSyncer{kube: kube, z: z}, finalize: true,
		}).setup,
		(&reconciler[*v1alpha1.UserGrant]{
			kube: kube, newObj: func() *v1alpha1.UserGrant { return &v1alpha1.UserGrant{} },
			impl: &userGrantSyncer{kube: kube, z: z}, finalize: true,
		}).setup,
		(&reconciler[*v1alpha1.MachineUser]{
			kube: kube, newObj: func() *v1alpha1.MachineUser { return &v1alpha1.MachineUser{} },
			impl: &machineUserSyncer{kube: kube, z: z}, finalize: true,
		}).setup,
		(&reconciler[*v1alpha1.Action]{
			kube: kube, newObj: func() *v1alpha1.Action { return &v1alpha1.Action{} },
			impl: &actionSyncer{z: z}, finalize: true,
		}).setup,
		// Singletons carry no finalizer: a prune must never reset the login
		// policy, drop the mail provider or blank the message texts.
		(&reconciler[*v1alpha1.LoginPolicy]{
			kube: kube, newObj: func() *v1alpha1.LoginPolicy { return &v1alpha1.LoginPolicy{} },
			impl: &loginPolicySyncer{z: z},
		}).setup,
		(&reconciler[*v1alpha1.EmailProvider]{
			kube: kube, newObj: func() *v1alpha1.EmailProvider { return &v1alpha1.EmailProvider{} },
			impl: &emailProviderSyncer{z: z},
		}).setup,
		(&reconciler[*v1alpha1.MessageText]{
			kube: kube, newObj: func() *v1alpha1.MessageText { return &v1alpha1.MessageText{} },
			impl: &messageTextSyncer{z: z},
		}).setup,
	}
	for _, s := range setups {
		if err := s(mgr); err != nil {
			return err
		}
	}

	return nil
}

func (r *reconciler[T]) setup(mgr ctrl.Manager) error {
	if err := ctrl.NewControllerManagedBy(mgr).For(r.newObj()).Complete(r); err != nil {
		return fmt.Errorf("setup controller for %T: %w", r.newObj(), err)
	}

	return nil
}

// Reconcile implements reconcile.Reconciler.
func (r *reconciler[T]) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	obj := r.newObj()
	if err := r.kube.Get(ctx, req.NamespacedName, obj); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}

		return ctrl.Result{}, fmt.Errorf("get %s: %w", req.NamespacedName, err)
	}
	if !obj.GetDeletionTimestamp().IsZero() {
		return ctrl.Result{}, r.finalizeDelete(ctx, obj)
	}
	if r.finalize && controllerutil.AddFinalizer(obj, finalizer) {
		if err := r.kube.Update(ctx, obj); err != nil {
			return ctrl.Result{}, fmt.Errorf("add finalizer: %w", err)
		}
	}

	result, retErr := classify(obj, r.impl.sync(ctx, obj))
	obj.CommonStatus().ObservedGeneration = obj.GetGeneration()
	if err := r.kube.Status().Update(ctx, obj); err != nil {
		return ctrl.Result{}, fmt.Errorf("update status: %w", err)
	}

	return result, retErr
}

func (r *reconciler[T]) finalizeDelete(ctx context.Context, obj T) error {
	if !controllerutil.ContainsFinalizer(obj, finalizer) {
		return nil
	}
	if obj.DeletionPolicy() != v1alpha1.DeletionPolicyOrphan && obj.CommonStatus().ID != "" {
		if err := r.impl.remove(ctx, obj); err != nil && !isNotFound(err) {
			return err
		}
	}
	controllerutil.RemoveFinalizer(obj, finalizer)
	if err := r.kube.Update(ctx, obj); err != nil {
		return fmt.Errorf("remove finalizer: %w", err)
	}

	return nil
}

// classify turns the sync outcome into the Ready condition and requeue.
func classify(obj Object, err error) (ctrl.Result, error) {
	switch {
	case err == nil:
		setReady(obj, metav1.ConditionTrue, v1alpha1.ReasonSynced, "in sync")

		return ctrl.Result{RequeueAfter: resync}, nil
	case errors.Is(err, errDependencyNotReady):
		setReady(obj, metav1.ConditionFalse, v1alpha1.ReasonDependencyNotReady, err.Error())

		return ctrl.Result{RequeueAfter: dependencyRetry}, nil
	case errors.Is(err, errSecretUnavailable):
		setReady(obj, metav1.ConditionFalse, v1alpha1.ReasonSecretUnavailable, err.Error())

		return ctrl.Result{RequeueAfter: resync}, nil
	default:
		setReady(obj, metav1.ConditionFalse, v1alpha1.ReasonZitadelError, err.Error())

		return ctrl.Result{}, err
	}
}

func setReady(obj Object, st metav1.ConditionStatus, reason, msg string) {
	meta.SetStatusCondition(&obj.CommonStatus().Conditions, metav1.Condition{
		Type:               v1alpha1.ConditionReady,
		Status:             st,
		Reason:             reason,
		Message:            msg,
		ObservedGeneration: obj.GetGeneration(),
	})
}

func isReady(obj Object) bool {
	return meta.IsStatusConditionTrue(obj.CommonStatus().Conditions, v1alpha1.ConditionReady)
}

func isNotFound(err error) bool {
	return status.Code(err) == codes.NotFound
}

// ensureID adopts an existing resource by lookup or creates it.
func ensureID(ctx context.Context, st *v1alpha1.Status, find, create func(context.Context) (string, error)) error {
	if st.ID != "" {
		return nil
	}
	id, err := find(ctx)
	if err != nil {
		return err
	}
	if id == "" {
		if id, err = create(ctx); err != nil {
			return err
		}
	}
	st.ID = id

	return nil
}

// projectID resolves a Project CR in ns to its Zitadel id, gated on Ready so
// roles exist before anything references them.
func projectID(ctx context.Context, kube client.Client, ns, name string) (string, error) {
	p := &v1alpha1.Project{}
	if err := kube.Get(ctx, types.NamespacedName{Namespace: ns, Name: name}, p); err != nil {
		if apierrors.IsNotFound(err) {
			return "", fmt.Errorf("%w: project %q not found", errDependencyNotReady, name)
		}

		return "", fmt.Errorf("get project %q: %w", name, err)
	}
	if p.Status.ID == "" || !isReady(p) {
		return "", fmt.Errorf("%w: project %q", errDependencyNotReady, name)
	}

	return p.Status.ID, nil
}

func secretName(ref v1alpha1.SecretRef, defaultNS string) types.NamespacedName {
	ns := ref.Namespace
	if ns == "" {
		ns = defaultNS
	}

	return types.NamespacedName{Namespace: ns, Name: ref.Name}
}

// writeSecret merges data into the referenced Secret, creating it if needed.
// Other keys in the Secret are left alone.
func writeSecret(
	ctx context.Context, kube client.Client, ref v1alpha1.SecretRef, defaultNS string, data map[string][]byte,
) error {
	nn := secretName(ref, defaultNS)
	s := &corev1.Secret{Name: nn.Name, Namespace: nn.Namespace}
	_, err := controllerutil.CreateOrUpdate(ctx, kube, s, func() error {
		if s.Labels == nil {
			s.Labels = map[string]string{}
		}
		s.Labels[labelManagedBy] = managedByValue
		if s.Data == nil {
			s.Data = map[string][]byte{}
		}
		maps.Copy(s.Data, data)

		return nil
	})
	if err != nil {
		return fmt.Errorf("write secret %s: %w", nn, err)
	}

	return nil
}

// secretHasKey reports whether the referenced Secret holds a non-empty key.
func secretHasKey(
	ctx context.Context,
	kube client.Client,
	ref v1alpha1.SecretRef,
	defaultNS, key string,
) (bool, error) {
	nn := secretName(ref, defaultNS)
	s := &corev1.Secret{}
	if err := kube.Get(ctx, nn, s); err != nil {
		if apierrors.IsNotFound(err) {
			return false, nil
		}

		return false, fmt.Errorf("get secret %s: %w", nn, err)
	}

	return len(s.Data[key]) > 0, nil
}

// rotateRequested reports the rotate annotation value and whether it differs
// from what the current secret was generated for.
func rotateRequested(obj client.Object, rotatedFor string) (string, bool) {
	v := obj.GetAnnotations()[v1alpha1.AnnotationRotate]

	return v, v != "" && v != rotatedFor
}

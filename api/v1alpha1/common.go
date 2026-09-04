package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// DeletionPolicy says what happens in Zitadel when the CR is deleted.
// +kubebuilder:validation:Enum=Delete;Orphan
type DeletionPolicy string

const (
	// DeletionPolicyDelete removes the Zitadel resource when the CR goes away.
	DeletionPolicyDelete DeletionPolicy = "Delete"
	// DeletionPolicyOrphan leaves the Zitadel resource in place.
	DeletionPolicyOrphan DeletionPolicy = "Orphan"
)

// SecretRef names the Secret an output (client credentials, token) is written to.
type SecretRef struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Namespace defaults to the CR's namespace.
	Namespace string `json:"namespace,omitempty"`
}

// Status is the part of every kind's status the shared reconcile helpers manage.
type Status struct {
	// ID is the Zitadel resource id once created or adopted.
	ID string `json:"id,omitempty"`
	// ObservedGeneration is the spec generation the status reflects.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// Conditions carries the single Ready condition.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// ConditionReady is the one condition type every kind reports.
const ConditionReady = "Ready"

// Condition reasons.
const (
	ReasonSynced             = "Synced"
	ReasonDependencyNotReady = "DependencyNotReady"
	ReasonZitadelError       = "ZitadelError"
	ReasonSecretUnavailable  = "SecretUnavailable"
)

// AnnotationRotate triggers a client-secret / PAT rotation when its value
// differs from status.rotatedFor.
const AnnotationRotate = "zitadel-operator.io/rotate"

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ExecutionCondition is when Zitadel runs the targets. Only function
// conditions exist so far; request, response and event conditions would be
// siblings of Function.
type ExecutionCondition struct {
	// Function is a Zitadel function name, for example preuserinfo or
	// preaccesstoken (the actions v1 flows they replace).
	// +kubebuilder:validation:MinLength=1
	Function string `json:"function"`
}

// ExecutionSpec mirrors the Terraform provider's zitadel_execution_function
// (actions v2). The CR owns the condition: its target list replaces whatever
// Zitadel held for it.
type ExecutionSpec struct {
	// +kubebuilder:validation:XValidation:rule="self == oldSelf",message="condition is immutable; create a new Execution"
	Condition ExecutionCondition `json:"condition"`
	// TargetRefs names Target CRs in the same namespace, called in this order.
	// +kubebuilder:validation:MinItems=1
	TargetRefs []string `json:"targetRefs"`
	// +kubebuilder:default=Delete
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`
}

// Execution binds Targets to a condition. Executions are instance-wide.
// status.id is the condition, for example function/preuserinfo.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.id`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
type Execution struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ExecutionSpec `json:"spec"`
	Status Status        `json:"status,omitempty"`
}

// CommonStatus implements Object.
func (e *Execution) CommonStatus() *Status { return &e.Status }

// DeletionPolicy implements Object.
func (e *Execution) DeletionPolicy() DeletionPolicy { return e.Spec.DeletionPolicy }

// ExecutionList is a list of Execution.
// +kubebuilder:object:root=true
type ExecutionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Execution `json:"items"`
}

package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ProjectRole is a role inside a project.
type ProjectRole struct {
	// +kubebuilder:validation:MinLength=1
	Key         string `json:"key"`
	DisplayName string `json:"displayName,omitempty"`
	Group       string `json:"group,omitempty"`
}

// ProjectSpec mirrors the Terraform provider's zitadel_project + zitadel_project_role.
type ProjectSpec struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// RoleAssertion puts the user's roles of this project into tokens.
	RoleAssertion bool `json:"roleAssertion,omitempty"`
	// AuthorizationRequired requires the user to hold a grant on this
	// project to log in (the Terraform provider calls this project_role_check).
	AuthorizationRequired bool          `json:"authorizationRequired,omitempty"`
	Roles                 []ProjectRole `json:"roles,omitempty"`
	// +kubebuilder:default=Delete
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`
}

// Project is a Zitadel project in the operator's organization.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.id`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
type Project struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectSpec `json:"spec"`
	Status Status      `json:"status,omitempty"`
}

// CommonStatus implements Object.
func (p *Project) CommonStatus() *Status { return &p.Status }

// DeletionPolicy implements Object.
func (p *Project) DeletionPolicy() DeletionPolicy { return p.Spec.DeletionPolicy }

// ProjectList is a list of Project.
// +kubebuilder:object:root=true
type ProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Project `json:"items"`
}

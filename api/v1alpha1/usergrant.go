package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// UserGrantSpec mirrors the Terraform provider's zitadel_user_grant (authorization.v2).
type UserGrantSpec struct {
	// +kubebuilder:validation:MinLength=1
	UserID string `json:"userID"`
	// +kubebuilder:validation:MinLength=1
	ProjectRef string `json:"projectRef"`
	// +kubebuilder:validation:MinItems=1
	RoleKeys []string `json:"roleKeys"`
	// +kubebuilder:default=Delete
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`
}

// UserGrant gives a user roles on a Project.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.id`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
type UserGrant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   UserGrantSpec `json:"spec"`
	Status Status        `json:"status,omitempty"`
}

// CommonStatus implements Object.
func (g *UserGrant) CommonStatus() *Status { return &g.Status }

// DeletionPolicy implements Object.
func (g *UserGrant) DeletionPolicy() DeletionPolicy { return g.Spec.DeletionPolicy }

// UserGrantList is a list of UserGrant.
// +kubebuilder:object:root=true
type UserGrantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []UserGrant `json:"items"`
}

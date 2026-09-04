package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// PersonalAccessToken asks for one PAT on the machine user, written to a Secret key "token".
type PersonalAccessToken struct {
	Expiration metav1.Time `json:"expiration"`
	SecretRef  SecretRef   `json:"secretRef"`
}

// MachineUserSpec mirrors the Terraform provider's zitadel_machine_user + zitadel_personal_access_token + zitadel_project_member.
type MachineUserSpec struct {
	// +kubebuilder:validation:MinLength=1
	UserName string `json:"userName"`
	// +kubebuilder:validation:MinLength=1
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	// +kubebuilder:default=BEARER
	AccessTokenType OIDCTokenType        `json:"accessTokenType,omitempty"`
	PAT             *PersonalAccessToken `json:"pat,omitempty"`
	// ProjectOwnerOf lists Project CR names this user is PROJECT_OWNER of.
	ProjectOwnerOf []string `json:"projectOwnerOf,omitempty"`
	// +kubebuilder:default=Delete
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`
}

// MachineUserStatus adds PAT bookkeeping.
type MachineUserStatus struct {
	Status `json:",inline"`

	PATID string `json:"patID,omitempty"`
	// RotatedFor is the rotate annotation value the current PAT was minted for.
	RotatedFor string `json:"rotatedFor,omitempty"`
}

// MachineUser is a service user in the operator's organization.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.id`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
type MachineUser struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MachineUserSpec   `json:"spec"`
	Status MachineUserStatus `json:"status,omitempty"`
}

// CommonStatus implements Object.
func (u *MachineUser) CommonStatus() *Status { return &u.Status.Status }

// DeletionPolicy implements Object.
func (u *MachineUser) DeletionPolicy() DeletionPolicy { return u.Spec.DeletionPolicy }

// MachineUserList is a list of MachineUser.
// +kubebuilder:object:root=true
type MachineUserList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MachineUser `json:"items"`
}

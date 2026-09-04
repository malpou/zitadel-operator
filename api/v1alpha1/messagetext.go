package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// MessageTextType is which notification mail the text customises.
// +kubebuilder:validation:Enum=VerifyEmail;PasswordReset;PasswordChange
type MessageTextType string

// Message text types.
const (
	MessageTextVerifyEmail    MessageTextType = "VerifyEmail"
	MessageTextPasswordReset  MessageTextType = "PasswordReset"
	MessageTextPasswordChange MessageTextType = "PasswordChange"
)

// MessageTextSpec mirrors the Terraform provider's zitadel_default_*_message_text (instance level).
type MessageTextSpec struct {
	Type MessageTextType `json:"type"`
	// +kubebuilder:validation:MinLength=2
	Language   string `json:"language"`
	Title      string `json:"title,omitempty"`
	PreHeader  string `json:"preHeader,omitempty"`
	Subject    string `json:"subject,omitempty"`
	Greeting   string `json:"greeting,omitempty"`
	Text       string `json:"text,omitempty"`
	ButtonText string `json:"buttonText,omitempty"`
}

// MessageText is an instance-level custom notification text for one
// (type, language). Deleting the CR leaves Zitadel untouched.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Lang",type=string,JSONPath=`.spec.language`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
type MessageText struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   MessageTextSpec `json:"spec"`
	Status Status          `json:"status,omitempty"`
}

// CommonStatus implements Object.
func (t *MessageText) CommonStatus() *Status { return &t.Status }

// DeletionPolicy implements Object; singletons are always orphaned.
func (t *MessageText) DeletionPolicy() DeletionPolicy { return DeletionPolicyOrphan }

// MessageTextList is a list of MessageText.
// +kubebuilder:object:root=true
type MessageTextList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []MessageText `json:"items"`
}

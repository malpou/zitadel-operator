package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// EmailProviderSpec mirrors the Terraform provider's zitadel_smtp_config (instance-level SMTP email
// provider, unauthenticated).
type EmailProviderSpec struct {
	Description string `json:"description,omitempty"`
	// +kubebuilder:validation:MinLength=1
	Host string `json:"host"`
	TLS  bool   `json:"tls,omitempty"`
	// +kubebuilder:validation:MinLength=1
	SenderAddress  string `json:"senderAddress"`
	SenderName     string `json:"senderName,omitempty"`
	ReplyToAddress string `json:"replyToAddress,omitempty"`
	// Active makes this the provider Zitadel sends with.
	Active bool `json:"active,omitempty"`
}

// EmailProvider is an instance-level SMTP provider, adopted by host and
// sender address. Deleting the CR leaves Zitadel untouched.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.id`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
type EmailProvider struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   EmailProviderSpec `json:"spec"`
	Status Status            `json:"status,omitempty"`
}

// CommonStatus implements Object.
func (p *EmailProvider) CommonStatus() *Status { return &p.Status }

// DeletionPolicy implements Object; singletons are always orphaned.
func (p *EmailProvider) DeletionPolicy() DeletionPolicy { return DeletionPolicyOrphan }

// EmailProviderList is a list of EmailProvider.
// +kubebuilder:object:root=true
type EmailProviderList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []EmailProvider `json:"items"`
}

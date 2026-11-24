package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// TargetType says how Zitadel calls a target and what it does with the answer.
// +kubebuilder:validation:Enum=Webhook;Call;Async
type TargetType string

// Target types (action.v2 rest_webhook, rest_call, rest_async).
const (
	// TargetTypeWebhook expects a 2xx and ignores the body.
	TargetTypeWebhook TargetType = "Webhook"
	// TargetTypeCall uses the response body, for example to add claims.
	TargetTypeCall TargetType = "Call"
	// TargetTypeAsync fires and forgets.
	TargetTypeAsync TargetType = "Async"
)

// PayloadType says how the request body is formatted and secured.
// +kubebuilder:validation:Enum=JSON;JWT;JWE
type PayloadType string

// Payload types.
const (
	PayloadTypeJSON PayloadType = "JSON"
	PayloadTypeJWT  PayloadType = "JWT"
	PayloadTypeJWE  PayloadType = "JWE"
)

// TargetSpec mirrors the Terraform provider's zitadel_target (actions v2).
type TargetSpec struct {
	// Name is the target's display name and the key it is adopted by.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:validation:MinLength=1
	Endpoint string `json:"endpoint"`
	// +kubebuilder:default=Call
	Type TargetType `json:"type,omitempty"`
	// InterruptOnError stops the whole execution, a login for example, when
	// the target fails. Off, a failing target is skipped. Async ignores it.
	InterruptOnError bool `json:"interruptOnError,omitempty"`
	// +kubebuilder:default="10s"
	Timeout metav1.Duration `json:"timeout,omitempty"`
	// +kubebuilder:default=JSON
	PayloadType PayloadType `json:"payloadType,omitempty"`
	// SigningKeySecretRef writes the key Zitadel signs each request with
	// (header X-ZITADEL-Signature) to key `signing-key`. Annotate the CR with
	// zitadel-operator.io/rotate to have Zitadel generate a new one.
	SigningKeySecretRef *SecretRef `json:"signingKeySecretRef,omitempty"`
	// +kubebuilder:default=Delete
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`
}

// TargetStatus adds the rotation marker to the common status.
type TargetStatus struct {
	Status `json:",inline"`

	// RotatedFor is the rotate annotation value the current signing key was
	// generated for.
	RotatedFor string `json:"rotatedFor,omitempty"`
}

// Target is an actions v2 endpoint Zitadel calls from the executions that
// name it. Targets are instance-wide.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.id`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
type Target struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   TargetSpec   `json:"spec"`
	Status TargetStatus `json:"status,omitempty"`
}

// CommonStatus implements Object.
func (t *Target) CommonStatus() *Status { return &t.Status.Status }

// DeletionPolicy implements Object.
func (t *Target) DeletionPolicy() DeletionPolicy { return t.Spec.DeletionPolicy }

// TargetList is a list of Target.
// +kubebuilder:object:root=true
type TargetList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Target `json:"items"`
}

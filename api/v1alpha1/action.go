package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// FlowType is a Zitadel actions v1 flow.
// +kubebuilder:validation:Enum=CUSTOMISE_TOKEN
type FlowType string

// TriggerType is a trigger inside a flow.
// +kubebuilder:validation:Enum=PRE_USERINFO_CREATION;PRE_ACCESS_TOKEN_CREATION
type TriggerType string

// Flow and trigger values.
const (
	FlowCustomiseToken            FlowType    = "CUSTOMISE_TOKEN"
	TriggerPreUserinfoCreation    TriggerType = "PRE_USERINFO_CREATION"
	TriggerPreAccessTokenCreation TriggerType = "PRE_ACCESS_TOKEN_CREATION"
)

// ActionTrigger binds the action to a (flow, trigger). The CR owns the whole
// action list of that pair.
type ActionTrigger struct {
	FlowType    FlowType    `json:"flowType"`
	TriggerType TriggerType `json:"triggerType"`
}

// ActionSpec mirrors the Terraform provider's zitadel_action + zitadel_trigger_actions (actions v1).
type ActionSpec struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// +kubebuilder:validation:MinLength=1
	Script string `json:"script"`
	// +kubebuilder:default="10s"
	Timeout       metav1.Duration `json:"timeout,omitempty"`
	AllowedToFail bool            `json:"allowedToFail,omitempty"`
	Triggers      []ActionTrigger `json:"triggers,omitempty"`
	// +kubebuilder:default=Delete
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`
}

// Action is an actions v1 script plus the flow triggers that run it.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.id`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
type Action struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ActionSpec `json:"spec"`
	Status Status     `json:"status,omitempty"`
}

// CommonStatus implements Object.
func (a *Action) CommonStatus() *Status { return &a.Status }

// DeletionPolicy implements Object.
func (a *Action) DeletionPolicy() DeletionPolicy { return a.Spec.DeletionPolicy }

// ActionList is a list of Action.
// +kubebuilder:object:root=true
type ActionList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []Action `json:"items"`
}

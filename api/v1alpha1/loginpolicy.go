package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// PasswordlessType says whether passwordless login is offered.
// +kubebuilder:validation:Enum=ALLOWED;NOT_ALLOWED
type PasswordlessType string

// SecondFactor is a 2FA method.
// +kubebuilder:validation:Enum=OTP;U2F;OTP_EMAIL;OTP_SMS;RECOVERY_CODES
type SecondFactor string

// MultiFactor is a passwordless MFA method.
// +kubebuilder:validation:Enum=U2F_WITH_VERIFICATION
type MultiFactor string

// LoginPolicySpec mirrors the Terraform provider's zitadel_login_policy. The org has exactly one
// login policy, so one CR replaces the whole policy.
type LoginPolicySpec struct {
	AllowRegister          bool `json:"allowRegister,omitempty"`
	UserLogin              bool `json:"userLogin,omitempty"`
	AllowExternalIDP       bool `json:"allowExternalIDP,omitempty"`
	AllowDomainDiscovery   bool `json:"allowDomainDiscovery,omitempty"`
	DisableLoginWithEmail  bool `json:"disableLoginWithEmail,omitempty"`
	DisableLoginWithPhone  bool `json:"disableLoginWithPhone,omitempty"`
	ForceMFA               bool `json:"forceMFA,omitempty"`
	ForceMFALocalOnly      bool `json:"forceMFALocalOnly,omitempty"`
	HidePasswordReset      bool `json:"hidePasswordReset,omitempty"`
	IgnoreUnknownUsernames bool `json:"ignoreUnknownUsernames,omitempty"`
	// +kubebuilder:default=NOT_ALLOWED
	PasswordlessType           PasswordlessType `json:"passwordlessType,omitempty"`
	DefaultRedirectURI         string           `json:"defaultRedirectURI,omitempty"`
	PasswordCheckLifetime      metav1.Duration  `json:"passwordCheckLifetime,omitempty"`
	ExternalLoginCheckLifetime metav1.Duration  `json:"externalLoginCheckLifetime,omitempty"`
	MFAInitSkipLifetime        metav1.Duration  `json:"mfaInitSkipLifetime,omitempty"`
	SecondFactorCheckLifetime  metav1.Duration  `json:"secondFactorCheckLifetime,omitempty"`
	MultiFactorCheckLifetime   metav1.Duration  `json:"multiFactorCheckLifetime,omitempty"`
	SecondFactors              []SecondFactor   `json:"secondFactors,omitempty"`
	MultiFactors               []MultiFactor    `json:"multiFactors,omitempty"`
}

// LoginPolicy is the organization's custom login policy. Deleting the CR
// leaves Zitadel untouched.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
type LoginPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   LoginPolicySpec `json:"spec"`
	Status Status          `json:"status,omitempty"`
}

// CommonStatus implements Object.
func (p *LoginPolicy) CommonStatus() *Status { return &p.Status }

// DeletionPolicy implements Object; singletons are always orphaned.
func (p *LoginPolicy) DeletionPolicy() DeletionPolicy { return DeletionPolicyOrphan }

// LoginPolicyList is a list of LoginPolicy.
// +kubebuilder:object:root=true
type LoginPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []LoginPolicy `json:"items"`
}

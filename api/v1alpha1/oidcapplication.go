package v1alpha1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// OIDCAppType is the OIDC application type.
// +kubebuilder:validation:Enum=WEB;USER_AGENT;NATIVE
type OIDCAppType string

// OIDCAuthMethod is how the client authenticates at the token endpoint.
// +kubebuilder:validation:Enum=BASIC;NONE
type OIDCAuthMethod string

// OIDCTokenType is the access token format.
// +kubebuilder:validation:Enum=BEARER;JWT
type OIDCTokenType string

// OIDCResponseType is an OIDC response type.
// +kubebuilder:validation:Enum=CODE
type OIDCResponseType string

// OIDCGrantType is an OIDC grant type.
// +kubebuilder:validation:Enum=AUTHORIZATION_CODE;REFRESH_TOKEN
type OIDCGrantType string

// OIDC enum values.
const (
	OIDCAppTypeWeb       OIDCAppType = "WEB"
	OIDCAppTypeUserAgent OIDCAppType = "USER_AGENT"
	OIDCAppTypeNative    OIDCAppType = "NATIVE"

	OIDCAuthMethodBasic OIDCAuthMethod = "BASIC"
	OIDCAuthMethodNone  OIDCAuthMethod = "NONE"

	OIDCTokenTypeBearer OIDCTokenType = "BEARER"
	OIDCTokenTypeJWT    OIDCTokenType = "JWT"

	OIDCResponseTypeCode OIDCResponseType = "CODE"

	OIDCGrantTypeAuthorizationCode OIDCGrantType = "AUTHORIZATION_CODE"
	OIDCGrantTypeRefreshToken      OIDCGrantType = "REFRESH_TOKEN"
)

// OIDCApplicationSpec mirrors the Terraform provider's zitadel_application_oidc.
type OIDCApplicationSpec struct {
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// ProjectRef names the Project CR (same namespace) the app belongs to.
	// +kubebuilder:validation:MinLength=1
	ProjectRef             string   `json:"projectRef"`
	RedirectURIs           []string `json:"redirectURIs,omitempty"`
	PostLogoutRedirectURIs []string `json:"postLogoutRedirectURIs,omitempty"`
	// +kubebuilder:default=WEB
	AppType OIDCAppType `json:"appType,omitempty"`
	// +kubebuilder:default=BASIC
	AuthMethod OIDCAuthMethod `json:"authMethod,omitempty"`
	// +kubebuilder:default={CODE}
	ResponseTypes []OIDCResponseType `json:"responseTypes,omitempty"`
	// +kubebuilder:default={AUTHORIZATION_CODE,REFRESH_TOKEN}
	GrantTypes []OIDCGrantType `json:"grantTypes,omitempty"`
	// +kubebuilder:default=BEARER
	AccessTokenType          OIDCTokenType `json:"accessTokenType,omitempty"`
	IDTokenRoleAssertion     bool          `json:"idTokenRoleAssertion,omitempty"`
	IDTokenUserinfoAssertion bool          `json:"idTokenUserinfoAssertion,omitempty"`
	AccessTokenRoleAssertion bool          `json:"accessTokenRoleAssertion,omitempty"`
	SkipNativeAppSuccessPage bool          `json:"skipNativeAppSuccessPage,omitempty"`
	// SecretRef receives client-id (always) and client-secret (BASIC only).
	SecretRef *SecretRef `json:"secretRef,omitempty"`
	// +kubebuilder:default=Delete
	DeletionPolicy DeletionPolicy `json:"deletionPolicy,omitempty"`
}

// OIDCApplicationStatus adds the public client id and rotation bookkeeping.
type OIDCApplicationStatus struct {
	Status `json:",inline"`

	ClientID string `json:"clientID,omitempty"`
	// RotatedFor is the rotate annotation value the current secret was generated for.
	RotatedFor string `json:"rotatedFor,omitempty"`
}

// OIDCApplication is an OIDC client in a Project.
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="ID",type=string,JSONPath=`.status.id`
// +kubebuilder:printcolumn:name="Client ID",type=string,JSONPath=`.status.clientID`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
type OIDCApplication struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OIDCApplicationSpec   `json:"spec"`
	Status OIDCApplicationStatus `json:"status,omitempty"`
}

// CommonStatus implements Object.
func (a *OIDCApplication) CommonStatus() *Status { return &a.Status.Status }

// DeletionPolicy implements Object.
func (a *OIDCApplication) DeletionPolicy() DeletionPolicy { return a.Spec.DeletionPolicy }

// OIDCApplicationList is a list of OIDCApplication.
// +kubebuilder:object:root=true
type OIDCApplicationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []OIDCApplication `json:"items"`
}

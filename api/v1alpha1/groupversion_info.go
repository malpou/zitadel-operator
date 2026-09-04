// Package v1alpha1 contains the zitadel-operator.io API types: one kind per
// Zitadel resource the operator manages.
//
// +kubebuilder:object:generate=true
// +groupName=zitadel-operator.io
package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

//go:generate go tool controller-gen object paths=./... crd output:crd:dir=../../deploy/crds

//nolint:gochecknoglobals // scheme registration convention.
var (
	// GroupVersion is the group version used to register these objects.
	GroupVersion = schema.GroupVersion{Group: "zitadel-operator.io", Version: "v1alpha1"}

	// SchemeBuilder registers every kind of this group version.
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)

	// AddToScheme adds the types in this group-version to the given scheme.
	AddToScheme = SchemeBuilder.AddToScheme
)

func addKnownTypes(s *runtime.Scheme) error {
	s.AddKnownTypes(GroupVersion,
		&Project{}, &ProjectList{},
		&OIDCApplication{}, &OIDCApplicationList{},
		&UserGrant{}, &UserGrantList{},
		&MachineUser{}, &MachineUserList{},
		&LoginPolicy{}, &LoginPolicyList{},
		&EmailProvider{}, &EmailProviderList{},
		&MessageText{}, &MessageTextList{},
		&Action{}, &ActionList{},
	)
	metav1.AddToGroupVersion(s, GroupVersion)

	return nil
}

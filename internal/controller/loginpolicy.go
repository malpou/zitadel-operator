package controller

import (
	"context"
	"fmt"
	"slices"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/policy"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/malpou/zitadel-operator/api/v1alpha1"
)

// loginPolicySyncer manages the org's one custom login policy (management v1,
// gone in Zitadel V5).
type loginPolicySyncer struct {
	z Clients
}

func (s *loginPolicySyncer) sync(ctx context.Context, p *v1alpha1.LoginPolicy) error {
	got, err := s.z.Management.GetLoginPolicy(ctx, &management.GetLoginPolicyRequest{})
	if err != nil {
		return fmt.Errorf("get login policy: %w", err)
	}
	want := loginPolicyUpdate(p.Spec)
	if got.GetIsDefault() {
		if _, err = s.z.Management.AddCustomLoginPolicy(ctx, &management.AddCustomLoginPolicyRequest{
			AllowUsernamePassword:      want.GetAllowUsernamePassword(),
			AllowRegister:              want.GetAllowRegister(),
			AllowExternalIdp:           want.GetAllowExternalIdp(),
			ForceMfa:                   want.GetForceMfa(),
			ForceMfaLocalOnly:          want.GetForceMfaLocalOnly(),
			PasswordlessType:           want.GetPasswordlessType(),
			HidePasswordReset:          want.GetHidePasswordReset(),
			IgnoreUnknownUsernames:     want.GetIgnoreUnknownUsernames(),
			DefaultRedirectUri:         want.GetDefaultRedirectUri(),
			AllowDomainDiscovery:       want.GetAllowDomainDiscovery(),
			DisableLoginWithEmail:      want.GetDisableLoginWithEmail(),
			DisableLoginWithPhone:      want.GetDisableLoginWithPhone(),
			PasswordCheckLifetime:      want.GetPasswordCheckLifetime(),
			ExternalLoginCheckLifetime: want.GetExternalLoginCheckLifetime(),
			MfaInitSkipLifetime:        want.GetMfaInitSkipLifetime(),
			SecondFactorCheckLifetime:  want.GetSecondFactorCheckLifetime(),
			MultiFactorCheckLifetime:   want.GetMultiFactorCheckLifetime(),
			SecondFactors:              secondFactors(p.Spec.SecondFactors),
			MultiFactors:               multiFactors(p.Spec.MultiFactors),
		}); err != nil {
			return fmt.Errorf("add custom login policy: %w", err)
		}

		return nil
	}
	if loginPolicyDiffers(got.GetPolicy(), want) {
		if _, err = s.z.Management.UpdateCustomLoginPolicy(ctx, want); err != nil {
			return fmt.Errorf("update custom login policy: %w", err)
		}
	}

	return s.syncFactors(ctx, got.GetPolicy(), p.Spec)
}

func (*loginPolicySyncer) remove(context.Context, *v1alpha1.LoginPolicy) error { return nil }

// syncFactors reconciles the factor sets; the update request cannot carry them.
func (s *loginPolicySyncer) syncFactors(
	ctx context.Context,
	cur *policy.LoginPolicy,
	spec v1alpha1.LoginPolicySpec,
) error {
	if err := syncSet(ctx, "second factor", secondFactors(spec.SecondFactors), cur.GetSecondFactors(),
		func(ctx context.Context, f policy.SecondFactorType) error {
			_, err := s.z.Management.AddSecondFactorToLoginPolicy(ctx,
				&management.AddSecondFactorToLoginPolicyRequest{Type: f})

			return err //nolint:wrapcheck // wrapped by syncSet.
		},
		func(ctx context.Context, f policy.SecondFactorType) error {
			_, err := s.z.Management.RemoveSecondFactorFromLoginPolicy(ctx,
				&management.RemoveSecondFactorFromLoginPolicyRequest{Type: f})

			return err //nolint:wrapcheck // wrapped by syncSet.
		}); err != nil {
		return err
	}

	return syncSet(ctx, "multi factor", multiFactors(spec.MultiFactors), cur.GetMultiFactors(),
		func(ctx context.Context, f policy.MultiFactorType) error {
			_, err := s.z.Management.AddMultiFactorToLoginPolicy(ctx,
				&management.AddMultiFactorToLoginPolicyRequest{Type: f})

			return err //nolint:wrapcheck // wrapped by syncSet.
		},
		func(ctx context.Context, f policy.MultiFactorType) error {
			_, err := s.z.Management.RemoveMultiFactorFromLoginPolicy(ctx,
				&management.RemoveMultiFactorFromLoginPolicyRequest{Type: f})

			return err //nolint:wrapcheck // wrapped by syncSet.
		})
}

// syncSet adds what is wanted but missing and removes what is present but unwanted.
func syncSet[T comparable](
	ctx context.Context, what string, want, have []T, add, remove func(context.Context, T) error,
) error {
	for _, f := range want {
		if !slices.Contains(have, f) {
			if err := add(ctx, f); err != nil {
				return fmt.Errorf("add %s %v: %w", what, f, err)
			}
		}
	}
	for _, f := range have {
		if !slices.Contains(want, f) {
			if err := remove(ctx, f); err != nil {
				return fmt.Errorf("remove %s %v: %w", what, f, err)
			}
		}
	}

	return nil
}

func loginPolicyUpdate(spec v1alpha1.LoginPolicySpec) *management.UpdateCustomLoginPolicyRequest {
	return &management.UpdateCustomLoginPolicyRequest{
		AllowUsernamePassword: spec.UserLogin,
		AllowRegister:         spec.AllowRegister,
		AllowExternalIdp:      spec.AllowExternalIDP,
		ForceMfa:              spec.ForceMFA,
		ForceMfaLocalOnly:     spec.ForceMFALocalOnly,
		PasswordlessType: policy.PasswordlessType(
			policy.PasswordlessType_value["PASSWORDLESS_TYPE_"+string(spec.PasswordlessType)],
		),
		HidePasswordReset:      spec.HidePasswordReset,
		IgnoreUnknownUsernames: spec.IgnoreUnknownUsernames,
		DefaultRedirectUri:     spec.DefaultRedirectURI,
		AllowDomainDiscovery:   spec.AllowDomainDiscovery,
		DisableLoginWithEmail:  spec.DisableLoginWithEmail,
		DisableLoginWithPhone:  spec.DisableLoginWithPhone,

		PasswordCheckLifetime:      durationpb.New(spec.PasswordCheckLifetime.Duration),
		ExternalLoginCheckLifetime: durationpb.New(spec.ExternalLoginCheckLifetime.Duration),
		MfaInitSkipLifetime:        durationpb.New(spec.MFAInitSkipLifetime.Duration),
		SecondFactorCheckLifetime:  durationpb.New(spec.SecondFactorCheckLifetime.Duration),
		MultiFactorCheckLifetime:   durationpb.New(spec.MultiFactorCheckLifetime.Duration),
	}
}

func loginPolicyDiffers(cur *policy.LoginPolicy, want *management.UpdateCustomLoginPolicyRequest) bool {
	return cur.GetAllowUsernamePassword() != want.GetAllowUsernamePassword() ||
		cur.GetAllowRegister() != want.GetAllowRegister() ||
		cur.GetAllowExternalIdp() != want.GetAllowExternalIdp() ||
		cur.GetForceMfa() != want.GetForceMfa() ||
		cur.GetForceMfaLocalOnly() != want.GetForceMfaLocalOnly() ||
		cur.GetPasswordlessType() != want.GetPasswordlessType() ||
		cur.GetHidePasswordReset() != want.GetHidePasswordReset() ||
		cur.GetIgnoreUnknownUsernames() != want.GetIgnoreUnknownUsernames() ||
		cur.GetDefaultRedirectUri() != want.GetDefaultRedirectUri() ||
		cur.GetAllowDomainDiscovery() != want.GetAllowDomainDiscovery() ||
		cur.GetDisableLoginWithEmail() != want.GetDisableLoginWithEmail() ||
		cur.GetDisableLoginWithPhone() != want.GetDisableLoginWithPhone() ||
		cur.GetPasswordCheckLifetime().AsDuration() != want.GetPasswordCheckLifetime().AsDuration() ||
		cur.GetExternalLoginCheckLifetime().AsDuration() != want.GetExternalLoginCheckLifetime().AsDuration() ||
		cur.GetMfaInitSkipLifetime().AsDuration() != want.GetMfaInitSkipLifetime().AsDuration() ||
		cur.GetSecondFactorCheckLifetime().AsDuration() != want.GetSecondFactorCheckLifetime().AsDuration() ||
		cur.GetMultiFactorCheckLifetime().AsDuration() != want.GetMultiFactorCheckLifetime().AsDuration()
}

func secondFactors(in []v1alpha1.SecondFactor) []policy.SecondFactorType {
	out := make([]policy.SecondFactorType, 0, len(in))
	for _, f := range in {
		out = append(out, policy.SecondFactorType(policy.SecondFactorType_value["SECOND_FACTOR_TYPE_"+string(f)]))
	}

	return out
}

func multiFactors(in []v1alpha1.MultiFactor) []policy.MultiFactorType {
	out := make([]policy.MultiFactorType, 0, len(in))
	for _, f := range in {
		out = append(out, policy.MultiFactorType(policy.MultiFactorType_value["MULTI_FACTOR_TYPE_"+string(f)]))
	}

	return out
}

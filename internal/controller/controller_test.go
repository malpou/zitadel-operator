package controller_test

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"testing"

	applicationv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/application/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/malpou/zitadel-operator/api/v1alpha1"
	"github.com/malpou/zitadel-operator/internal/controller"
)

var errBoom = errors.New("boom")

func TestClassify(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name    string
		err     error
		ready   metav1.ConditionStatus
		reason  string
		requeue bool
		wantErr bool
	}{
		{"synced", nil, metav1.ConditionTrue, v1alpha1.ReasonSynced, true, false},
		{
			"dependency", fmt.Errorf("x: %w", controller.ErrDependency),
			metav1.ConditionFalse, v1alpha1.ReasonDependencyNotReady, true, false,
		},
		{"secret", controller.ErrSecret, metav1.ConditionFalse, v1alpha1.ReasonSecretUnavailable, true, false},
		{"zitadel", errBoom, metav1.ConditionFalse, v1alpha1.ReasonZitadelError, false, true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			obj := &v1alpha1.Project{}
			obj.Generation = 3
			res, err := controller.Classify(obj, tc.err)
			if (err != nil) != tc.wantErr {
				t.Fatalf("err = %v, wantErr %v", err, tc.wantErr)
			}
			if (res.RequeueAfter > 0) != tc.requeue {
				t.Fatalf("requeue = %v, want %v", res.RequeueAfter, tc.requeue)
			}
			c := obj.Status.Conditions[0]
			if c.Type != v1alpha1.ConditionReady || c.Status != tc.ready || c.Reason != tc.reason ||
				c.ObservedGeneration != 3 {
				t.Fatalf("condition = %+v", c)
			}
		})
	}
}

func TestOIDCConfigRoundTrip(t *testing.T) {
	t.Parallel()
	spec := v1alpha1.OIDCApplicationSpec{
		RedirectURIs:           []string{"https://a/cb"},
		PostLogoutRedirectURIs: []string{"https://a/"},
		AppType:                v1alpha1.OIDCAppTypeNative,
		AuthMethod:             v1alpha1.OIDCAuthMethodNone,
		ResponseTypes:          []v1alpha1.OIDCResponseType{v1alpha1.OIDCResponseTypeCode},
		GrantTypes: []v1alpha1.OIDCGrantType{
			v1alpha1.OIDCGrantTypeAuthorizationCode,
			v1alpha1.OIDCGrantTypeRefreshToken,
		},
		AccessTokenType:          v1alpha1.OIDCTokenTypeJWT,
		IDTokenRoleAssertion:     true,
		SkipNativeAppSuccessPage: true,
	}
	want := controller.OIDCConfig(spec)
	if want.GetApplicationType() != applicationv2.OIDCApplicationType_OIDC_APP_TYPE_NATIVE ||
		want.GetAuthMethodType() != applicationv2.OIDCAuthMethodType_OIDC_AUTH_METHOD_TYPE_NONE ||
		want.GetAccessTokenType() != applicationv2.OIDCTokenType_OIDC_TOKEN_TYPE_JWT ||
		want.GetGrantTypes()[1] != applicationv2.OIDCGrantType_OIDC_GRANT_TYPE_REFRESH_TOKEN {
		t.Fatalf("enum mapping wrong: %+v", want)
	}
	got := &applicationv2.OIDCConfiguration{
		RedirectUris: spec.RedirectURIs, PostLogoutRedirectUris: spec.PostLogoutRedirectURIs,
		ResponseTypes: want.GetResponseTypes(), GrantTypes: want.GetGrantTypes(),
		ApplicationType: want.GetApplicationType(), AuthMethodType: want.GetAuthMethodType(),
		AccessTokenType: want.GetAccessTokenType(), IdTokenRoleAssertion: true, SkipNativeAppSuccessPage: true,
	}
	if !controller.OIDCEqual(want, got) {
		t.Fatal("identical config reported as different")
	}
	got.RedirectUris = []string{"https://b/cb"}
	if controller.OIDCEqual(want, got) {
		t.Fatal("changed redirect uri not detected")
	}
}

func TestSameSet(t *testing.T) {
	t.Parallel()
	if !controller.SameSet([]string{"a", "b"}, []string{"b", "a"}) ||
		controller.SameSet([]string{"a"}, []string{"a", "b"}) {
		t.Fatal("sameSet wrong")
	}
}

func TestSyncSet(t *testing.T) {
	t.Parallel()
	var added, removed []string
	err := controller.SyncSet(t.Context(), "x", []string{"a", "b"}, []string{"b", "c"},
		func(_ context.Context, s string) error { added = append(added, s); return nil },
		func(_ context.Context, s string) error { removed = append(removed, s); return nil },
	)
	if err != nil || !slices.Equal(added, []string{"a"}) || !slices.Equal(removed, []string{"c"}) {
		t.Fatalf("added %v removed %v err %v", added, removed, err)
	}
}

func TestFlowIDs(t *testing.T) {
	t.Parallel()
	flow, trigger := controller.FlowIDs(v1alpha1.ActionTrigger{
		FlowType: v1alpha1.FlowCustomiseToken, TriggerType: v1alpha1.TriggerPreAccessTokenCreation,
	})
	if flow != "2" || trigger != "5" {
		t.Fatalf("got %s/%s", flow, trigger)
	}
}

func TestRotateRequested(t *testing.T) {
	t.Parallel()
	app := &v1alpha1.OIDCApplication{}
	if _, rotate := controller.RotateRequested(app, ""); rotate {
		t.Fatal("no annotation must not rotate")
	}
	app.Annotations = map[string]string{v1alpha1.AnnotationRotate: "v1"}
	if v, rotate := controller.RotateRequested(app, ""); !rotate || v != "v1" {
		t.Fatal("new annotation must rotate")
	}
	if _, rotate := controller.RotateRequested(app, "v1"); rotate {
		t.Fatal("already rotated for this value must not rotate again")
	}
}

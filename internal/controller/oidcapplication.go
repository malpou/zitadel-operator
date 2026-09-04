package controller

import (
	"context"
	"errors"
	"fmt"
	"slices"

	applicationv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/application/v2"
	filterv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/filter/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/malpou/zitadel-operator/api/v1alpha1"
)

var errNotOIDC = errors.New("application exists but is not an OIDC application")

type oidcApplicationSyncer struct {
	kube client.Client
	z    Clients
}

func (s *oidcApplicationSyncer) sync(ctx context.Context, app *v1alpha1.OIDCApplication) error {
	projectID, err := projectID(ctx, s.kube, app.Namespace, app.Spec.ProjectRef)
	if err != nil {
		return err
	}
	desired := oidcConfig(app.Spec)

	// The client secret only ever leaves Zitadel in the create response.
	var createdSecret string
	if err = ensureID(ctx, &app.Status.Status,
		func(ctx context.Context) (string, error) { return s.find(ctx, projectID, app.Spec.Name) },
		func(ctx context.Context) (string, error) {
			resp, cerr := s.z.Application.CreateApplication(ctx, &applicationv2.CreateApplicationRequest{
				ProjectId: projectID, Name: app.Spec.Name,
				ApplicationType: &applicationv2.CreateApplicationRequest_OidcConfiguration{OidcConfiguration: desired},
			})
			if cerr != nil {
				return "", fmt.Errorf("create application: %w", cerr)
			}
			createdSecret = resp.GetOidcConfiguration().GetClientSecret()

			return resp.GetApplicationId(), nil
		},
	); err != nil {
		return err
	}

	got, err := s.z.Application.GetApplication(ctx, &applicationv2.GetApplicationRequest{ApplicationId: app.Status.ID})
	if err != nil {
		if isNotFound(err) {
			app.Status.ID = ""

			return fmt.Errorf("application %q: %w", app.Spec.Name, errVanished)
		}

		return fmt.Errorf("get application: %w", err)
	}
	cur := got.GetApplication().GetOidcConfiguration()
	if cur == nil {
		return errNotOIDC
	}
	app.Status.ClientID = cur.GetClientId()
	if got.GetApplication().GetName() != app.Spec.Name || !oidcEqual(desired, cur) {
		if _, err = s.z.Application.UpdateApplication(
			ctx,
			updateOIDC(app.Status.ID, projectID, app.Spec, desired),
		); err != nil {
			return fmt.Errorf("update application: %w", err)
		}
	}

	return s.syncSecret(ctx, app, projectID, createdSecret)
}

func (s *oidcApplicationSyncer) remove(ctx context.Context, app *v1alpha1.OIDCApplication) error {
	projectID, err := projectID(ctx, s.kube, app.Namespace, app.Spec.ProjectRef)
	if err != nil {
		// Project CR gone or not ready: Zitadel cascades app deletion with the project.
		return nil //nolint:nilerr // a missing parent means the app is already gone or orphaned with it.
	}
	if _, err = s.z.Application.DeleteApplication(ctx, &applicationv2.DeleteApplicationRequest{
		ApplicationId: app.Status.ID, ProjectId: projectID,
	}); err != nil {
		return fmt.Errorf("delete application: %w", err)
	}

	return nil
}

func (s *oidcApplicationSyncer) find(ctx context.Context, projectID, name string) (string, error) {
	resp, err := s.z.Application.ListApplications(ctx, &applicationv2.ListApplicationsRequest{
		Filters: []*applicationv2.ApplicationSearchFilter{
			{Filter: &applicationv2.ApplicationSearchFilter_ProjectIdFilter{
				ProjectIdFilter: &applicationv2.ProjectIDFilter{ProjectId: projectID},
			}},
			{Filter: &applicationv2.ApplicationSearchFilter_NameFilter{NameFilter: &applicationv2.ApplicationNameFilter{
				Name: name, Method: filterv2.TextFilterMethod_TEXT_FILTER_METHOD_EQUALS,
			}}},
		},
	})
	if err != nil {
		return "", fmt.Errorf("list applications: %w", err)
	}
	if len(resp.GetApplications()) == 0 {
		return "", nil
	}

	return resp.GetApplications()[0].GetApplicationId(), nil
}

// syncSecret keeps the output Secret current: client-id always, client-secret
// from the create response or an explicit rotation.
func (s *oidcApplicationSyncer) syncSecret(
	ctx context.Context, app *v1alpha1.OIDCApplication, projectID, createdSecret string,
) error {
	if app.Spec.SecretRef == nil {
		return nil
	}
	data := map[string][]byte{keyClientID: []byte(app.Status.ClientID)}
	needsSecret := app.Spec.AuthMethod == v1alpha1.OIDCAuthMethodBasic
	rotateVal, rotate := rotateRequested(app, app.Status.RotatedFor)
	switch {
	case !needsSecret:
	case createdSecret != "":
		data[keyClientSecret] = []byte(createdSecret)
		app.Status.RotatedFor = rotateVal
	case rotate:
		resp, err := s.z.Application.GenerateClientSecret(ctx, &applicationv2.GenerateClientSecretRequest{
			ApplicationId: app.Status.ID, ProjectId: projectID,
		})
		if err != nil {
			return fmt.Errorf("generate client secret: %w", err)
		}
		data[keyClientSecret] = []byte(resp.GetClientSecret())
		app.Status.RotatedFor = rotateVal
	}
	if err := writeSecret(ctx, s.kube, *app.Spec.SecretRef, app.Namespace, data); err != nil {
		return err
	}
	if !needsSecret {
		return nil
	}
	has, err := secretHasKey(ctx, s.kube, *app.Spec.SecretRef, app.Namespace, keyClientSecret)
	if err != nil {
		return err
	}
	if !has {
		return errSecretUnavailable
	}

	return nil
}

// oidcConfig maps the spec onto the create request. Enum names are validated
// by the CRD schema, so the proto value lookup cannot miss.
func oidcConfig(spec v1alpha1.OIDCApplicationSpec) *applicationv2.CreateOIDCApplicationRequest {
	rt := make([]applicationv2.OIDCResponseType, 0, len(spec.ResponseTypes))
	for _, t := range spec.ResponseTypes {
		rt = append(
			rt,
			applicationv2.OIDCResponseType(applicationv2.OIDCResponseType_value["OIDC_RESPONSE_TYPE_"+string(t)]),
		)
	}
	gt := make([]applicationv2.OIDCGrantType, 0, len(spec.GrantTypes))
	for _, t := range spec.GrantTypes {
		gt = append(gt, applicationv2.OIDCGrantType(applicationv2.OIDCGrantType_value["OIDC_GRANT_TYPE_"+string(t)]))
	}

	return &applicationv2.CreateOIDCApplicationRequest{
		RedirectUris:           spec.RedirectURIs,
		PostLogoutRedirectUris: spec.PostLogoutRedirectURIs,
		ResponseTypes:          rt,
		GrantTypes:             gt,
		ApplicationType: applicationv2.OIDCApplicationType(
			applicationv2.OIDCApplicationType_value["OIDC_APP_TYPE_"+string(spec.AppType)]),
		AuthMethodType: applicationv2.OIDCAuthMethodType(
			applicationv2.OIDCAuthMethodType_value["OIDC_AUTH_METHOD_TYPE_"+string(spec.AuthMethod)]),
		AccessTokenType: applicationv2.OIDCTokenType(
			applicationv2.OIDCTokenType_value["OIDC_TOKEN_TYPE_"+string(spec.AccessTokenType)]),
		AccessTokenRoleAssertion: spec.AccessTokenRoleAssertion,
		IdTokenRoleAssertion:     spec.IDTokenRoleAssertion,
		IdTokenUserinfoAssertion: spec.IDTokenUserinfoAssertion,
		SkipNativeAppSuccessPage: spec.SkipNativeAppSuccessPage,
	}
}

func oidcEqual(want *applicationv2.CreateOIDCApplicationRequest, got *applicationv2.OIDCConfiguration) bool {
	return slices.Equal(want.GetRedirectUris(), got.GetRedirectUris()) &&
		slices.Equal(want.GetPostLogoutRedirectUris(), got.GetPostLogoutRedirectUris()) &&
		slices.Equal(want.GetResponseTypes(), got.GetResponseTypes()) &&
		slices.Equal(want.GetGrantTypes(), got.GetGrantTypes()) &&
		want.GetApplicationType() == got.GetApplicationType() &&
		want.GetAuthMethodType() == got.GetAuthMethodType() &&
		want.GetAccessTokenType() == got.GetAccessTokenType() &&
		want.GetAccessTokenRoleAssertion() == got.GetAccessTokenRoleAssertion() &&
		want.GetIdTokenRoleAssertion() == got.GetIdTokenRoleAssertion() &&
		want.GetIdTokenUserinfoAssertion() == got.GetIdTokenUserinfoAssertion() &&
		want.GetSkipNativeAppSuccessPage() == got.GetSkipNativeAppSuccessPage()
}

func updateOIDC(
	id, projectID string, spec v1alpha1.OIDCApplicationSpec, want *applicationv2.CreateOIDCApplicationRequest,
) *applicationv2.UpdateApplicationRequest {
	return &applicationv2.UpdateApplicationRequest{
		ApplicationId: id, ProjectId: projectID, Name: spec.Name,
		ApplicationType: &applicationv2.UpdateApplicationRequest_OidcConfiguration{
			OidcConfiguration: &applicationv2.UpdateOIDCApplicationConfigurationRequest{
				RedirectUris:             want.GetRedirectUris(),
				PostLogoutRedirectUris:   want.GetPostLogoutRedirectUris(),
				ResponseTypes:            want.GetResponseTypes(),
				GrantTypes:               want.GetGrantTypes(),
				ApplicationType:          &want.ApplicationType,
				AuthMethodType:           &want.AuthMethodType,
				AccessTokenType:          &want.AccessTokenType,
				AccessTokenRoleAssertion: &want.AccessTokenRoleAssertion,
				IdTokenRoleAssertion:     &want.IdTokenRoleAssertion,
				IdTokenUserinfoAssertion: &want.IdTokenUserinfoAssertion,
				SkipNativeAppSuccessPage: &want.SkipNativeAppSuccessPage,
			},
		},
	}
}

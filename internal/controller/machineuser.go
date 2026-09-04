package controller

import (
	"context"
	"errors"
	"fmt"

	filterv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/filter/v2"
	permissionv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/internal_permission/v2"
	objectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/object/v2"
	userv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/user/v2"
	"google.golang.org/protobuf/types/known/timestamppb"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/malpou/zitadel-operator/api/v1alpha1"
)

const roleProjectOwner = "PROJECT_OWNER"

var errNotMachine = errors.New("user exists but is not a machine user")

type machineUserSyncer struct {
	kube client.Client
	z    Clients
}

func (s *machineUserSyncer) sync(ctx context.Context, u *v1alpha1.MachineUser) error {
	if err := ensureID(ctx, &u.Status.Status,
		func(ctx context.Context) (string, error) { return s.find(ctx, u.Spec.UserName) },
		func(ctx context.Context) (string, error) { return s.create(ctx, u.Spec) },
	); err != nil {
		return err
	}

	got, err := s.z.User.GetUserByID(ctx, &userv2.GetUserByIDRequest{UserId: u.Status.ID})
	if err != nil {
		if isNotFound(err) {
			u.Status.ID = ""

			return fmt.Errorf("user %q: %w", u.Spec.UserName, errVanished)
		}

		return fmt.Errorf("get user: %w", err)
	}
	m := got.GetUser().GetMachine()
	if m == nil {
		return errNotMachine
	}
	tokenType := accessTokenType(u.Spec.AccessTokenType)
	if got.GetUser().GetUsername() != u.Spec.UserName || m.GetName() != u.Spec.Name ||
		m.GetDescription() != u.Spec.Description || m.GetAccessTokenType() != tokenType {
		if _, err = s.z.User.UpdateUser(ctx, &userv2.UpdateUserRequest{
			UserId: u.Status.ID, Username: &u.Spec.UserName,
			UserType: &userv2.UpdateUserRequest_Machine_{Machine: &userv2.UpdateUserRequest_Machine{
				Name: &u.Spec.Name, Description: &u.Spec.Description, AccessTokenType: &tokenType,
			}},
		}); err != nil {
			return fmt.Errorf("update user: %w", err)
		}
	}
	if err = s.syncMemberships(ctx, u); err != nil {
		return err
	}

	return s.syncPAT(ctx, u)
}

func (s *machineUserSyncer) remove(ctx context.Context, u *v1alpha1.MachineUser) error {
	// Zitadel cascades PATs and memberships with the user.
	if _, err := s.z.User.DeleteUser(ctx, &userv2.DeleteUserRequest{UserId: u.Status.ID}); err != nil {
		return fmt.Errorf("delete user: %w", err)
	}

	return nil
}

func (s *machineUserSyncer) find(ctx context.Context, username string) (string, error) {
	resp, err := s.z.User.ListUsers(ctx, &userv2.ListUsersRequest{Queries: []*userv2.SearchQuery{
		{Query: &userv2.SearchQuery_UserNameQuery{UserNameQuery: &userv2.UserNameQuery{
			UserName: username, Method: objectv2.TextQueryMethod_TEXT_QUERY_METHOD_EQUALS,
		}}},
	}})
	if err != nil {
		return "", fmt.Errorf("list users: %w", err)
	}
	if len(resp.GetResult()) == 0 {
		return "", nil
	}

	return resp.GetResult()[0].GetUserId(), nil
}

func (s *machineUserSyncer) create(ctx context.Context, spec v1alpha1.MachineUserSpec) (string, error) {
	resp, err := s.z.User.CreateUser(ctx, &userv2.CreateUserRequest{
		OrganizationId: s.z.OrgID,
		Username:       &spec.UserName,
		UserType: &userv2.CreateUserRequest_Machine_{Machine: &userv2.CreateUserRequest_Machine{
			Name: spec.Name, Description: &spec.Description, AccessTokenType: accessTokenType(spec.AccessTokenType),
		}},
	})
	if err != nil {
		return "", fmt.Errorf("create user: %w", err)
	}

	return resp.GetId(), nil
}

// syncMemberships makes the user PROJECT_OWNER of exactly spec.projectOwnerOf.
func (s *machineUserSyncer) syncMemberships(ctx context.Context, u *v1alpha1.MachineUser) error {
	want := map[string]bool{}
	for _, ref := range u.Spec.ProjectOwnerOf {
		id, err := projectID(ctx, s.kube, u.Namespace, ref)
		if err != nil {
			return err
		}
		want[id] = true
	}
	resp, err := s.z.Permission.ListAdministrators(ctx, &permissionv2.ListAdministratorsRequest{
		Filters: []*permissionv2.AdministratorSearchFilter{
			{Filter: &permissionv2.AdministratorSearchFilter_InUserIdsFilter{
				InUserIdsFilter: &filterv2.InIDsFilter{Ids: []string{u.Status.ID}},
			}},
		},
	})
	if err != nil {
		return fmt.Errorf("list administrators: %w", err)
	}
	for _, a := range resp.GetAdministrators() {
		pid := a.GetProject().GetId()
		if pid == "" {
			continue
		}
		if want[pid] {
			delete(want, pid)

			continue
		}
		if _, err = s.z.Permission.DeleteAdministrator(ctx, &permissionv2.DeleteAdministratorRequest{
			UserId: u.Status.ID, Resource: projectResource(pid),
		}); err != nil {
			return fmt.Errorf("delete administrator on project %s: %w", pid, err)
		}
	}
	for pid := range want {
		if _, err = s.z.Permission.CreateAdministrator(ctx, &permissionv2.CreateAdministratorRequest{
			UserId: u.Status.ID, Resource: projectResource(pid), Roles: []string{roleProjectOwner},
		}); err != nil {
			return fmt.Errorf("create administrator on project %s: %w", pid, err)
		}
	}

	return nil
}

// syncPAT keeps exactly one token on the user. The token value only leaves
// Zitadel when minted, so adoption of a foreign token ends in
// SecretUnavailable until a rotation replaces it. A changed expiration is
// picked up by the next rotation, not by itself.
func (s *machineUserSyncer) syncPAT(ctx context.Context, u *v1alpha1.MachineUser) error {
	if u.Spec.PAT == nil {
		return nil
	}
	resp, err := s.z.User.ListPersonalAccessTokens(ctx, &userv2.ListPersonalAccessTokensRequest{
		Filters: []*userv2.PersonalAccessTokensSearchFilter{
			{Filter: &userv2.PersonalAccessTokensSearchFilter_UserIdFilter{
				UserIdFilter: &filterv2.IDFilter{Id: u.Status.ID},
			}},
		},
	})
	if err != nil {
		return fmt.Errorf("list personal access tokens: %w", err)
	}
	tokens := resp.GetResult()
	rotateVal, rotate := rotateRequested(u, u.Status.RotatedFor)
	if rotate || len(tokens) == 0 {
		var minted *userv2.AddPersonalAccessTokenResponse
		minted, err = s.z.User.AddPersonalAccessToken(ctx, &userv2.AddPersonalAccessTokenRequest{
			UserId: u.Status.ID, ExpirationDate: timestamppb.New(u.Spec.PAT.Expiration.Time),
		})
		if err != nil {
			return fmt.Errorf("add personal access token: %w", err)
		}
		if err = writeSecret(ctx, s.kube, u.Spec.PAT.SecretRef, u.Namespace,
			map[string][]byte{keyToken: []byte(minted.GetToken())}); err != nil {
			return err
		}
		for _, t := range tokens {
			if _, err = s.z.User.RemovePersonalAccessToken(ctx, &userv2.RemovePersonalAccessTokenRequest{
				UserId: u.Status.ID, TokenId: t.GetId(),
			}); err != nil && !isNotFound(err) {
				return fmt.Errorf("remove old personal access token: %w", err)
			}
		}
		u.Status.PATID = minted.GetTokenId()
		u.Status.RotatedFor = rotateVal

		return nil
	}
	if u.Status.PATID == "" && len(tokens) == 1 {
		u.Status.PATID = tokens[0].GetId()
	}
	has, err := secretHasKey(ctx, s.kube, u.Spec.PAT.SecretRef, u.Namespace, keyToken)
	if err != nil {
		return err
	}
	if !has {
		return errSecretUnavailable
	}

	return nil
}

func projectResource(projectID string) *permissionv2.ResourceType {
	return &permissionv2.ResourceType{Resource: &permissionv2.ResourceType_ProjectId{ProjectId: projectID}}
}

func accessTokenType(t v1alpha1.OIDCTokenType) userv2.AccessTokenType {
	return userv2.AccessTokenType(userv2.AccessTokenType_value["ACCESS_TOKEN_TYPE_"+string(t)])
}

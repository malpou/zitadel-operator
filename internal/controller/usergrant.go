package controller

import (
	"context"
	"fmt"
	"slices"

	authorizationv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/authorization/v2"
	filterv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/filter/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/malpou/zitadel-operator/api/v1alpha1"
)

type userGrantSyncer struct {
	kube client.Client
	z    Clients
}

func (s *userGrantSyncer) sync(ctx context.Context, g *v1alpha1.UserGrant) error {
	projectID, err := projectID(ctx, s.kube, g.Namespace, g.Spec.ProjectRef)
	if err != nil {
		return err
	}
	if err = ensureID(ctx, &g.Status,
		func(ctx context.Context) (string, error) {
			a, ferr := s.findOne(
				ctx,
				&authorizationv2.AuthorizationsSearchFilter{
					Filter: &authorizationv2.AuthorizationsSearchFilter_InUserIds{
						InUserIds: &filterv2.InIDsFilter{Ids: []string{g.Spec.UserID}},
					},
				},
				&authorizationv2.AuthorizationsSearchFilter{
					Filter: &authorizationv2.AuthorizationsSearchFilter_ProjectId{
						ProjectId: &filterv2.IDFilter{Id: projectID},
					},
				},
			)

			return a.GetId(), ferr
		},
		func(ctx context.Context) (string, error) {
			resp, cerr := s.z.Authorization.CreateAuthorization(ctx, &authorizationv2.CreateAuthorizationRequest{
				UserId: g.Spec.UserID, ProjectId: projectID, OrganizationId: s.z.OrgID, RoleKeys: g.Spec.RoleKeys,
			})
			if cerr != nil {
				return "", fmt.Errorf("create authorization: %w", cerr)
			}

			return resp.GetId(), nil
		},
	); err != nil {
		return err
	}

	cur, err := s.findOne(ctx, &authorizationv2.AuthorizationsSearchFilter{
		Filter: &authorizationv2.AuthorizationsSearchFilter_AuthorizationIds{
			AuthorizationIds: &filterv2.InIDsFilter{Ids: []string{g.Status.ID}},
		},
	})
	if err != nil {
		return err
	}
	if cur == nil {
		g.Status.ID = ""

		return fmt.Errorf("authorization for user %s: %w", g.Spec.UserID, errVanished)
	}
	have := make([]string, 0, len(cur.GetRoles()))
	for _, r := range cur.GetRoles() {
		have = append(have, r.GetKey())
	}
	if !sameSet(have, g.Spec.RoleKeys) {
		if _, err = s.z.Authorization.UpdateAuthorization(ctx, &authorizationv2.UpdateAuthorizationRequest{
			Id: g.Status.ID, RoleKeys: g.Spec.RoleKeys,
		}); err != nil {
			return fmt.Errorf("update authorization: %w", err)
		}
	}

	return nil
}

func (s *userGrantSyncer) remove(ctx context.Context, g *v1alpha1.UserGrant) error {
	if _, err := s.z.Authorization.DeleteAuthorization(ctx, &authorizationv2.DeleteAuthorizationRequest{
		Id: g.Status.ID,
	}); err != nil {
		return fmt.Errorf("delete authorization: %w", err)
	}

	return nil
}

func (s *userGrantSyncer) findOne(
	ctx context.Context, filters ...*authorizationv2.AuthorizationsSearchFilter,
) (*authorizationv2.Authorization, error) {
	resp, err := s.z.Authorization.ListAuthorizations(ctx, &authorizationv2.ListAuthorizationsRequest{Filters: filters})
	if err != nil {
		return nil, fmt.Errorf("list authorizations: %w", err)
	}
	if len(resp.GetAuthorizations()) == 0 {
		return nil, nil //nolint:nilnil // "not found" is a valid outcome for a lookup.
	}

	return resp.GetAuthorizations()[0], nil
}

func sameSet(a, b []string) bool {
	a, b = slices.Clone(a), slices.Clone(b)
	slices.Sort(a)
	slices.Sort(b)

	return slices.Equal(a, b)
}

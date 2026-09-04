package controller

import (
	"context"
	"fmt"

	filterv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/filter/v2"
	projectv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/project/v2"

	"github.com/malpou/zitadel-operator/api/v1alpha1"
)

type projectSyncer struct {
	z Clients
}

func (s *projectSyncer) sync(ctx context.Context, p *v1alpha1.Project) error {
	if err := ensureID(ctx, &p.Status,
		func(ctx context.Context) (string, error) { return s.find(ctx, p.Spec.Name) },
		func(ctx context.Context) (string, error) { return s.create(ctx, p.Spec) },
	); err != nil {
		return err
	}

	got, err := s.z.Project.GetProject(ctx, &projectv2.GetProjectRequest{ProjectId: p.Status.ID})
	if err != nil {
		if isNotFound(err) {
			p.Status.ID = ""

			return fmt.Errorf("project %q: %w", p.Spec.Name, errVanished)
		}

		return fmt.Errorf("get project: %w", err)
	}
	cur := got.GetProject()
	if cur.GetName() != p.Spec.Name ||
		cur.GetProjectRoleAssertion() != p.Spec.RoleAssertion ||
		cur.GetAuthorizationRequired() != p.Spec.AuthorizationRequired {
		if _, err = s.z.Project.UpdateProject(ctx, &projectv2.UpdateProjectRequest{
			ProjectId:             p.Status.ID,
			Name:                  &p.Spec.Name,
			ProjectRoleAssertion:  &p.Spec.RoleAssertion,
			AuthorizationRequired: &p.Spec.AuthorizationRequired,
		}); err != nil {
			return fmt.Errorf("update project: %w", err)
		}
	}

	return s.syncRoles(ctx, p.Status.ID, p.Spec.Roles)
}

func (s *projectSyncer) remove(ctx context.Context, p *v1alpha1.Project) error {
	if _, err := s.z.Project.DeleteProject(ctx, &projectv2.DeleteProjectRequest{ProjectId: p.Status.ID}); err != nil {
		return fmt.Errorf("delete project: %w", err)
	}

	return nil
}

func (s *projectSyncer) find(ctx context.Context, name string) (string, error) {
	resp, err := s.z.Project.ListProjects(ctx, &projectv2.ListProjectsRequest{Filters: []*projectv2.ProjectSearchFilter{
		{Filter: &projectv2.ProjectSearchFilter_ProjectNameFilter{ProjectNameFilter: &projectv2.ProjectNameFilter{
			ProjectName: name, Method: filterv2.TextFilterMethod_TEXT_FILTER_METHOD_EQUALS,
		}}},
		{
			Filter: &projectv2.ProjectSearchFilter_OrganizationIdFilter{
				OrganizationIdFilter: &projectv2.ProjectOrganizationIDFilter{
					OrganizationId: s.z.OrgID, Type: projectv2.ProjectOrganizationIDFilter_OWNED,
				},
			},
		},
	}})
	if err != nil {
		return "", fmt.Errorf("list projects: %w", err)
	}
	if len(resp.GetProjects()) == 0 {
		return "", nil
	}

	return resp.GetProjects()[0].GetProjectId(), nil
}

func (s *projectSyncer) create(ctx context.Context, spec v1alpha1.ProjectSpec) (string, error) {
	resp, err := s.z.Project.CreateProject(ctx, &projectv2.CreateProjectRequest{
		OrganizationId:        s.z.OrgID,
		Name:                  spec.Name,
		ProjectRoleAssertion:  spec.RoleAssertion,
		AuthorizationRequired: spec.AuthorizationRequired,
	})
	if err != nil {
		return "", fmt.Errorf("create project: %w", err)
	}

	return resp.GetProjectId(), nil
}

// syncRoles reconciles the role set: add missing, update changed, remove extra.
func (s *projectSyncer) syncRoles(ctx context.Context, projectID string, want []v1alpha1.ProjectRole) error {
	resp, err := s.z.Project.ListProjectRoles(ctx, &projectv2.ListProjectRolesRequest{ProjectId: projectID})
	if err != nil {
		return fmt.Errorf("list project roles: %w", err)
	}
	have := map[string]*projectv2.ProjectRole{}
	for _, r := range resp.GetProjectRoles() {
		have[r.GetKey()] = r
	}
	for _, w := range want {
		h, ok := have[w.Key]
		delete(have, w.Key)
		switch {
		case !ok:
			_, err = s.z.Project.AddProjectRole(ctx, &projectv2.AddProjectRoleRequest{
				ProjectId: projectID, RoleKey: w.Key, DisplayName: w.DisplayName, Group: &w.Group,
			})
		case h.GetDisplayName() != w.DisplayName || h.GetGroup() != w.Group:
			_, err = s.z.Project.UpdateProjectRole(ctx, &projectv2.UpdateProjectRoleRequest{
				ProjectId: projectID, RoleKey: w.Key, DisplayName: &w.DisplayName, Group: &w.Group,
			})
		}
		if err != nil {
			return fmt.Errorf("sync project role %q: %w", w.Key, err)
		}
	}
	for key := range have {
		if _, err = s.z.Project.RemoveProjectRole(ctx, &projectv2.RemoveProjectRoleRequest{
			ProjectId: projectID, RoleKey: key,
		}); err != nil {
			return fmt.Errorf("remove project role %q: %w", key, err)
		}
	}

	return nil
}

package controller_test

import (
	"context"
	"slices"
	"testing"

	permissionv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/internal_permission/v2"
	"google.golang.org/grpc"

	"github.com/malpou/zitadel-operator/internal/controller"
)

// fakePermission records administrator writes; ListAdministrators returns admins.
type fakePermission struct {
	permissionv2.InternalPermissionServiceClient

	admins []*permissionv2.Administrator
	calls  []string
	roles  []string
	org    string
}

func (f *fakePermission) ListAdministrators(
	context.Context, *permissionv2.ListAdministratorsRequest, ...grpc.CallOption,
) (*permissionv2.ListAdministratorsResponse, error) {
	return &permissionv2.ListAdministratorsResponse{Administrators: f.admins}, nil
}

func (f *fakePermission) CreateAdministrator(
	_ context.Context, in *permissionv2.CreateAdministratorRequest, _ ...grpc.CallOption,
) (*permissionv2.CreateAdministratorResponse, error) {
	f.calls = append(f.calls, "create")
	f.roles, f.org = in.GetRoles(), in.GetResource().GetOrganizationId()

	return &permissionv2.CreateAdministratorResponse{}, nil
}

func (f *fakePermission) UpdateAdministrator(
	_ context.Context, in *permissionv2.UpdateAdministratorRequest, _ ...grpc.CallOption,
) (*permissionv2.UpdateAdministratorResponse, error) {
	f.calls = append(f.calls, "update")
	f.roles, f.org = in.GetRoles(), in.GetResource().GetOrganizationId()

	return &permissionv2.UpdateAdministratorResponse{}, nil
}

func (f *fakePermission) DeleteAdministrator(
	_ context.Context, in *permissionv2.DeleteAdministratorRequest, _ ...grpc.CallOption,
) (*permissionv2.DeleteAdministratorResponse, error) {
	f.calls = append(f.calls, "delete")
	f.org = in.GetResource().GetOrganizationId()

	return &permissionv2.DeleteAdministratorResponse{}, nil
}

func orgAdmin(org string, roles ...string) *permissionv2.Administrator {
	return &permissionv2.Administrator{
		Resource: &permissionv2.Administrator_Organization{Organization: &permissionv2.Organization{Id: org}},
		Roles:    roles,
	}
}

func projectAdmin(project string, roles ...string) *permissionv2.Administrator {
	return &permissionv2.Administrator{
		Resource: &permissionv2.Administrator_Project{Project: &permissionv2.Project{Id: project}},
		Roles:    roles,
	}
}

func TestSyncOrgRoles(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		admins    []*permissionv2.Administrator
		want      []string
		calls     []string
		wantRoles []string
	}{
		{name: "nothing wanted, nothing held", admins: []*permissionv2.Administrator{projectAdmin("p", "PROJECT_OWNER")}},
		{
			name: "missing is created", admins: []*permissionv2.Administrator{projectAdmin("p", "PROJECT_OWNER")},
			want: []string{"ORG_USER_MANAGER"}, calls: []string{"create"}, wantRoles: []string{"ORG_USER_MANAGER"},
		},
		{
			name: "same set is left alone", admins: []*permissionv2.Administrator{orgAdmin("org", "B", "A")},
			want: []string{"A", "B"},
		},
		{
			name: "changed set is updated", admins: []*permissionv2.Administrator{orgAdmin("org", "A")},
			want: []string{"A", "B"}, calls: []string{"update"}, wantRoles: []string{"A", "B"},
		},
		{
			name: "empty want removes", admins: []*permissionv2.Administrator{orgAdmin("org", "A")},
			calls: []string{"delete"},
		},
		{
			name: "another org is ignored", admins: []*permissionv2.Administrator{orgAdmin("other", "ORG_OWNER")},
			want: []string{"A"}, calls: []string{"create"}, wantRoles: []string{"A"},
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			f := &fakePermission{admins: tc.admins}
			if err := controller.SyncOrgRoles(t.Context(), f, "org", "u", tc.want); err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(f.calls, tc.calls) || !slices.Equal(f.roles, tc.wantRoles) {
				t.Fatalf("calls %v roles %v, want %v %v", f.calls, f.roles, tc.calls, tc.wantRoles)
			}
			if len(f.calls) > 0 && f.org != "org" {
				t.Fatalf("wrote to org %q", f.org)
			}
		})
	}
}

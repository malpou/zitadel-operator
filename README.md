# zitadel-operator

A Kubernetes operator that manages the *content* of a [Zitadel](https://zitadel.com)
instance from custom resources: projects and roles, OIDC applications, user
grants, machine users with personal access tokens, the login policy, the SMTP
email provider, notification message texts, and actions v1 with their flow
triggers. Client ids, client secrets and tokens are written straight into
Kubernetes Secrets in the namespaces where your apps read them.

It deploys nothing; it configures an existing Zitadel through its API, the way
the Terraform provider does, but from CRDs reconciled continuously.

## Kinds (`zitadel-operator.io/v1alpha1`)

| Kind | Zitadel API | Scope |
|---|---|---|
| `Project` (roles inline) | project.v2 | organization |
| `OIDCApplication` | application.v2 | project |
| `UserGrant` | authorization.v2 | organization |
| `MachineUser` (PAT + PROJECT_OWNER memberships inline) | user.v2, internal_permission.v2 | organization |
| `LoginPolicy` | management v1 | organization (singleton) |
| `EmailProvider` | admin v1 | instance |
| `MessageText` | admin v1 | instance |
| `Action` (flow triggers inline) | management v1 | organization |

The v1-backed kinds are removed in Zitadel V5; each lives in its own
controller file so the swap to a v2 API is local.

## Conventions

- **Adopt by name.** The operator lists before it creates, so applying a CR
  for something that already exists takes it over instead of duplicating it.
  `EmailProvider` adopts by host and sender address.
- `status.id` holds the Zitadel id, `status.conditions[Ready]` the state.
- `spec.deletionPolicy: Delete|Orphan` on Project, OIDCApplication,
  UserGrant, MachineUser and Action. LoginPolicy, EmailProvider and
  MessageText carry no finalizer: deleting the CR never touches Zitadel.
- `spec.secretRef` writes `client-id` (always) and `client-secret` (auth
  method BASIC) for applications, and `token` for a MachineUser PAT. These
  values are only known at creation, so annotate the CR with
  `zitadel-operator.io/rotate: <any new value>` to regenerate and rewrite them.
- CRs referencing a `Project` (by CR name, same namespace) wait until that
  Project is Ready, so roles exist before grants use them.
- Every write to Zitadel is logged as `zitadel write` with the gRPC method.
  `DRY_RUN=true` refuses every write instead; run it first against a new set
  of CRs to prove they match what is already there.

## Install

The operator authenticates as a Zitadel service user with a machine key
(JWT profile). The user needs `IAM_OWNER` for the instance-level kinds.

```sh
kubectl create namespace zitadel-operator
kubectl -n zitadel-operator create secret generic zitadel-operator-key \
  --from-file=key.json=<machine key json>
kubectl -n zitadel-operator create configmap zitadel-operator \
  --from-literal=ZITADEL_DOMAIN=auth.example.com \
  --from-literal=ZITADEL_ORG_ID=<organization id>
kubectl apply -k deploy
```

| Env | Meaning |
|---|---|
| `ZITADEL_DOMAIN` | public hostname of the instance |
| `ZITADEL_KEY_PATH` | path of the machine key JSON |
| `ZITADEL_ORG_ID` | organization the org-scoped kinds live in |
| `WATCH_NAMESPACE` | namespace CRs are watched in (default `zitadel-operator`) |
| `DRY_RUN` | `true` logs and refuses every write |

## Example

```yaml
apiVersion: zitadel-operator.io/v1alpha1
kind: Project
metadata: {name: grafana, namespace: zitadel-operator}
spec:
  name: grafana
  roleAssertion: true
  authorizationRequired: true
  roles:
    - {key: admin, displayName: Grafana Admin, group: grafana}
---
apiVersion: zitadel-operator.io/v1alpha1
kind: OIDCApplication
metadata: {name: grafana, namespace: zitadel-operator}
spec:
  name: grafana
  projectRef: grafana
  redirectURIs: ["https://grafana.example.com/login/generic_oauth"]
  postLogoutRedirectURIs: ["https://grafana.example.com/"]
  idTokenRoleAssertion: true
  idTokenUserinfoAssertion: true
  secretRef: {name: grafana-oidc, namespace: monitoring}
```

## Development

Go 1.27, plain controller-runtime, `controller-gen` as a Go tool, strict
`golangci-lint` (`.golangci.yml`).

```sh
go generate ./...   # deepcopy + deploy/crds
go build ./... && go vet ./... && go test ./...
golangci-lint run
```

Run against a cluster and instance from your machine:

```sh
kubectl apply -f deploy/crds
ZITADEL_DOMAIN=auth.example.com ZITADEL_KEY_PATH=key.json ZITADEL_ORG_ID=<id> \
  go run ./cmd/zitadel-operator
```

## License

MIT, see `LICENSE`. `.golangci.yml` is based on
[maratori/golangci-lint-config](https://github.com/maratori/golangci-lint-config) (MIT).

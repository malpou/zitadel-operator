package controller

import (
	"context"
	"fmt"

	actionv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/action/v2"
	filterv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/filter/v2"
	"google.golang.org/protobuf/types/known/durationpb"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/malpou/zitadel-operator/api/v1alpha1"
)

const keySigningKey = "signing-key"

// targetSyncer manages an actions v2 target (instance-wide).
type targetSyncer struct {
	kube client.Client
	z    Clients
}

func (s *targetSyncer) sync(ctx context.Context, t *v1alpha1.Target) error {
	// A target created or adopted here has its key read and written out as
	// it is, so the rotate annotation it carries is spent, not a request.
	if t.Status.ID == "" {
		t.Status.RotatedFor = t.GetAnnotations()[v1alpha1.AnnotationRotate]
	}
	if err := ensureID(ctx, &t.Status.Status,
		func(ctx context.Context) (string, error) { return s.find(ctx, t.Spec.Name) },
		func(ctx context.Context) (string, error) {
			resp, err := s.z.Action.CreateTarget(ctx, createTarget(t.Spec))
			if err != nil {
				return "", fmt.Errorf("create target: %w", err)
			}

			return resp.GetId(), nil
		},
	); err != nil {
		return err
	}

	got, err := s.z.Action.GetTarget(ctx, &actionv2.GetTargetRequest{Id: t.Status.ID})
	if err != nil {
		if isNotFound(err) {
			t.Status.ID = ""

			return fmt.Errorf("target %q: %w", t.Spec.Name, errVanished)
		}

		return fmt.Errorf("get target: %w", err)
	}
	cur := got.GetTarget()
	if upd := updateTarget(t.Status.ID, t.Spec, cur); upd != nil {
		if _, err = s.z.Action.UpdateTarget(ctx, upd); err != nil {
			return fmt.Errorf("update target: %w", err)
		}
	}

	return s.syncSigningKey(ctx, t, cur.GetSigningKey())
}

func (s *targetSyncer) remove(ctx context.Context, t *v1alpha1.Target) error {
	if _, err := s.z.Action.DeleteTarget(ctx, &actionv2.DeleteTargetRequest{Id: t.Status.ID}); err != nil {
		return fmt.Errorf("delete target: %w", err)
	}

	return nil
}

func (s *targetSyncer) find(ctx context.Context, name string) (string, error) {
	resp, err := s.z.Action.ListTargets(ctx, &actionv2.ListTargetsRequest{Filters: []*actionv2.TargetSearchFilter{
		{Filter: &actionv2.TargetSearchFilter_TargetNameFilter{TargetNameFilter: &actionv2.TargetNameFilter{
			TargetName: name, Method: filterv2.TextFilterMethod_TEXT_FILTER_METHOD_EQUALS,
		}}},
	}})
	if err != nil {
		return "", fmt.Errorf("list targets: %w", err)
	}
	if len(resp.GetTargets()) == 0 {
		return "", nil
	}

	return resp.GetTargets()[0].GetId(), nil
}

// syncSigningKey writes the current signing key, which Zitadel returns on
// every read, or a new one when the rotate annotation asks for it.
func (s *targetSyncer) syncSigningKey(ctx context.Context, t *v1alpha1.Target, key string) error {
	if rotateVal, rotate := rotateRequested(t, t.Status.RotatedFor); rotate {
		// "0s": the old key stops at once, the only expiry Zitadel supports.
		resp, err := s.z.Action.UpdateTarget(ctx, &actionv2.UpdateTargetRequest{
			Id: t.Status.ID, ExpirationSigningKey: durationpb.New(0),
		})
		if err != nil {
			return fmt.Errorf("rotate target signing key: %w", err)
		}
		key = resp.GetSigningKey()
		t.Status.RotatedFor = rotateVal
	}
	if t.Spec.SigningKeySecretRef == nil {
		return nil
	}

	return writeSecret(ctx, s.kube, *t.Spec.SigningKeySecretRef, t.Namespace,
		map[string][]byte{keySigningKey: []byte(key)})
}

func payloadType(p v1alpha1.PayloadType) actionv2.PayloadType {
	return actionv2.PayloadType(actionv2.PayloadType_value["PAYLOAD_TYPE_"+string(p)])
}

func createTarget(spec v1alpha1.TargetSpec) *actionv2.CreateTargetRequest {
	req := &actionv2.CreateTargetRequest{
		Name: spec.Name, Endpoint: spec.Endpoint,
		Timeout: durationpb.New(spec.Timeout.Duration), PayloadType: payloadType(spec.PayloadType),
	}
	switch spec.Type {
	case v1alpha1.TargetTypeWebhook:
		req.TargetType = &actionv2.CreateTargetRequest_RestWebhook{
			RestWebhook: &actionv2.RESTWebhook{InterruptOnError: spec.InterruptOnError},
		}
	case v1alpha1.TargetTypeAsync:
		req.TargetType = &actionv2.CreateTargetRequest_RestAsync{RestAsync: &actionv2.RESTAsync{}}
	case v1alpha1.TargetTypeCall:
		req.TargetType = &actionv2.CreateTargetRequest_RestCall{
			RestCall: &actionv2.RESTCall{InterruptOnError: spec.InterruptOnError},
		}
	}

	return req
}

// updateTarget builds an update carrying only the fields that differ, or nil
// when the target matches the spec. The name goes in only when it changes,
// as for applications.
func updateTarget(id string, spec v1alpha1.TargetSpec, cur *actionv2.Target) *actionv2.UpdateTargetRequest {
	upd := &actionv2.UpdateTargetRequest{Id: id}
	changed := false
	if cur.GetName() != spec.Name {
		upd.Name, changed = &spec.Name, true
	}
	if cur.GetEndpoint() != spec.Endpoint {
		upd.Endpoint, changed = &spec.Endpoint, true
	}
	if cur.GetTimeout().AsDuration() != spec.Timeout.Duration {
		upd.Timeout, changed = durationpb.New(spec.Timeout.Duration), true
	}
	if want := payloadType(spec.PayloadType); cur.GetPayloadType() != want {
		upd.PayloadType, changed = want, true
	}
	if !sameTargetType(spec, cur) {
		switch tt := createTarget(spec).GetTargetType().(type) {
		case *actionv2.CreateTargetRequest_RestWebhook:
			upd.TargetType = &actionv2.UpdateTargetRequest_RestWebhook{RestWebhook: tt.RestWebhook}
		case *actionv2.CreateTargetRequest_RestCall:
			upd.TargetType = &actionv2.UpdateTargetRequest_RestCall{RestCall: tt.RestCall}
		case *actionv2.CreateTargetRequest_RestAsync:
			upd.TargetType = &actionv2.UpdateTargetRequest_RestAsync{RestAsync: tt.RestAsync}
		}
		changed = true
	}
	if !changed {
		return nil
	}

	return upd
}

func sameTargetType(spec v1alpha1.TargetSpec, cur *actionv2.Target) bool {
	switch spec.Type {
	case v1alpha1.TargetTypeWebhook:
		return cur.GetRestWebhook() != nil && cur.GetRestWebhook().GetInterruptOnError() == spec.InterruptOnError
	case v1alpha1.TargetTypeAsync:
		return cur.GetRestAsync() != nil
	case v1alpha1.TargetTypeCall:
		return cur.GetRestCall() != nil && cur.GetRestCall().GetInterruptOnError() == spec.InterruptOnError
	}

	return false
}

package controller_test

import (
	"testing"
	"time"

	actionv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/action/v2"
	"google.golang.org/protobuf/types/known/durationpb"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"github.com/malpou/zitadel-operator/api/v1alpha1"
	"github.com/malpou/zitadel-operator/internal/controller"
)

func claimsSpec() v1alpha1.TargetSpec {
	return v1alpha1.TargetSpec{
		Name: "claims", Endpoint: "http://api/claims", Type: v1alpha1.TargetTypeCall,
		Timeout: metav1.Duration{Duration: 3 * time.Second}, PayloadType: v1alpha1.PayloadTypeJSON,
	}
}

// asTarget is what Zitadel reads back for a target created from spec.
func asTarget(spec v1alpha1.TargetSpec) *actionv2.Target {
	req := controller.CreateTarget(spec)
	t := &actionv2.Target{
		Name: req.GetName(), Endpoint: req.GetEndpoint(), Timeout: req.GetTimeout(), PayloadType: req.GetPayloadType(),
	}
	switch tt := req.GetTargetType().(type) {
	case *actionv2.CreateTargetRequest_RestCall:
		t.TargetType = &actionv2.Target_RestCall{RestCall: tt.RestCall}
	case *actionv2.CreateTargetRequest_RestWebhook:
		t.TargetType = &actionv2.Target_RestWebhook{RestWebhook: tt.RestWebhook}
	case *actionv2.CreateTargetRequest_RestAsync:
		t.TargetType = &actionv2.Target_RestAsync{RestAsync: tt.RestAsync}
	}

	return t
}

func TestCreateTargetMapping(t *testing.T) {
	t.Parallel()
	spec := claimsSpec()
	req := controller.CreateTarget(spec)
	if req.GetRestCall() == nil || req.GetRestCall().GetInterruptOnError() ||
		req.GetPayloadType() != actionv2.PayloadType_PAYLOAD_TYPE_JSON ||
		req.GetTimeout().AsDuration() != 3*time.Second {
		t.Fatalf("create request wrong: %+v", req)
	}
	spec.Type, spec.InterruptOnError = v1alpha1.TargetTypeWebhook, true
	if !controller.CreateTarget(spec).GetRestWebhook().GetInterruptOnError() {
		t.Fatal("webhook interruptOnError lost")
	}
}

// TestUpdateTargetOnlyWhatDiffers: a matching target needs no update, and an
// update carries only the changed fields, never an unchanged name.
func TestUpdateTargetOnlyWhatDiffers(t *testing.T) {
	t.Parallel()
	spec := claimsSpec()
	if upd := controller.UpdateTarget("id", spec, asTarget(spec)); upd != nil {
		t.Fatalf("matching target got an update: %+v", upd)
	}

	cur := asTarget(spec)
	cur.Endpoint = "http://old/claims"
	upd := controller.UpdateTarget("id", spec, cur)
	if upd.GetEndpoint() != spec.Endpoint || upd.Name != nil || upd.GetTimeout() != nil ||
		upd.GetTargetType() != nil || upd.GetPayloadType() != actionv2.PayloadType_PAYLOAD_TYPE_UNSPECIFIED {
		t.Fatalf("endpoint change: %+v", upd)
	}

	cur = asTarget(spec)
	cur.Timeout = durationpb.New(10 * time.Second)
	if upd = controller.UpdateTarget("id", spec, cur); upd.GetTimeout().AsDuration() != 3*time.Second {
		t.Fatalf("timeout change: %+v", upd)
	}

	interrupting := spec
	interrupting.InterruptOnError = true
	upd = controller.UpdateTarget("id", spec, asTarget(interrupting))
	if upd.GetRestCall() == nil || upd.GetRestCall().GetInterruptOnError() {
		t.Fatalf("interruptOnError change: %+v", upd)
	}

	webhook := spec
	webhook.Type = v1alpha1.TargetTypeWebhook
	if upd = controller.UpdateTarget("id", spec, asTarget(webhook)); upd.GetRestCall() == nil {
		t.Fatalf("type change: %+v", upd)
	}

	renamed := asTarget(spec)
	renamed.Name = "old"
	if upd = controller.UpdateTarget("id", spec, renamed); upd.GetName() != "claims" {
		t.Fatalf("rename: %+v", upd)
	}
}

package controller

import (
	"context"
	"fmt"
	"slices"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/action"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/management"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/object"
	"google.golang.org/protobuf/types/known/durationpb"

	"github.com/malpou/zitadel-operator/api/v1alpha1"
)

// actionSyncer manages an actions v1 script and its flow triggers
// (management v1, gone in Zitadel V5).
type actionSyncer struct {
	z Clients
}

func (s *actionSyncer) sync(ctx context.Context, a *v1alpha1.Action) error {
	if err := ensureID(ctx, &a.Status,
		func(ctx context.Context) (string, error) { return s.find(ctx, a.Spec.Name) },
		func(ctx context.Context) (string, error) {
			resp, err := s.z.Management.CreateAction(ctx, &management.CreateActionRequest{
				Name: a.Spec.Name, Script: a.Spec.Script,
				Timeout: durationpb.New(a.Spec.Timeout.Duration), AllowedToFail: a.Spec.AllowedToFail,
			})
			if err != nil {
				return "", fmt.Errorf("create action: %w", err)
			}

			return resp.GetId(), nil
		},
	); err != nil {
		return err
	}

	got, err := s.z.Management.GetAction(ctx, &management.GetActionRequest{Id: a.Status.ID})
	if err != nil {
		if isNotFound(err) {
			a.Status.ID = ""

			return fmt.Errorf("action %q: %w", a.Spec.Name, errVanished)
		}

		return fmt.Errorf("get action: %w", err)
	}
	cur := got.GetAction()
	if cur.GetName() != a.Spec.Name || cur.GetScript() != a.Spec.Script ||
		cur.GetTimeout().AsDuration() != a.Spec.Timeout.Duration || cur.GetAllowedToFail() != a.Spec.AllowedToFail {
		if _, err = s.z.Management.UpdateAction(ctx, &management.UpdateActionRequest{
			Id: a.Status.ID, Name: a.Spec.Name, Script: a.Spec.Script,
			Timeout: durationpb.New(a.Spec.Timeout.Duration), AllowedToFail: a.Spec.AllowedToFail,
		}); err != nil {
			return fmt.Errorf("update action: %w", err)
		}
	}

	return s.syncTriggers(ctx, a)
}

func (s *actionSyncer) remove(ctx context.Context, a *v1alpha1.Action) error {
	if _, err := s.z.Management.DeleteAction(ctx, &management.DeleteActionRequest{Id: a.Status.ID}); err != nil {
		return fmt.Errorf("delete action: %w", err)
	}

	return nil
}

func (s *actionSyncer) find(ctx context.Context, name string) (string, error) {
	resp, err := s.z.Management.ListActions(ctx, &management.ListActionsRequest{Queries: []*management.ActionQuery{
		{Query: &management.ActionQuery_ActionNameQuery{ActionNameQuery: &action.ActionNameQuery{
			Name: name, Method: object.TextQueryMethod_TEXT_QUERY_METHOD_EQUALS,
		}}},
	}})
	if err != nil {
		return "", fmt.Errorf("list actions: %w", err)
	}
	if len(resp.GetResult()) == 0 {
		return "", nil
	}

	return resp.GetResult()[0].GetId(), nil
}

// syncTriggers makes this action the whole action list of every (flow,
// trigger) pair in the spec. SetTriggerActions replaces the list, so the CR
// owns those pairs.
func (s *actionSyncer) syncTriggers(ctx context.Context, a *v1alpha1.Action) error {
	for _, t := range a.Spec.Triggers {
		flowID, triggerID := flowIDs(t)
		flow, err := s.z.Management.GetFlow(ctx, &management.GetFlowRequest{Type: flowID})
		if err != nil {
			return fmt.Errorf("get flow %s: %w", t.FlowType, err)
		}
		var have []string
		for _, ta := range flow.GetFlow().GetTriggerActions() {
			if ta.GetTriggerType().GetId() != triggerID {
				continue
			}
			for _, act := range ta.GetActions() {
				have = append(have, act.GetId())
			}
		}
		if slices.Equal(have, []string{a.Status.ID}) {
			continue
		}
		if _, err = s.z.Management.SetTriggerActions(ctx, &management.SetTriggerActionsRequest{
			FlowType: flowID, TriggerType: triggerID, ActionIds: []string{a.Status.ID},
		}); err != nil {
			return fmt.Errorf("set trigger actions %s/%s: %w", t.FlowType, t.TriggerType, err)
		}
	}

	return nil
}

// flowIDs maps the CRD names onto the numeric ids the v1 flow API takes.
func flowIDs(t v1alpha1.ActionTrigger) (string, string) {
	var flow, trigger string
	if t.FlowType == v1alpha1.FlowCustomiseToken {
		flow = "2"
	}
	switch t.TriggerType {
	case v1alpha1.TriggerPreUserinfoCreation:
		trigger = "4"
	case v1alpha1.TriggerPreAccessTokenCreation:
		trigger = "5"
	}

	return flow, trigger
}

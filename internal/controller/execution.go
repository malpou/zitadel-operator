package controller

import (
	"context"
	"fmt"
	"slices"

	actionv2 "github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/action/v2"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/malpou/zitadel-operator/api/v1alpha1"
)

// executionSyncer manages an actions v2 execution (instance-wide). An
// execution has no id of its own: it is the target list of one condition,
// so status.id holds the condition and the CR owns that list.
type executionSyncer struct {
	kube client.Client
	z    Clients
}

func (s *executionSyncer) sync(ctx context.Context, e *v1alpha1.Execution) error {
	want := make([]string, 0, len(e.Spec.TargetRefs))
	for _, ref := range e.Spec.TargetRefs {
		id, err := readyID(ctx, s.kube, &v1alpha1.Target{}, "target", e.Namespace, ref)
		if err != nil {
			return err
		}
		want = append(want, id)
	}
	cond := executionCondition(e.Spec.Condition)
	resp, err := s.z.Action.ListExecutions(ctx, &actionv2.ListExecutionsRequest{
		Filters: []*actionv2.ExecutionSearchFilter{{Filter: &actionv2.ExecutionSearchFilter_InConditionsFilter{
			InConditionsFilter: &actionv2.InConditionsFilter{Conditions: []*actionv2.Condition{cond}},
		}}},
	})
	if err != nil {
		return fmt.Errorf("list executions: %w", err)
	}
	var have []string
	if execs := resp.GetExecutions(); len(execs) > 0 {
		have = execs[0].GetTargets()
	}
	if !slices.Equal(have, want) {
		if _, err = s.z.Action.SetExecution(ctx, &actionv2.SetExecutionRequest{
			Condition: cond, Targets: want,
		}); err != nil {
			return fmt.Errorf("set execution %s: %w", executionID(e.Spec.Condition), err)
		}
	}
	e.Status.ID = executionID(e.Spec.Condition)

	return nil
}

// remove empties the condition's target list, which is how Zitadel deletes
// an execution.
func (s *executionSyncer) remove(ctx context.Context, e *v1alpha1.Execution) error {
	if _, err := s.z.Action.SetExecution(ctx, &actionv2.SetExecutionRequest{
		Condition: executionCondition(e.Spec.Condition),
	}); err != nil {
		return fmt.Errorf("delete execution %s: %w", executionID(e.Spec.Condition), err)
	}

	return nil
}

func executionCondition(c v1alpha1.ExecutionCondition) *actionv2.Condition {
	return &actionv2.Condition{ConditionType: &actionv2.Condition_Function{
		Function: &actionv2.FunctionExecution{Name: c.Function},
	}}
}

func executionID(c v1alpha1.ExecutionCondition) string {
	return "function/" + c.Function
}

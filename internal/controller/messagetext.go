package controller

import (
	"context"
	"fmt"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/text"

	"github.com/malpou/zitadel-operator/api/v1alpha1"
)

// messageTextSyncer manages instance-level custom message texts (admin v1,
// gone in Zitadel V5). Each type has its own identically shaped request
// pair, hence the adapters below.
type messageTextSyncer struct {
	z Clients
}

func (s *messageTextSyncer) sync(ctx context.Context, t *v1alpha1.MessageText) error {
	cur, err := s.get(ctx, t.Spec.Type, t.Spec.Language)
	if err != nil {
		return fmt.Errorf("get custom %s text: %w", t.Spec.Type, err)
	}
	if !cur.GetIsDefault() && messageTextEqual(cur, t.Spec) {
		return nil
	}
	if err = s.set(ctx, t.Spec); err != nil {
		return fmt.Errorf("set %s text: %w", t.Spec.Type, err)
	}

	return nil
}

func (*messageTextSyncer) remove(context.Context, *v1alpha1.MessageText) error { return nil }

func (s *messageTextSyncer) get(
	ctx context.Context, typ v1alpha1.MessageTextType, lang string,
) (*text.MessageCustomText, error) {
	switch typ {
	case v1alpha1.MessageTextVerifyEmail:
		r, err := s.z.Admin.GetCustomVerifyEmailMessageText(
			ctx,
			&admin.GetCustomVerifyEmailMessageTextRequest{Language: lang},
		)

		return r.GetCustomText(), err
	case v1alpha1.MessageTextPasswordReset:
		r, err := s.z.Admin.GetCustomPasswordResetMessageText(ctx,
			&admin.GetCustomPasswordResetMessageTextRequest{Language: lang})

		return r.GetCustomText(), err
	case v1alpha1.MessageTextPasswordChange:
		r, err := s.z.Admin.GetCustomPasswordChangeMessageText(ctx,
			&admin.GetCustomPasswordChangeMessageTextRequest{Language: lang})

		return r.GetCustomText(), err
	}

	return nil, fmt.Errorf("unsupported message text type %q", typ)
}

func (s *messageTextSyncer) set(ctx context.Context, spec v1alpha1.MessageTextSpec) error {
	var err error
	switch spec.Type {
	case v1alpha1.MessageTextVerifyEmail:
		_, err = s.z.Admin.SetDefaultVerifyEmailMessageText(ctx, &admin.SetDefaultVerifyEmailMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader, Subject: spec.Subject,
			Greeting: spec.Greeting, Text: spec.Text, ButtonText: spec.ButtonText,
		})
	case v1alpha1.MessageTextPasswordReset:
		_, err = s.z.Admin.SetDefaultPasswordResetMessageText(ctx, &admin.SetDefaultPasswordResetMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader, Subject: spec.Subject,
			Greeting: spec.Greeting, Text: spec.Text, ButtonText: spec.ButtonText,
		})
	case v1alpha1.MessageTextPasswordChange:
		_, err = s.z.Admin.SetDefaultPasswordChangeMessageText(ctx, &admin.SetDefaultPasswordChangeMessageTextRequest{
			Language: spec.Language, Title: spec.Title, PreHeader: spec.PreHeader, Subject: spec.Subject,
			Greeting: spec.Greeting, Text: spec.Text, ButtonText: spec.ButtonText,
		})
	default:
		return fmt.Errorf("unsupported message text type %q", spec.Type)
	}

	return err //nolint:wrapcheck // wrapped by the caller with the type.
}

func messageTextEqual(cur *text.MessageCustomText, spec v1alpha1.MessageTextSpec) bool {
	return cur.GetTitle() == spec.Title && cur.GetPreHeader() == spec.PreHeader && cur.GetSubject() == spec.Subject &&
		cur.GetGreeting() == spec.Greeting && cur.GetText() == spec.Text && cur.GetButtonText() == spec.ButtonText
}

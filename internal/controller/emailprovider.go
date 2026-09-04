package controller

import (
	"context"
	"errors"
	"fmt"

	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/admin"
	"github.com/zitadel/zitadel-go/v3/pkg/client/zitadel/settings"

	"github.com/malpou/zitadel-operator/api/v1alpha1"
)

var errNotSMTP = errors.New("email provider exists but is not an SMTP provider")

// emailProviderSyncer manages an instance-level SMTP provider (admin v1,
// gone in Zitadel V5). Providers have no name, so adoption matches on
// host + sender address.
type emailProviderSyncer struct {
	z Clients
}

func (s *emailProviderSyncer) sync(ctx context.Context, p *v1alpha1.EmailProvider) error {
	if err := ensureID(ctx, &p.Status,
		func(ctx context.Context) (string, error) { return s.find(ctx, p.Spec) },
		func(ctx context.Context) (string, error) {
			resp, err := s.z.Admin.AddEmailProviderSMTP(ctx, &admin.AddEmailProviderSMTPRequest{
				SenderAddress: p.Spec.SenderAddress, SenderName: p.Spec.SenderName, Tls: p.Spec.TLS,
				Host: p.Spec.Host, ReplyToAddress: p.Spec.ReplyToAddress, Description: p.Spec.Description,
			})
			if err != nil {
				return "", fmt.Errorf("add email provider: %w", err)
			}

			return resp.GetId(), nil
		},
	); err != nil {
		return err
	}

	got, err := s.z.Admin.GetEmailProviderById(ctx, &admin.GetEmailProviderByIdRequest{Id: p.Status.ID})
	if err != nil {
		if isNotFound(err) {
			p.Status.ID = ""

			return fmt.Errorf("email provider %s: %w", p.Spec.Host, errVanished)
		}

		return fmt.Errorf("get email provider: %w", err)
	}
	cur := got.GetConfig()
	smtp := cur.GetSmtp()
	if smtp == nil {
		return errNotSMTP
	}
	if cur.GetDescription() != p.Spec.Description || smtp.GetHost() != p.Spec.Host || smtp.GetTls() != p.Spec.TLS ||
		smtp.GetSenderAddress() != p.Spec.SenderAddress || smtp.GetSenderName() != p.Spec.SenderName ||
		smtp.GetReplyToAddress() != p.Spec.ReplyToAddress {
		if _, err = s.z.Admin.UpdateEmailProviderSMTP(ctx, &admin.UpdateEmailProviderSMTPRequest{
			Id: p.Status.ID, SenderAddress: p.Spec.SenderAddress, SenderName: p.Spec.SenderName, Tls: p.Spec.TLS,
			Host: p.Spec.Host, ReplyToAddress: p.Spec.ReplyToAddress, Description: p.Spec.Description,
		}); err != nil {
			return fmt.Errorf("update email provider: %w", err)
		}
	}
	if p.Spec.Active && cur.GetState() != settings.EmailProviderState_EMAIL_PROVIDER_ACTIVE {
		if _, err = s.z.Admin.ActivateEmailProvider(
			ctx,
			&admin.ActivateEmailProviderRequest{Id: p.Status.ID},
		); err != nil {
			return fmt.Errorf("activate email provider: %w", err)
		}
	}

	return nil
}

func (*emailProviderSyncer) remove(context.Context, *v1alpha1.EmailProvider) error { return nil }

func (s *emailProviderSyncer) find(ctx context.Context, spec v1alpha1.EmailProviderSpec) (string, error) {
	resp, err := s.z.Admin.ListEmailProviders(ctx, &admin.ListEmailProvidersRequest{})
	if err != nil {
		return "", fmt.Errorf("list email providers: %w", err)
	}
	for _, p := range resp.GetResult() {
		smtp := p.GetSmtp()
		if smtp != nil && smtp.GetHost() == spec.Host && smtp.GetSenderAddress() == spec.SenderAddress {
			return p.GetId(), nil
		}
	}

	return "", nil
}

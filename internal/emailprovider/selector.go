package emailprovider

import (
	"context"

	"logtheater/internal/domain"
	"logtheater/internal/emailconfig"
)

type Selector struct {
	settings *emailconfig.Store
	outlook  Provider
	gmail    Provider
}

func NewSelector(settings *emailconfig.Store, outlook, gmail Provider) *Selector {
	return &Selector{settings: settings, outlook: outlook, gmail: gmail}
}

func (s *Selector) Provider() domain.EmailProviderType {
	runtime, err := s.settings.Runtime()
	if err == nil {
		return runtime.Provider
	}
	return domain.EmailProviderOutlook
}

func (s *Selector) selected() Provider {
	if s.Provider() == domain.EmailProviderGmail {
		return s.gmail
	}
	return s.outlook
}

func (s *Selector) TestConnection(ctx context.Context) error { return s.selected().TestConnection(ctx) }
func (s *Selector) Send(ctx context.Context, message domain.EmailMessage) error {
	return s.selected().Send(ctx, message)
}

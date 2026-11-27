package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

type whatsAppDeliveryStatusStore interface {
	ApplyProviderStatuses(ctx context.Context, q repository.Querier, accountID, channelID uuid.UUID,
		statuses []domain.ProviderDeliveryStatus) ([]domain.QuoteSend, error)
}

// WhatsAppDeliveryStatusService records what Meta reports about quote messages a branch number
// sent: delivered, read, or refused.
type WhatsAppDeliveryStatusService struct {
	db     tenantTxRunner
	routes whatsAppInboundRouter
	sends  whatsAppDeliveryStatusStore
	log    *slog.Logger
}

// NewWhatsAppDeliveryStatusService builds a WhatsAppDeliveryStatusService.
func NewWhatsAppDeliveryStatusService(db tenantTxRunner, routes whatsAppInboundRouter,
	sends whatsAppDeliveryStatusStore, log *slog.Logger,
) *WhatsAppDeliveryStatusService {
	if log == nil {
		log = slog.Default()
	}
	return &WhatsAppDeliveryStatusService{db: db, routes: routes, sends: sends, log: log}
}

// Record moves each reported send forward under its channel's tenant. Statuses for channels Coti
// does not route, or for messages that are not quote sends, are dropped.
func (s *WhatsAppDeliveryStatusService) Record(ctx context.Context,
	statuses []domain.ProviderDeliveryStatus,
) error {
	if s.db == nil || s.routes == nil || s.sends == nil {
		return fmt.Errorf("%w: WhatsApp delivery statuses are not fully wired", domain.ErrNotConfigured)
	}
	if len(statuses) == 0 {
		return nil
	}
	byDestination := map[string]map[string]domain.ProviderDeliveryStatus{}
	destinations := make([]string, 0, len(statuses))
	for _, status := range statuses {
		destination, err := domain.NormalizeInboundChannelIdentifier(domain.ChannelTypeWhatsApp,
			status.DestinationIdentifier)
		if err != nil {
			return err
		}
		latest, seen := byDestination[destination]
		if !seen {
			latest = map[string]domain.ProviderDeliveryStatus{}
			byDestination[destination] = latest
			destinations = append(destinations, destination)
		}
		// Meta may report out of order; one update per message keeps the furthest outcome.
		if current, found := latest[status.ProviderReference]; !found ||
			deliveryStatusRank(status.Status) > deliveryStatusRank(current.Status) {
			latest[status.ProviderReference] = status
		}
	}

	routes, err := s.routes.ResolveChannels(ctx, domain.ChannelTypeWhatsApp, destinations)
	if err != nil && !errors.Is(err, domain.ErrInboundDestinationInactive) {
		return err
	}
	for _, destination := range destinations {
		route, found := routes[destination]
		if !found {
			continue
		}
		reported := byDestination[destination]
		batch := make([]domain.ProviderDeliveryStatus, 0, len(reported))
		for _, status := range reported {
			batch = append(batch, status)
		}
		var changed []domain.QuoteSend
		if err := s.db.InTenantTx(ctx, route.Tenant(), func(q repository.Querier) error {
			var applyErr error
			changed, applyErr = s.sends.ApplyProviderStatuses(ctx, q, route.AccountID,
				route.ChannelID, batch)
			return applyErr
		}); err != nil {
			return err
		}
		for _, send := range changed {
			if send.TrackingStatus != domain.SendTrackingStatusFailed || send.ProviderReference == nil {
				continue
			}
			status := reported[*send.ProviderReference]
			s.log.WarnContext(ctx, "whatsapp quote message was not delivered",
				slog.String("send_id", send.ID.String()), slog.String("error_code", status.ErrorCode),
				slog.Bool("outside_window", status.OutsideWindow))
		}
	}
	return nil
}

func deliveryStatusRank(status domain.SendTrackingStatus) int {
	switch status {
	case domain.SendTrackingStatusFailed:
		return 1
	case domain.SendTrackingStatusDelivered:
		return 2
	case domain.SendTrackingStatusViewed:
		return 3
	}
	return 0
}

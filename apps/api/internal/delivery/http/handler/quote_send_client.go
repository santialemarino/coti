package handler

import (
	"github.com/santialemarino/coti/apps/api/internal/delivery/http/dto"
	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func fromSendClientRequest(in *dto.AssociateQuoteClientRequest) *domain.AssociateQuoteClientInput {
	if in == nil {
		return nil
	}
	result := &domain.AssociateQuoteClientInput{TaxID: in.TaxID, ClientID: in.ClientID, TagIDs: in.TagIDs}
	if in.NewClient != nil {
		result.New = &domain.NewClient{Name: in.NewClient.Name, Phone: in.NewClient.Phone, Email: in.NewClient.Email}
	}
	return result
}

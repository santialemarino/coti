package services

import (
	"github.com/shopspring/decimal"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

func previousReportQuery(in domain.SellerReportQuery) domain.SellerReportQuery {
	start, end := in.StartAt.In(domain.Argentina), in.EndAt.In(domain.Argentina)
	days := 0
	for day := start; day.Before(end); day = day.AddDate(0, 0, 1) {
		days++
	}
	in.EndAt = in.StartAt
	in.StartAt = start.AddDate(0, 0, -days).UTC()
	in.FromDate = in.StartAt.In(domain.Argentina).Format(reportDateLayout)
	in.ToDate = start.AddDate(0, 0, -1).Format(reportDateLayout)
	return in
}

func reportChanges(current, previous domain.SellerReportKPIs) map[string]domain.SellerReportChange {
	count := func(n int) *decimal.Decimal { value := decimal.NewFromInt(int64(n)); return &value }
	duration := func(n *int64) *decimal.Decimal {
		if n == nil {
			return nil
		}
		value := decimal.NewFromInt(*n)
		return &value
	}
	return map[string]domain.SellerReportChange{
		"rfqs_received":      reportChange(count(current.RFQsReceived), count(previous.RFQsReceived), false),
		"quotes_sent":        reportChange(count(current.QuotesSent), count(previous.QuotesSent), false),
		"quotes_accepted":    reportChange(count(current.QuotesAccepted), count(previous.QuotesAccepted), false),
		"conversion_rate":    reportChange(current.ConversionRate, previous.ConversionRate, true),
		"response_time":      reportChange(duration(current.ResponseTime.AverageSeconds), duration(previous.ResponseTime.AverageSeconds), false),
		"time_to_acceptance": reportChange(duration(current.TimeToAcceptance.AverageSeconds), duration(previous.TimeToAcceptance.AverageSeconds), false),
	}
}

func reportChange(current, previous *decimal.Decimal, points bool) domain.SellerReportChange {
	change := domain.SellerReportChange{Previous: previous, PercentagePoints: points}
	if current == nil || previous == nil {
		return change
	}
	delta := current.Sub(*previous)
	if points {
		delta = delta.Mul(decimal.NewFromInt(100))
	}
	change.Delta = &delta
	if !points && previous.IsPositive() {
		percent := delta.Div(*previous).Mul(decimal.NewFromInt(100))
		change.Percentage = &percent
	}
	if !points && previous.IsZero() && current.IsZero() {
		zero := decimal.Zero
		change.Percentage = &zero
	}
	return change
}

package services

import (
	"context"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
)

type fakeEmailMailbox struct {
	history       map[string]domain.EmailSyncPage
	full          map[string]domain.EmailSyncPage
	messages      map[string]domain.InboundEmailMessage
	historyErr    error
	messageErr    map[string]error
	profileCursor string
	fullSince     time.Time
	readIDs       []string
}

func (m *fakeEmailMailbox) CurrentCursor(context.Context) (string, error) {
	return m.profileCursor, nil
}

func (m *fakeEmailMailbox) History(_ context.Context, _, page string) (domain.EmailSyncPage, error) {
	return m.history[page], m.historyErr
}

func (m *fakeEmailMailbox) MessagesSince(_ context.Context, since time.Time, page string) (domain.EmailSyncPage, error) {
	m.fullSince = since
	return m.full[page], nil
}

func (m *fakeEmailMailbox) ReadMessage(_ context.Context, id string) (domain.InboundEmailMessage, error) {
	m.readIDs = append(m.readIDs, id)
	return m.messages[id], m.messageErr[id]
}

type fakeEmailIngester struct {
	ids     []string
	routes  []domain.InboundChannelRoute
	failFor string
}

func (i *fakeEmailIngester) Ingest(_ context.Context, route domain.InboundChannelRoute,
	message domain.InboundEmailMessage) (uuid.UUID, error) {
	if message.ExternalMessageID == i.failFor {
		return uuid.Nil, errors.New("storage unavailable")
	}
	i.ids = append(i.ids, message.ExternalMessageID)
	i.routes = append(i.routes, route)
	return uuid.New(), nil
}

func testEmailConnection() domain.EmailMailboxConnection {
	return domain.EmailMailboxConnection{Route: domain.InboundChannelRoute{AccountID: uuid.New(),
		BranchID: uuid.New(), ChannelID: uuid.New(), ChannelType: domain.ChannelTypeEmail},
		Address: "branch@example.test", HistoryID: "100", ConnectedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
}

func testSyncMessage(id string, received time.Time) domain.InboundEmailMessage {
	return domain.InboundEmailMessage{ExternalMessageID: id, ProviderReceivedAt: received,
		Original: []byte("original")}
}

func TestEmailSync_HistoryPagesAreDeduplicatedAndKeepTheRoute(t *testing.T) {
	connection := testEmailConnection()
	mailbox := &fakeEmailMailbox{history: map[string]domain.EmailSyncPage{
		"":     {MessageIDs: []string{"one", "two"}, NextPageToken: "next", HistoryID: "110"},
		"next": {MessageIDs: []string{"two", "three"}, HistoryID: "120"},
	}, messages: map[string]domain.InboundEmailMessage{
		"one":   testSyncMessage("one", connection.ConnectedAt),
		"two":   testSyncMessage("two", connection.ConnectedAt),
		"three": testSyncMessage("three", connection.ConnectedAt),
	}}
	ingester := &fakeEmailIngester{}
	cursor, err := NewEmailSyncService(ingester).Synchronize(context.Background(), mailbox, connection)
	if err != nil || cursor != "120" || !reflect.DeepEqual(ingester.ids, []string{"one", "two", "three"}) ||
		!reflect.DeepEqual(mailbox.readIDs, []string{"one", "two", "three"}) {
		t.Fatalf("cursor = %q, ingested = %v, read = %v, error = %v", cursor, ingester.ids, mailbox.readIDs, err)
	}
	for _, route := range ingester.routes {
		if route != connection.Route {
			t.Fatalf("route = %+v, want %+v", route, connection.Route)
		}
	}
}

func TestEmailSync_FailureLeavesTheOriginalCursorForRetry(t *testing.T) {
	connection := testEmailConnection()
	mailbox := &fakeEmailMailbox{history: map[string]domain.EmailSyncPage{
		"": {MessageIDs: []string{"one", "two"}, HistoryID: "120"},
	}, messages: map[string]domain.InboundEmailMessage{
		"one": testSyncMessage("one", connection.ConnectedAt),
		"two": testSyncMessage("two", connection.ConnectedAt),
	}}
	ingester := &fakeEmailIngester{failFor: "two"}
	cursor, err := NewEmailSyncService(ingester).Synchronize(context.Background(), mailbox, connection)
	if err == nil || cursor != "" || !reflect.DeepEqual(ingester.ids, []string{"one"}) {
		t.Fatalf("cursor = %q, ingested = %v, error = %v", cursor, ingester.ids, err)
	}
}

func TestEmailSync_ExpiredHistoryRecoversOnlyMessagesSinceConnection(t *testing.T) {
	connection := testEmailConnection()
	mailbox := &fakeEmailMailbox{historyErr: domain.ErrEmailHistoryExpired, profileCursor: "200",
		full: map[string]domain.EmailSyncPage{
			"":     {MessageIDs: []string{"old", "new"}, NextPageToken: "next"},
			"next": {MessageIDs: []string{"gone", "newer"}},
		}, messages: map[string]domain.InboundEmailMessage{
			"old":   testSyncMessage("old", connection.ConnectedAt.Add(-time.Second)),
			"new":   testSyncMessage("new", connection.ConnectedAt),
			"newer": testSyncMessage("newer", connection.ConnectedAt.Add(time.Second)),
		}, messageErr: map[string]error{"gone": domain.ErrEmailMessageGone},
	}
	ingester := &fakeEmailIngester{}
	cursor, err := NewEmailSyncService(ingester).Synchronize(context.Background(), mailbox, connection)
	if err != nil || cursor != "200" || mailbox.fullSince != connection.ConnectedAt ||
		!reflect.DeepEqual(ingester.ids, []string{"new", "newer"}) {
		t.Fatalf("cursor = %q, since = %v, ingested = %v, error = %v", cursor, mailbox.fullSince, ingester.ids, err)
	}
}

func TestEmailSync_FirstSynchronizationIncludesMessagesReceivedDuringAuthorization(t *testing.T) {
	connection := testEmailConnection()
	connection.InitialSyncPending = true
	mailbox := &fakeEmailMailbox{historyErr: errors.New("history must not be called"), profileCursor: "250",
		full: map[string]domain.EmailSyncPage{"": {MessageIDs: []string{"during-authorization"}}},
		messages: map[string]domain.InboundEmailMessage{
			"during-authorization": testSyncMessage("during-authorization", connection.ConnectedAt),
		},
	}
	ingester := &fakeEmailIngester{}
	cursor, err := NewEmailSyncService(ingester).Synchronize(context.Background(), mailbox, connection)
	if err != nil || cursor != "250" || !reflect.DeepEqual(ingester.ids, []string{"during-authorization"}) {
		t.Fatalf("cursor = %q, ingested = %v, error = %v", cursor, ingester.ids, err)
	}
}

func TestEmailSync_RepeatedPageDoesNotAdvanceCursor(t *testing.T) {
	connection := testEmailConnection()
	mailbox := &fakeEmailMailbox{history: map[string]domain.EmailSyncPage{
		"": {NextPageToken: "same"}, "same": {NextPageToken: "same"},
	}}
	cursor, err := NewEmailSyncService(&fakeEmailIngester{}).Synchronize(context.Background(), mailbox, connection)
	if err == nil || cursor != "" {
		t.Fatalf("cursor = %q, error = %v", cursor, err)
	}
}

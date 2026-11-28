package services

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/santialemarino/coti/apps/api/internal/domain"
	"github.com/santialemarino/coti/apps/api/internal/repository"
)

// --- intake: what the webhook does with a file ---

type fakeMediaCreator struct {
	rfqIDs   []uuid.UUID
	messages []domain.InboundTextMessage
}

func (f *fakeMediaCreator) CreateMediaOrder(_ context.Context, _ domain.InboundChannelRoute,
	rfqID uuid.UUID, message domain.InboundTextMessage) error {
	f.rfqIDs = append(f.rfqIDs, rfqID)
	f.messages = append(f.messages, message)
	return nil
}

func voiceNoteMessage() []domain.InboundTextMessage {
	return []domain.InboundTextMessage{{ExternalMessageID: "wamid.voice",
		DestinationIdentifier: "phone-id", SenderID: "5491112345678",
		Media: &domain.InboundMedia{ProviderID: "media-voice", MediaType: "audio/ogg; codecs=opus",
			Kind: domain.InboundMediaAudio},
		Payload: []byte(`{"message":"wamid.voice"}`)}}
}

// A voice note with no caption is an order of its own: it reaches the media intake under the
// reserved order, never the text pipeline, which would read an empty message.
func TestWhatsAppInboundService_IngestOpensAnOrderForAFileWithNoCaption(t *testing.T) {
	t.Parallel()
	service, messages, rfqs, _ := inboundTestService(&fakeWhatsAppInboundConversations{})
	media := &fakeMediaCreator{}
	service.WithMedia(media)

	if err := service.Ingest(context.Background(), voiceNoteMessage()); err != nil {
		t.Fatalf("Ingest() = %v, want the voice note accepted", err)
	}
	if len(media.rfqIDs) != 1 || messages.reserved[0].RFQID == nil ||
		media.rfqIDs[0] != *messages.reserved[0].RFQID {
		t.Fatalf("media orders = %v, want one under the reserved order", media.rfqIDs)
	}
	if messages.reserved[0].Body != "[Audio]" || len(rfqs.inputs) != 0 {
		t.Errorf("stored body = %q, text drafts = %d; want the file named and no text draft",
			messages.reserved[0].Body, len(rfqs.inputs))
	}
}

// A file from a sender with a live quote joins its conversation, named so the seller knows a
// file arrived even when it carried no words.
func TestWhatsAppInboundService_IngestJoinsAFileToALiveConversation(t *testing.T) {
	t.Parallel()
	conversations := &fakeWhatsAppInboundConversations{
		match: &domain.ConversationRoute{QuoteID: uuid.New()}}
	service, _, _, _ := inboundTestService(conversations)
	media := &fakeMediaCreator{}
	service.WithMedia(media)
	photo := voiceNoteMessage()
	photo[0].Body = "lo de la foto"
	photo[0].Media.Kind = domain.InboundMediaImage

	if err := service.Ingest(context.Background(), photo); err != nil {
		t.Fatalf("Ingest() = %v", err)
	}
	if len(conversations.appended) != 1 || conversations.appended[0].Body != "[Foto] lo de la foto" {
		t.Fatalf("appended = %+v, want the photo named with its caption", conversations.appended)
	}
	if len(media.rfqIDs) != 0 {
		t.Errorf("media orders = %d, want none for a conversation message", len(media.rfqIDs))
	}
}

// Without media intake wired a file is skipped, never turned into an order of its caption.
func TestWhatsAppInboundService_IngestSkipsAFileWhenMediaIsNotWired(t *testing.T) {
	t.Parallel()
	service, messages, rfqs, _ := inboundTestService(&fakeWhatsAppInboundConversations{})
	if err := service.Ingest(context.Background(), voiceNoteMessage()); err != nil {
		t.Fatalf("Ingest() = %v", err)
	}
	if len(messages.reserved) != 0 || len(rfqs.inputs) != 0 {
		t.Fatalf("reserved %d, drafted %d; want the file ignored", len(messages.reserved),
			len(rfqs.inputs))
	}
}

// --- the media order itself ---

type fakeMediaRFQs struct {
	existing bool
	created  []domain.NewRFQ
}

func (f *fakeMediaRFQs) Create(_ context.Context, _ repository.Querier, _ uuid.UUID,
	in domain.NewRFQ) (*domain.RFQ, error) {
	f.created = append(f.created, in)
	return &domain.RFQ{ID: *in.ID, Status: in.Status}, nil
}

func (f *fakeMediaRFQs) GetByID(_ context.Context, _ repository.Querier, _, _,
	rfqID uuid.UUID) (*domain.RFQ, error) {
	if f.existing {
		return &domain.RFQ{ID: rfqID}, nil
	}
	return nil, domain.ErrNotFound
}

type fakeMediaAttachments struct {
	batches [][]domain.NewRetainedAttachment
}

func (f *fakeMediaAttachments) CreateInboundBatch(_ context.Context, _ repository.Querier, _, _,
	_ uuid.UUID, attachments []domain.NewRetainedAttachment) error {
	f.batches = append(f.batches, attachments)
	return nil
}

type recordingStorage struct {
	fakeObjectStorage
	uploads map[string][]byte
	types   map[string]string
}

func (s *recordingStorage) Upload(_ context.Context, key, contentType string, content io.Reader) error {
	data, err := io.ReadAll(content)
	if err != nil {
		return err
	}
	if s.uploads == nil {
		s.uploads, s.types = map[string][]byte{}, map[string]string{}
	}
	s.uploads[key], s.types[key] = data, contentType
	return nil
}

func mediaRoute() domain.InboundChannelRoute {
	return domain.InboundChannelRoute{AccountID: uuid.New(), BranchID: uuid.New(),
		ChannelID: uuid.New(), ChannelType: domain.ChannelTypeWhatsApp}
}

func TestWhatsAppMediaIntake_StoresTheFileAsPendingWithItsCaption(t *testing.T) {
	t.Parallel()
	rfqs, attachments, storage := &fakeMediaRFQs{}, &fakeMediaAttachments{}, &recordingStorage{}
	woken := 0
	intake := NewWhatsAppMediaIntake(&fakeWhatsAppInboundDB{}, rfqs, attachments, storage,
		func() { woken++ })
	message := voiceNoteMessage()[0]
	message.Body = "para la obra de Ramos"

	if err := intake.CreateMediaOrder(context.Background(), mediaRoute(), uuid.New(), message); err != nil {
		t.Fatalf("CreateMediaOrder() = %v", err)
	}
	if len(rfqs.created) != 1 || rfqs.created[0].Status != domain.RFQStatusReceived ||
		*rfqs.created[0].RawText != "[Audio] para la obra de Ramos" {
		t.Fatalf("rfq = %+v, want a RECEIVED order naming the recording", rfqs.created)
	}
	files := attachments.batches[0]
	if len(files) != 2 || files[0].Type != domain.AttachmentTypeAudio ||
		files[0].Status != domain.AttachmentProcessingPending ||
		files[0].StorageKey != domain.WhatsAppMediaKeyPrefix+"media-voice" {
		t.Fatalf("file = %+v, want the recording pending under its media ID", files[0])
	}
	if files[1].Type != domain.AttachmentTypeText ||
		string(storage.uploads[files[1].StorageKey]) != "para la obra de Ramos" {
		t.Errorf("caption = %+v, want it stored as a text attachment", files[1])
	}
	if woken != 1 {
		t.Errorf("sweep woken %d times, want once", woken)
	}
}

// A video is not an order format: the order still appears, failed, so the seller sees it.
func TestWhatsAppMediaIntake_LeavesAFileNoFormatAcceptsAsAFailedOrder(t *testing.T) {
	t.Parallel()
	rfqs, attachments := &fakeMediaRFQs{}, &fakeMediaAttachments{}
	woken := 0
	intake := NewWhatsAppMediaIntake(&fakeWhatsAppInboundDB{}, rfqs, attachments,
		&recordingStorage{}, func() { woken++ })
	message := voiceNoteMessage()[0]
	message.Media = &domain.InboundMedia{ProviderID: "media-video", MediaType: "video/mp4",
		Kind: domain.InboundMediaVideo}

	if err := intake.CreateMediaOrder(context.Background(), mediaRoute(), uuid.New(), message); err != nil {
		t.Fatalf("CreateMediaOrder() = %v", err)
	}
	if rfqs.created[0].Status != domain.RFQStatusFailed ||
		attachments.batches[0][0].Status != domain.AttachmentProcessingFailed || woken != 0 {
		t.Fatalf("rfq %s, file %s, woken %d; want a visible failed order and no sweep",
			rfqs.created[0].Status, attachments.batches[0][0].Status, woken)
	}
}

// A document that is a photo («pedido.png») is read as a photo.
func TestWhatsAppMediaIntake_ReadsAnImageSentAsADocumentAsAPhoto(t *testing.T) {
	t.Parallel()
	attachments := &fakeMediaAttachments{}
	intake := NewWhatsAppMediaIntake(&fakeWhatsAppInboundDB{}, &fakeMediaRFQs{}, attachments,
		&recordingStorage{}, nil)
	message := voiceNoteMessage()[0]
	message.Media = &domain.InboundMedia{ProviderID: "media-doc", MediaType: "image/png",
		Filename: "pedido.png", Kind: domain.InboundMediaDocument}

	if err := intake.CreateMediaOrder(context.Background(), mediaRoute(), uuid.New(), message); err != nil {
		t.Fatalf("CreateMediaOrder() = %v", err)
	}
	file := attachments.batches[0][0]
	if file.Type != domain.AttachmentTypeImage || file.OriginalFilename == nil ||
		*file.OriginalFilename != "pedido.png" {
		t.Fatalf("file = %+v, want an image keeping its filename", file)
	}
}

func TestWhatsAppMediaIntake_ARetriedDeliveryChangesNothing(t *testing.T) {
	t.Parallel()
	rfqs, attachments := &fakeMediaRFQs{existing: true}, &fakeMediaAttachments{}
	intake := NewWhatsAppMediaIntake(&fakeWhatsAppInboundDB{}, rfqs, attachments,
		&recordingStorage{}, nil)
	if err := intake.CreateMediaOrder(context.Background(), mediaRoute(), uuid.New(),
		voiceNoteMessage()[0]); err != nil {
		t.Fatalf("CreateMediaOrder() = %v", err)
	}
	if len(rfqs.created) != 0 || len(attachments.batches) != 0 {
		t.Fatalf("created %d orders and %d batches, want none on a retry", len(rfqs.created),
			len(attachments.batches))
	}
}

// --- the sweep fetching the file ---

type fakeMediaFetcher struct {
	media    *domain.FetchedMedia
	err      error
	tokens   []string
	maxBytes []int64
}

func (f *fakeMediaFetcher) FetchMedia(_ context.Context, token, _ string,
	maxBytes int64) (*domain.FetchedMedia, error) {
	f.tokens = append(f.tokens, token)
	f.maxBytes = append(f.maxBytes, maxBytes)
	return f.media, f.err
}

type fakeMediaChannels struct{ raw []byte }

func (f fakeMediaChannels) GetActiveConfigurationsByIDs(_ context.Context, _ repository.Querier,
	_, _ uuid.UUID, ids []uuid.UUID) (map[uuid.UUID][]byte, error) {
	return map[uuid.UUID][]byte{ids[0]: f.raw}, nil
}

type fakeStoredFiles struct {
	keys  []string
	kinds []domain.AttachmentType
}

func (f *fakeStoredFiles) SetStoredFile(_ context.Context, _ repository.Querier, _, _ uuid.UUID,
	key string, kind domain.AttachmentType) error {
	f.keys, f.kinds = append(f.keys, key), append(f.kinds, kind)
	return nil
}

func whatsAppChannelConfig() []byte {
	return []byte(`{"phone_number_id":"phone-id","access_token":"sealed:token-1",` +
		`"webhook_verify_token":"sealed:verify"}`)
}

func mediaSource(fetcher *fakeMediaFetcher, files *fakeStoredFiles,
	storage domain.ObjectStorage) *WhatsAppMediaSource {
	return NewWhatsAppMediaSource(&fakeWhatsAppInboundDB{}, fakeMediaChannels{raw: whatsAppChannelConfig()},
		files, fakeWhatsAppChannelOpener{}, fetcher, storage, 100<<20)
}

func pendingMedia(kind domain.AttachmentType) domain.ClaimedAttachment {
	return domain.ClaimedAttachment{ID: uuid.New(), AccountID: uuid.New(), BranchID: uuid.New(),
		ChannelID: uuid.New(), RFQID: uuid.New(), Type: kind,
		StorageKey: domain.WhatsAppMediaKeyPrefix + "media-voice", CreatedAt: time.Now()}
}

func TestWhatsAppMediaSource_FetchesWithTheChannelTokenAndStoresTheFile(t *testing.T) {
	t.Parallel()
	fetcher := &fakeMediaFetcher{media: &domain.FetchedMedia{
		MediaType: "audio/ogg; codecs=opus", Data: []byte("OggS")}}
	files, storage := &fakeStoredFiles{}, &recordingStorage{}

	got, err := mediaSource(fetcher, files, storage).Materialize(context.Background(),
		pendingMedia(domain.AttachmentTypeAudio))
	if err != nil {
		t.Fatalf("Materialize() = %v", err)
	}
	if fetcher.tokens[0] != "token-1" || fetcher.maxBytes[0] != maxTranscriptionBytes {
		t.Errorf("fetched with %q under %d, want the channel's token and the transcription cap",
			fetcher.tokens[0], fetcher.maxBytes[0])
	}
	if !strings.HasSuffix(got.StorageKey, ".ogg") || storage.types[got.StorageKey] != "audio/ogg" ||
		!bytes.Equal(storage.uploads[got.StorageKey], []byte("OggS")) || files.keys[0] != got.StorageKey {
		t.Fatalf("stored %q as %q, want the recording under an .ogg key, recorded on the row",
			got.StorageKey, storage.types[got.StorageKey])
	}
}

func TestWhatsAppMediaSource_RefusesBytesNoFormatAccepts(t *testing.T) {
	t.Parallel()
	fetcher := &fakeMediaFetcher{media: &domain.FetchedMedia{MediaType: "video/mp4", Data: []byte("x")}}
	_, err := mediaSource(fetcher, &fakeStoredFiles{}, &recordingStorage{}).Materialize(
		context.Background(), pendingMedia(domain.AttachmentTypeImage))
	if !errors.Is(err, domain.ErrInvalidInput) {
		t.Fatalf("Materialize() = %v, want ErrInvalidInput", err)
	}
}

// --- the sweep's seam for the intake window ---

type fakeHold struct{ held map[uuid.UUID]bool }

func (f fakeHold) Held(context.Context, repository.Querier, []uuid.UUID) (map[uuid.UUID]bool, error) {
	return f.held, nil
}

// An order its sender may still be adding to waits; the others are read now.
func TestAttachmentExtractionJob_Run_SkipsAnOrderTheHoldNames(t *testing.T) {
	t.Parallel()
	waiting, ready := claimedIn(uuid.New(), domain.AttachmentTypeText),
		claimedIn(uuid.New(), domain.AttachmentTypeText)
	repo := &fakeClaimRepo{claim: []domain.ClaimedAttachment{waiting, ready}}
	folder := &fakeFolder{}

	job := attachmentJob(repo, textReader("cemento"), folder).
		WithHold(fakeHold{held: map[uuid.UUID]bool{waiting.RFQID: true}})
	if _, err := job.Run(context.Background(), nil); err != nil {
		t.Fatalf("Run() = %v", err)
	}
	if len(folder.calls) != 1 || folder.calls[0].rfqID != ready.RFQID {
		t.Fatalf("folds = %+v, want only the order not held", folder.calls)
	}
	if _, closed := repo.closed[waiting.ID]; closed {
		t.Error("held order was closed out, want it left claimed")
	}
}

// --- the sweep's answer to an outage ---

// Meta being unreachable is not the file's fault: the order stays claimed for a later firing.
func TestAttachmentExtractionJob_Run_LeavesAnOrderClaimedWhenItsMediaIsUnavailable(t *testing.T) {
	t.Parallel()
	attachment := claimedIn(uuid.New(), domain.AttachmentTypeAudio)
	repo := &fakeClaimRepo{claim: []domain.ClaimedAttachment{attachment}}
	reader := fakeReader{byType: map[domain.AttachmentType]readResult{
		domain.AttachmentTypeAudio: {err: fmt.Errorf("%w: 503", domain.ErrMediaUnavailable)},
	}}
	folder := &fakeFolder{}

	if _, err := attachmentJob(repo, reader, folder).Run(context.Background(), nil); err == nil {
		t.Fatal("Run() = nil, want the outage reported")
	}
	if _, closed := repo.closed[attachment.ID]; closed || len(folder.calls) != 0 {
		t.Fatalf("closed = %v, folds = %d; want the order left claimed and not extracted",
			closed, len(folder.calls))
	}
}

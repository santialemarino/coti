package domain

import "testing"

func TestAttachmentFormatFor_AcceptsOnlyTheClosedSet(t *testing.T) {
	t.Parallel()

	if _, ok := AttachmentFormatFor("application/pdf"); !ok {
		t.Error("application/pdf is not accepted")
	}
	if _, ok := AttachmentFormatFor("text/html"); ok {
		t.Error("text/html is accepted, which would put an executable document in storage")
	}
}

// Every accepted format maps to a real attachment_type value and carries an extension: a blank
// one would produce keys ending in a bare dot, and an invented type would fail the insert.
func TestAttachmentFormatFor_EveryFormatIsUsable(t *testing.T) {
	t.Parallel()
	valid := map[AttachmentType]bool{
		AttachmentTypeImage: true, AttachmentTypePDF: true, AttachmentTypeSpreadsheet: true,
		AttachmentTypeAudio: true, AttachmentTypeText: true,
	}

	for _, contentType := range AcceptedAttachmentContentTypes() {
		format, ok := AttachmentFormatFor(contentType)
		if !ok {
			t.Fatalf("%q is listed as accepted but does not resolve", contentType)
		}
		if !valid[format.Type] {
			t.Errorf("%q maps to %q, which is not an attachment_type value", contentType, format.Type)
		}
		if format.Extension == "" {
			t.Errorf("%q has no extension", contentType)
		}
	}
}

func TestAcceptedAttachmentContentTypes_ListsEveryFormat(t *testing.T) {
	t.Parallel()

	if got, want := len(AcceptedAttachmentContentTypes()), len(attachmentFormats); got != want {
		t.Fatalf("listed %d content types, want %d", got, want)
	}
}

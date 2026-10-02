-- An attachment claimed for processing has no timestamp of its own, so a run that dies between
-- claiming a row and finishing it leaves that row at PROCESSING with nothing able to tell it apart
-- from one being worked right now. The queue then leaks exactly the way it leaks at PENDING when
-- nothing drains it.
--
-- processing_started_at is what makes a claim expire: the sweep reclaims a PROCESSING row whose
-- claim is older than its configured window. processed_at keeps its own meaning — the transition
-- that finished the work — so the two timestamps answer different questions.

-- +goose Up
ALTER TABLE rfq_attachment ADD COLUMN processing_started_at TIMESTAMPTZ;

-- +goose Down
ALTER TABLE rfq_attachment DROP COLUMN processing_started_at;

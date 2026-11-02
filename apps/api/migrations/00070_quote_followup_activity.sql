-- +goose Up
-- Inactivity is quote.updated_at: raising the follow-up flag must not count as activity, and any other
-- write to the quote is activity that settles the flag.
-- +goose StatementBegin
CREATE OR REPLACE FUNCTION set_quote_updated_at() RETURNS TRIGGER AS $$
BEGIN
  IF NEW.updated_at = OLD.updated_at
     AND (NEW.needs_followup, NEW.followup_flagged_at) IS DISTINCT FROM (OLD.needs_followup, OLD.followup_flagged_at)
     AND to_jsonb(NEW) - ARRAY['needs_followup', 'followup_flagged_at'] = to_jsonb(OLD) - ARRAY['needs_followup', 'followup_flagged_at'] THEN
    RETURN NEW;
  END IF;
  NEW.updated_at = now();
  NEW.needs_followup = FALSE;
  NEW.followup_flagged_at = NULL;
  RETURN NEW;
END;
$$ LANGUAGE plpgsql;
-- +goose StatementEnd
DROP TRIGGER trg_quote_updated ON quote;
CREATE TRIGGER trg_quote_updated BEFORE UPDATE ON quote FOR EACH ROW EXECUTE FUNCTION set_quote_updated_at();

-- +goose Down
DROP TRIGGER trg_quote_updated ON quote;
CREATE TRIGGER trg_quote_updated BEFORE UPDATE ON quote FOR EACH ROW EXECUTE FUNCTION set_updated_at();
DROP FUNCTION set_quote_updated_at();

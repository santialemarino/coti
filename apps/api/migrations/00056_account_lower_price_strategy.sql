-- +goose Up
CREATE TYPE lower_price_strategy AS ENUM ('FIND_CHEAPER_ALTERNATIVES', 'ADAPT_TO_REACH_DISCOUNT');

ALTER TABLE account
  ADD COLUMN preferred_lower_price_strategy lower_price_strategy NOT NULL
    DEFAULT 'FIND_CHEAPER_ALTERNATIVES';

-- +goose Down
ALTER TABLE account DROP COLUMN preferred_lower_price_strategy;
DROP TYPE lower_price_strategy;

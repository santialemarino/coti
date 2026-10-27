-- +goose Up
ALTER TABLE branch ADD COLUMN point_of_sale INTEGER;
ALTER TABLE branch ADD CONSTRAINT ck_branch_point_of_sale CHECK (point_of_sale BETWEEN 1 AND 99998);
ALTER TABLE branch ADD CONSTRAINT uq_branch_point_of_sale UNIQUE (account_id, point_of_sale);
UPDATE branch b SET point_of_sale = s.point_of_sale FROM branch_arca_setup s WHERE s.account_id = b.account_id AND s.branch_id = b.id;

-- +goose Down
ALTER TABLE branch DROP COLUMN point_of_sale;

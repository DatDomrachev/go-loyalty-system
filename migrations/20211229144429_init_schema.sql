-- +goose Up
-- +goose StatementBegin

CREATE TABLE if not exists url (
	id BIGSERIAL primary key,
	full_url text,
	user_token text,
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE if exists url;
-- +goose StatementEnd

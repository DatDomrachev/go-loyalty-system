-- +goose Up
-- +goose StatementBegin

CREATE TABLE if not exists users (
	id BIGSERIAL primary key,
	login text,
	password text,
	user_token text,
	balance float default 0.0,
	withdrawn float default 0.0
);

CREATE UNIQUE INDEX IF NOT EXISTS unique_login_constrain ON users(login);
CREATE UNIQUE INDEX IF NOT EXISTS unique_token_constrain ON users(user_token);

CREATE TABLE if not exists transactions (
	id BIGSERIAL primary key,
	user_token text,
	order_id text,
	type integer,
	status integer,
	points float default 0.0,
	uploaded_at TIMESTAMPTZ default now(),
	processed_at TIMESTAMPTZ,
	FOREIGN KEY (user_token) REFERENCES users (user_token)
);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP TABLE if exists transactions;
DROP INDEX IF exists unique_login_constrain;
DROP INDEX IF exists unique_token_constrain;
DROP TABLE if exists users;
-- +goose StatementEnd

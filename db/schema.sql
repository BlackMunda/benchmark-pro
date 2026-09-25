-- Portable schema: runs on both SQLite and PostgreSQL.
-- init-db always resets, so every benchmark run starts from the same state.
DROP TABLE IF EXISTS logs;
DROP TABLE IF EXISTS users;

CREATE TABLE users (
    id         INTEGER PRIMARY KEY,
    email      VARCHAR(255) NOT NULL,
    name       VARCHAR(100) NOT NULL,
    created_at TIMESTAMP    NOT NULL
);

CREATE TABLE logs (
    id         INTEGER PRIMARY KEY,
    user_id    INTEGER   NOT NULL,
    message    TEXT      NOT NULL,
    created_at TIMESTAMP NOT NULL
);

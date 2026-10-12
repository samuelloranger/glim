CREATE TABLE comments (
  id          INTEGER PRIMARY KEY,
  preview     TEXT NOT NULL,
  version     TEXT NOT NULL DEFAULT '',
  user_id     INTEGER REFERENCES users(id) ON DELETE SET NULL,
  selector    TEXT NOT NULL DEFAULT '',
  quote       TEXT NOT NULL DEFAULT '',
  x           REAL NOT NULL DEFAULT 0,
  y           REAL NOT NULL DEFAULT 0,
  body        TEXT NOT NULL,
  reply       TEXT NOT NULL DEFAULT '',
  created_at  INTEGER NOT NULL,
  updated_at  INTEGER NOT NULL,
  resolved_at INTEGER
);

CREATE INDEX comments_preview ON comments(preview, id);

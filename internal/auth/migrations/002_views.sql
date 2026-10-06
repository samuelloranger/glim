CREATE TABLE settings (
  key   TEXT PRIMARY KEY,
  value BLOB NOT NULL
);

CREATE TABLE preview_views (
  name      TEXT PRIMARY KEY,
  count     INTEGER NOT NULL DEFAULT 0,
  last_seen TIMESTAMP NOT NULL
);

-- +goose Up
CREATE TABLE review_formats (
  id TEXT NOT NULL,
  version INTEGER NOT NULL,
  definition TEXT NOT NULL,
  PRIMARY KEY (id, version)
);
CREATE TABLE review_artifacts (
  id TEXT PRIMARY KEY,
  media_type TEXT NOT NULL,
  data BLOB NOT NULL
);
CREATE TABLE item_answers (
  seq INTEGER PRIMARY KEY AUTOINCREMENT,
  id TEXT NOT NULL UNIQUE,
  item_id TEXT NOT NULL REFERENCES items(id),
  review_id TEXT NOT NULL,
  supersedes TEXT REFERENCES item_answers(id),
  response TEXT NOT NULL
);
CREATE INDEX item_answers_item ON item_answers(item_id, seq);
CREATE UNIQUE INDEX item_answers_supersedes ON item_answers(supersedes) WHERE supersedes IS NOT NULL;

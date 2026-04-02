CREATE TABLE IF NOT EXISTS users (
	id BIGINT PRIMARY KEY,
	name TEXT NOT NULL,
	created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

INSERT INTO users (id, name, created_at)
VALUES
	(1, 'Ada Lovelace', '2026-04-02T00:00:00Z'),
	(2, 'Grace Hopper', '2026-04-02T00:00:00Z'),
	(3, 'Linus Torvalds', '2026-04-02T00:00:00Z')
ON CONFLICT (id) DO NOTHING;
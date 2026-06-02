package eventlog

const createEventsTableSQL = `
CREATE TABLE IF NOT EXISTS events (
	id INTEGER PRIMARY KEY AUTOINCREMENT,
	run_id TEXT NOT NULL,
	type TEXT NOT NULL,
	payload BLOB NOT NULL,
	created_at TEXT NOT NULL
);

CREATE INDEX IF NOT EXISTS idx_events_run_id_id ON events(run_id, id);
`

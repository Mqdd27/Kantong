-- emails whose transaction was deleted by hand: never re-import them
CREATE TABLE deleted_refs (ref TEXT PRIMARY KEY);

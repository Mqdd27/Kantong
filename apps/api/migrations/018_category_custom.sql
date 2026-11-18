-- Categories created by the user (POST /api/categories) can be renamed; the built-in ones cannot,
-- because code and the Hermes skills refer to their names (Investasi, Penyesuaian, Lainnya, ...).
ALTER TABLE categories ADD COLUMN custom INTEGER NOT NULL DEFAULT 0;

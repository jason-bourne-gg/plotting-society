-- Where the project physically is.
--
-- An owner in Bengaluru and a buyer comparing sites both want to see the land
-- on a map, not read "Rui & Banwadi" and go and search for it themselves.
--
-- Nullable on purpose: coordinates are the one thing here nobody should
-- guess. Until the site office sets them the app shows the address and a
-- search link, which is always correct, rather than a pin in the wrong field.

ALTER TABLE societies
    ADD COLUMN latitude   numeric(9,6),
    ADD COLUMN longitude  numeric(9,6),
    -- What to call the pin, e.g. "Sandesh Nagari 7, Rui & Banwadi"
    ADD COLUMN map_label  text,
    -- Free-text "10 min to AIIMS" style list for the guest page
    ADD COLUMN landmarks  jsonb NOT NULL DEFAULT '[]'::jsonb;

COMMENT ON COLUMN societies.latitude IS
    'Approximate site centre. Null until the site office confirms it — never guessed.';

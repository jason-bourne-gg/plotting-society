-- Maintenance rates move out of societies and into their own table, because
-- the site office sets them per sector — a Sector 01 plot on a 15 M road does
-- not carry the same upkeep as one on a 9 M internal road.
--
-- A row with sector IS NULL is the society-wide fallback, used for any sector
-- that has no rate of its own.

CREATE TABLE maintenance_rates (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    society_id    uuid NOT NULL REFERENCES societies(id) ON DELETE CASCADE,
    sector        text,
    rate_per_sqft numeric(10,2) NOT NULL CHECK (rate_per_sqft >= 0),
    updated_by    uuid REFERENCES users(id) ON DELETE SET NULL,
    created_at    timestamptz NOT NULL DEFAULT now(),
    updated_at    timestamptz NOT NULL DEFAULT now()
);

-- One rate per sector, and one fallback. A partial index is needed because
-- NULL is not equal to itself in a plain UNIQUE constraint, so without this a
-- society could collect several competing fallbacks.
CREATE UNIQUE INDEX maintenance_rates_sector_idx
    ON maintenance_rates (society_id, sector) WHERE sector IS NOT NULL;
CREATE UNIQUE INDEX maintenance_rates_default_idx
    ON maintenance_rates (society_id) WHERE sector IS NULL;

-- Carry across whatever each society was already charging.
INSERT INTO maintenance_rates (society_id, sector, rate_per_sqft)
SELECT id, NULL, maintenance_rate_per_sqft FROM societies;

ALTER TABLE societies DROP COLUMN maintenance_rate_per_sqft;

-- The sector a bill was raised for, alongside the rate and area already
-- snapshotted on it. Together these make a bill fully self-explaining.
ALTER TABLE maintenance_dues ADD COLUMN sector text;

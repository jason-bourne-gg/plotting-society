-- Maintenance is a one-time charge per plot, priced per square foot, so a
-- 1,130 sq ft plot and a 3,422 sq ft plot pay proportionally.

ALTER TABLE societies
    ADD COLUMN maintenance_rate_per_sqft numeric(10,2) NOT NULL DEFAULT 5.00;

COMMENT ON COLUMN societies.maintenance_rate_per_sqft IS
    'Current one-time maintenance rate in rupees per square foot.';

-- A bill records what it was computed from, not just the total. The rate on the
-- society can change; an invoice already raised must not silently change with
-- it, and an owner querying the amount has to be shown the working.
ALTER TABLE maintenance_dues
    ADD COLUMN rate_per_sqft numeric(10,2),
    ADD COLUMN area_sqft     numeric(10,2);

COMMENT ON COLUMN maintenance_dues.rate_per_sqft IS
    'Rate this bill was raised at. Snapshot, never back-filled from the society.';
COMMENT ON COLUMN maintenance_dues.area_sqft IS
    'Plot area this bill was raised against. Snapshot, for the same reason.';

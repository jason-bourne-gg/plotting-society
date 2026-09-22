-- Guest access: anyone with the public link browses the layout without an
-- account. When they ask about a plot, that becomes a lead in the builder's
-- inbox — which is the commercial reason for the builder to share the link.

CREATE TABLE enquiries (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    society_id  uuid NOT NULL REFERENCES societies(id) ON DELETE CASCADE,
    plot_id     uuid REFERENCES plots(id) ON DELETE SET NULL,
    name        text NOT NULL,
    phone       text NOT NULL,
    email       text,
    message     text,
    budget      text,
    -- how the lead arrived: 'layout_map', 'plot_detail', 'brochure'
    source      text NOT NULL DEFAULT 'layout_map',
    status      text NOT NULL DEFAULT 'new'
                CHECK (status IN ('new','contacted','visit_scheduled','converted','lost')),
    handled_by  uuid REFERENCES users(id) ON DELETE SET NULL,
    notes       text,
    -- coarse client fingerprint, only for spam triage; never shown in the UI
    client_hash text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX enquiries_society_status_idx ON enquiries(society_id, status, created_at DESC);
CREATE INDEX enquiries_plot_idx ON enquiries(plot_id) WHERE plot_id IS NOT NULL;

-- Marketing copy for the public page, kept in the database so the site office
-- can change it without a deploy.
ALTER TABLE societies
    ADD COLUMN tagline          text,
    ADD COLUMN highlights       jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN amenities        jsonb NOT NULL DEFAULT '[]'::jsonb,
    ADD COLUMN brochure_url     text,
    ADD COLUMN contact_phone    text,
    ADD COLUMN contact_email    text,
    -- guests see the layout only while this is true
    ADD COLUMN public_listing   boolean NOT NULL DEFAULT true;

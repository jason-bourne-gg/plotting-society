-- Plotting Society: initial schema.
--
-- Money is numeric(14,2) everywhere, never float.
-- The fund ledger is append-only: corrections are reversal rows, never UPDATEs,
-- so an owner disputing a number can always be shown the full history.

CREATE EXTENSION IF NOT EXISTS pgcrypto;

-- ---------------------------------------------------------------- builders

CREATE TABLE builders (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    name        text NOT NULL,
    slug        text NOT NULL UNIQUE,
    phone       text,
    email       text,
    logo_url    text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

-- ------------------------------------------------------------------- users
-- One table for everybody. `role` decides what they can reach; `builder_id`
-- is set for staff and null for plot owners.

CREATE TABLE users (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    builder_id       uuid REFERENCES builders(id) ON DELETE CASCADE,
    email            text UNIQUE,
    phone            text,
    password_hash    text,
    name             text NOT NULL,
    role             text NOT NULL CHECK (role IN ('super_admin','builder_admin','builder_staff','owner')),
    avatar_url       text,
    current_address  text,
    city             text,
    -- owners opt in before their name/phone is visible to other owners
    directory_opt_in boolean NOT NULL DEFAULT false,
    is_active        boolean NOT NULL DEFAULT true,
    last_login_at    timestamptz,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT staff_needs_builder CHECK (
        role = 'owner' OR role = 'super_admin' OR builder_id IS NOT NULL
    )
);

CREATE INDEX users_builder_idx ON users(builder_id) WHERE builder_id IS NOT NULL;

-- Invites: an owner never self-registers. The builder adds the plot, invites
-- the owner, and the token binds that person to that plot on first login.
CREATE TABLE invites (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    email       text NOT NULL,
    name        text NOT NULL,
    role        text NOT NULL CHECK (role IN ('builder_admin','builder_staff','owner')),
    builder_id  uuid REFERENCES builders(id) ON DELETE CASCADE,
    plot_id     uuid,
    token_hash  text NOT NULL UNIQUE,
    expires_at  timestamptz NOT NULL,
    accepted_at timestamptz,
    created_by  uuid REFERENCES users(id),
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE refresh_tokens (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id     uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash  text NOT NULL UNIQUE,
    expires_at  timestamptz NOT NULL,
    revoked_at  timestamptz,
    user_agent  text,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX refresh_tokens_user_idx ON refresh_tokens(user_id);

-- --------------------------------------------------------------- societies

CREATE TABLE societies (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    builder_id       uuid NOT NULL REFERENCES builders(id) ON DELETE CASCADE,
    name             text NOT NULL,
    slug             text NOT NULL,
    city             text,
    address          text,
    -- the scanned layout plan the plot polygons are drawn on top of
    layout_image_url text,
    layout_width     integer,
    layout_height    integer,
    rera_number      text,
    created_at       timestamptz NOT NULL DEFAULT now(),
    updated_at       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (builder_id, slug)
);

-- ------------------------------------------------------------------- plots

CREATE TABLE plots (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    society_id  uuid NOT NULL REFERENCES societies(id) ON DELETE CASCADE,
    plot_no     text NOT NULL,
    phase       text,
    area_sqft   numeric(10,2),
    facing      text,
    is_corner   boolean NOT NULL DEFAULT false,
    status      text NOT NULL DEFAULT 'available'
                CHECK (status IN ('available','booked','sold','on_hold','disputed','not_for_sale')),
    price       numeric(14,2),
    owner_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    -- polygon on the layout image: {"points":[[x,y],...]} in layout pixel space
    map_shape   jsonb,
    notes       text,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (society_id, plot_no)
);

CREATE INDEX plots_society_status_idx ON plots(society_id, status);
CREATE INDEX plots_owner_idx ON plots(owner_id) WHERE owner_id IS NOT NULL;

ALTER TABLE invites
    ADD CONSTRAINT invites_plot_fk FOREIGN KEY (plot_id) REFERENCES plots(id) ON DELETE CASCADE;

-- -------------------------------------------------------------------- dues

CREATE TABLE maintenance_dues (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    plot_id      uuid NOT NULL REFERENCES plots(id) ON DELETE CASCADE,
    period_label text NOT NULL,                       -- 'FY2026-Q1', 'Apr 2026'
    amount_due   numeric(14,2) NOT NULL CHECK (amount_due >= 0),
    amount_paid  numeric(14,2) NOT NULL DEFAULT 0 CHECK (amount_paid >= 0),
    due_date     date,
    paid_on      date,
    payment_ref  text,
    receipt_url  text,
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (plot_id, period_label)
);

CREATE INDEX dues_plot_idx ON maintenance_dues(plot_id);

-- ------------------------------------------------------------ fund ledger
-- Append-only. To fix a mistake, insert a reversal row pointing at the
-- original via reverses_id. Nothing here is ever UPDATEd or DELETEd.

CREATE TABLE fund_entries (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    society_id   uuid NOT NULL REFERENCES societies(id) ON DELETE CASCADE,
    entry_date   date NOT NULL,
    head         text NOT NULL,                       -- 'security', 'roads', 'water'
    description  text NOT NULL,
    credit       numeric(14,2) NOT NULL DEFAULT 0 CHECK (credit >= 0),
    debit        numeric(14,2) NOT NULL DEFAULT 0 CHECK (debit  >= 0),
    document_url text,
    reverses_id  uuid REFERENCES fund_entries(id),
    created_by   uuid REFERENCES users(id),
    created_at   timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT one_sided CHECK ((credit > 0) <> (debit > 0))
);

CREATE INDEX fund_society_date_idx ON fund_entries(society_id, entry_date DESC);

-- ----------------------------------------------------------- site updates

CREATE TABLE site_updates (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    society_id   uuid NOT NULL REFERENCES societies(id) ON DELETE CASCADE,
    title        text NOT NULL,
    body         text,
    phase        text,
    published_at timestamptz,
    created_by   uuid REFERENCES users(id),
    created_at   timestamptz NOT NULL DEFAULT now(),
    updated_at   timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX updates_society_pub_idx ON site_updates(society_id, published_at DESC NULLS LAST);

CREATE TABLE site_update_media (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    update_id   uuid NOT NULL REFERENCES site_updates(id) ON DELETE CASCADE,
    url         text NOT NULL,
    caption     text,
    sort_order  integer NOT NULL DEFAULT 0,
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX update_media_update_idx ON site_update_media(update_id, sort_order);

-- ----------------------------------------------------------------- queries

CREATE TABLE queries (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    society_id  uuid NOT NULL REFERENCES societies(id) ON DELETE CASCADE,
    plot_id     uuid REFERENCES plots(id) ON DELETE SET NULL,
    raised_by   uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category    text NOT NULL CHECK (category IN (
                    'documents_legal','payments_dues','plot_condition','infrastructure',
                    'construction_noc','resale_transfer','site_visit','other')),
    subject     text NOT NULL,
    status      text NOT NULL DEFAULT 'open'
                CHECK (status IN ('open','in_progress','waiting_on_owner','resolved','closed')),
    priority    text NOT NULL DEFAULT 'normal' CHECK (priority IN ('low','normal','high')),
    assigned_to uuid REFERENCES users(id) ON DELETE SET NULL,
    -- the SLA clock the owner can see; this is the whole point of the module
    sla_due_at  timestamptz,
    resolved_at timestamptz,
    created_at  timestamptz NOT NULL DEFAULT now(),
    updated_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX queries_society_status_idx ON queries(society_id, status, created_at DESC);
CREATE INDEX queries_raised_by_idx ON queries(raised_by, created_at DESC);

CREATE TABLE query_messages (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    query_id       uuid NOT NULL REFERENCES queries(id) ON DELETE CASCADE,
    author_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    body           text NOT NULL,
    attachment_url text,
    -- staff-only notes stay off the owner's thread
    is_internal    boolean NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX query_messages_query_idx ON query_messages(query_id, created_at);

-- --------------------------------------------------------------- documents

CREATE TABLE documents (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    society_id  uuid NOT NULL REFERENCES societies(id) ON DELETE CASCADE,
    plot_id     uuid REFERENCES plots(id) ON DELETE CASCADE,
    doc_type    text NOT NULL CHECK (doc_type IN (
                    'sale_deed','noc','layout_plan','approval','receipt','agreement','other')),
    title       text NOT NULL,
    url         text NOT NULL,
    -- 'society' = every owner; 'plot' = that plot's owner only; 'staff' = builder only
    visibility  text NOT NULL DEFAULT 'plot' CHECK (visibility IN ('society','plot','staff')),
    uploaded_by uuid REFERENCES users(id),
    created_at  timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX documents_society_idx ON documents(society_id, visibility);
CREATE INDEX documents_plot_idx ON documents(plot_id) WHERE plot_id IS NOT NULL;

-- --------------------------------------------------------------- audit log
-- Anything touching money, ownership or plot status lands here.

CREATE TABLE audit_log (
    id         bigserial PRIMARY KEY,
    actor_id   uuid REFERENCES users(id) ON DELETE SET NULL,
    action     text NOT NULL,
    entity     text NOT NULL,
    entity_id  text,
    meta       jsonb,
    created_at timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX audit_entity_idx ON audit_log(entity, entity_id, created_at DESC);

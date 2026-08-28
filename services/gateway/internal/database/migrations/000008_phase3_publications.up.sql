CREATE TABLE mindcreek.kb_publications (
    id varchar(36) PRIMARY KEY,
    knowledge_base_id varchar(36) NOT NULL UNIQUE,
    publisher_id varchar(512) NOT NULL,
    publisher_tenant_id bigint NOT NULL,
    title varchar(160) NOT NULL,
    description varchar(2000) NOT NULL DEFAULT '',
    tags jsonb NOT NULL DEFAULT '[]'::jsonb,
    usage_guidance varchar(2000) NOT NULL DEFAULT '',
    audience_type varchar(32) NOT NULL,
    audience_config jsonb NOT NULL DEFAULT '{}'::jsonb,
    access_mode varchar(32) NOT NULL,
    status varchar(16) NOT NULL,
    published_revision bigint NOT NULL,
    created_at timestamptz NOT NULL,
    published_at timestamptz NOT NULL,
    unpublished_at timestamptz,
    updated_at timestamptz NOT NULL,
    row_version bigint NOT NULL DEFAULT 1,
    last_audit_correlation_id varchar(128) NOT NULL,
    CONSTRAINT kb_publications_publisher_tenant_positive CHECK (publisher_tenant_id > 0),
    CONSTRAINT kb_publications_audience_type CHECK (audience_type IN ('organization', 'workspace_set')),
    CONSTRAINT kb_publications_access_mode CHECK (access_mode IN ('subscriber', 'organization_public')),
    CONSTRAINT kb_publications_status CHECK (status IN ('published', 'unpublished')),
    CONSTRAINT kb_publications_revision_positive CHECK (published_revision > 0),
    CONSTRAINT kb_publications_row_version_positive CHECK (row_version > 0),
    CONSTRAINT kb_publications_tags_array CHECK (jsonb_typeof(tags) = 'array'),
    CONSTRAINT kb_publications_audience_object CHECK (jsonb_typeof(audience_config) = 'object'),
    CONSTRAINT kb_publications_time_order CHECK (updated_at >= created_at AND published_at >= created_at),
    CONSTRAINT kb_publications_unpublished_time CHECK (
        (status = 'published' AND unpublished_at IS NULL) OR
        (status = 'unpublished' AND unpublished_at IS NOT NULL AND unpublished_at >= published_at)
    )
);

CREATE INDEX kb_publications_catalog
    ON mindcreek.kb_publications (status, updated_at DESC, id);

CREATE INDEX kb_publications_publisher
    ON mindcreek.kb_publications (publisher_id, status, updated_at DESC);

CREATE TABLE mindcreek.kb_subscriptions (
    id varchar(36) PRIMARY KEY,
    publication_id varchar(36) NOT NULL REFERENCES mindcreek.kb_publications(id) ON DELETE RESTRICT,
    subscriber_id varchar(512) NOT NULL,
    subscriber_tenant_id bigint NOT NULL,
    status varchar(16) NOT NULL,
    notification_enabled boolean NOT NULL DEFAULT true,
    last_seen_revision bigint NOT NULL,
    created_at timestamptz NOT NULL,
    updated_at timestamptz NOT NULL,
    ended_at timestamptz,
    last_audit_correlation_id varchar(128) NOT NULL,
    CONSTRAINT kb_subscriptions_identity_unique UNIQUE (publication_id, subscriber_id),
    CONSTRAINT kb_subscriptions_tenant_positive CHECK (subscriber_tenant_id > 0),
    CONSTRAINT kb_subscriptions_status CHECK (status IN ('active', 'inactive', 'unsubscribed')),
    CONSTRAINT kb_subscriptions_revision_nonnegative CHECK (last_seen_revision >= 0),
    CONSTRAINT kb_subscriptions_time_order CHECK (updated_at >= created_at),
    CONSTRAINT kb_subscriptions_ended_state CHECK (
        (status = 'active' AND ended_at IS NULL) OR
        (status IN ('inactive', 'unsubscribed') AND ended_at IS NOT NULL AND ended_at >= created_at)
    )
);

CREATE INDEX kb_subscriptions_user_active
    ON mindcreek.kb_subscriptions (subscriber_id, updated_at DESC)
    WHERE status = 'active';

CREATE INDEX kb_subscriptions_publication_active
    ON mindcreek.kb_subscriptions (publication_id, subscriber_id)
    WHERE status = 'active';

CREATE TABLE mindcreek.kb_content_revisions (
    knowledge_base_id varchar(36) PRIMARY KEY,
    content_revision bigint NOT NULL,
    updated_at timestamptz NOT NULL,
    CONSTRAINT kb_content_revisions_positive CHECK (content_revision > 0)
);

CREATE TABLE mindcreek.kb_activity_events (
    id varchar(36) PRIMARY KEY,
    knowledge_base_id varchar(36) NOT NULL,
    actor_id varchar(512),
    event_type varchar(64) NOT NULL,
    content_revision bigint NOT NULL,
    summary varchar(500) NOT NULL DEFAULT '',
    correlation_id varchar(128) NOT NULL,
    created_at timestamptz NOT NULL,
    CONSTRAINT kb_activity_revision_positive CHECK (content_revision > 0)
);

CREATE INDEX kb_activity_events_kb_revision
    ON mindcreek.kb_activity_events (knowledge_base_id, content_revision DESC, created_at DESC);

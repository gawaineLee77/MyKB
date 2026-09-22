-- No credentials, tokens or imported historical sessions.
CREATE TABLE mindcreek.assistant_sessions (
    session_id varchar(128) PRIMARY KEY REFERENCES mindcreek.native_session_bindings(session_id),
    channel_id varchar(128) NOT NULL,
    agent_id varchar(128) NOT NULL,
    host_origin varchar(512) NOT NULL,
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX assistant_sessions_channel ON mindcreek.assistant_sessions(channel_id, created_at DESC);
CREATE TABLE mindcreek.assistant_rate_windows (
    bucket varchar(512) PRIMARY KEY,
    used integer NOT NULL CHECK (used > 0),
    expires_at timestamptz NOT NULL
);

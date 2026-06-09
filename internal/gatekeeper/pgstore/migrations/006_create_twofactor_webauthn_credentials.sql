-- 006_create_twofactor_webauthn_credentials.sql
-- WebAuthn/FIDO2 credential storage for phishing-resistant authentication.

CREATE TABLE IF NOT EXISTS twofactor_webauthn_credentials (
    id               BYTEA        NOT NULL PRIMARY KEY,  -- Raw credential ID bytes
    user_id          VARCHAR(36)  NOT NULL,               -- IAM user UUID
    public_key       BYTEA        NOT NULL,               -- COSE-encoded public key
    attestation_type VARCHAR(50)  NOT NULL DEFAULT 'none', -- 'none', 'basic', 'attca'
    aaguid           BYTEA        NOT NULL DEFAULT '',     -- Authenticator AAGUID (16 bytes)
    sign_count       INTEGER      NOT NULL DEFAULT 0,      -- Monotonic counter for clone detection
    clone_warning    BOOLEAN      NOT NULL DEFAULT FALSE,  -- True if sign count anomaly detected
    created_at       TIMESTAMP    NOT NULL DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_webauthn_credentials_user_id
    ON twofactor_webauthn_credentials (user_id);

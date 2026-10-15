ALTER TABLE proxy_profiles ADD COLUMN protocol TEXT NOT NULL DEFAULT 'socks5'
    CHECK (protocol IN ('http', 'https', 'socks4', 'socks5'));

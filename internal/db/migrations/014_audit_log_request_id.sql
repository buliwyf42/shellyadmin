-- Add a request-correlation column so audit entries can be tied back to the
-- HTTP request that triggered them. Existing rows keep an empty value; new
-- rows populated by the request-id middleware carry a stable 16-hex token
-- (or whatever the client supplied via X-Request-ID).
ALTER TABLE audit_log ADD COLUMN request_id TEXT NOT NULL DEFAULT '';

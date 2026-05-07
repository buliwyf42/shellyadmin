-- Risk level on audit entries. Populated only for action-execution rows
-- (currently set in services.ExecuteDeviceAction via context-bound
-- propagation — see risk_context.go). Other audit rows leave it empty.
--
-- Lets a future "show me every high-risk action in the last 30 days"
-- compliance query just SELECT against this column instead of regex-
-- parsing the message body.
ALTER TABLE audit_log ADD COLUMN risk_level TEXT NOT NULL DEFAULT '';

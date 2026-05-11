# ADR-0014: MCP-Agent Threat Model + Prompt-Injection Defense

- Status: `Accepted`
- Date: 2026-05-11
- Implements: post-v0.2.12 follow-up (Phase 4 / T5 from the consolidated review)
- Related: ADR-0011 (Read-Only MCP Server), ADR-0012 (STRIDE Threat Model)

## Context

ADR-0011 introduced the MCP server with state-changing tools gated
behind a `confirm: true` parameter. The contract is human-readable in
the tool description ("OPERATOR APPROVAL REQUIRED") and the agent is
expected to summarize + ask the operator before flipping confirm to
true. In practice this trust model has a sharp edge that ADR-0011 did
not formalize: **the agent itself decides when to set
`confirm: true`**. A prompt-injection in any input the agent ingests —
a Shelly device name, an audit-log row that was forwarded into the
chat, a markdown file the operator pasted, a webhook payload an
external system delivered — can manipulate the agent into bypassing
the confirmation step.

The Phase-4 expansion of the MCP feature surface (read-only fleet
state via MCP, audit-log query tools, device-detail dumps) increases
the input attack surface. A confirm-gate is not a defense if the
gating condition is set by the same untrusted-input-influenced agent
that operates the tool.

## Decision

### Threat Model

| Threat | Vector | Current Defense | Gap |
|---|---|---|---|
| Indirect prompt injection via device name | Operator-controlled, but a compromised Shelly firmware could write its own name and exfiltrate via `list_devices` → agent → confirm-bypass | Device-name sanitisation does not exist; the agent sees raw text | **High** |
| Audit-log poisoning | Future audit-log MCP tool could surface logs containing attacker-controlled strings (HTTP body, URL params already in audit messages) | `SanitizeLogMessage` redacts secrets but not prompt-injection markers | **High** |
| Tool-description manipulation | Static — tools are registered at startup | Source is in-tree, reviewed at PR time | **Low** |
| Stolen MCP token, agent gone rogue | Operator believes the agent is theirs, attacker speaks via the same connection | Rate-limit + audit-log of every call; no anomaly detection | **Medium** |
| Auto-confirm on "low-risk" tools | Risk level is per-tool, not per-target; an agent might auto-confirm `refresh_device` on every device of a type | Risk catalog exists; auto-confirm is a client-side policy | **Medium** |

### Defenses (Phased)

**Now (no code change, documentation only):**

1. **Operator-side rule**: the agent must echo a one-line summary of
   the planned action ("I am about to reboot device shelly-pro-4-pm
   on 192.168.1.42 because…") and wait for an explicit "yes" from the
   operator before setting `confirm: true`. The MCP tool description
   already requires this; this ADR makes it the operator's enforceable
   expectation rather than a polite suggestion.
2. **No automated MCP execution from untrusted sources**. The agent
   may be reachable from operator chat (Claude Desktop, Claude Code).
   It must NOT run from a cron job, GitHub Action, or webhook handler.
   The trust boundary is the human in the loop.

**Phase 4 (post-v0.3):**

3. **Device-name sanitisation at the MCP boundary**. Before serving
   `list_devices` / `get_device`, strip control characters and tokens
   matching common prompt-injection patterns (`<|im_start|>`,
   `<|system|>`, `Ignore previous instructions`, `ASSISTANT:`,
   `### Instruction`). Length-cap to 64 chars. Pre-existing scanner-
   strictness rule (see Memory: `feedback_scanner_strictness`)
   already enforces device validity but not text-content hygiene.
4. **Audit-log redaction for MCP**. Future audit-log MCP tool must
   replace operator-controlled fields (URL, body, params) with
   placeholder tokens (`<url>`, `<body>`, `<param>`) before serving
   to an agent. The full row stays in the SQLite audit_log for
   non-MCP review.
5. **Per-tool execution-source restriction**. State-changing MCP
   tools should refuse a request whose source IP is outside an
   operator-configurable allowlist (in addition to the token check).
   This raises the bar against a stolen token used from a different
   machine.

**Phase 5 / strategic:**

6. **Two-confirmation mode** for the highest-risk tools
   (firmware_install, bulk_action on >N targets). After the agent
   issues confirm=true, the server emits a webhook to a separate
   operator channel (Slack, email, mobile push) and waits for an
   out-of-band ACK before the underlying action runs. Defeats the
   single-channel-compromised case.

### What This ADR Does NOT Try to Cover

- **Adversarial fine-tuning** of the agent model itself (out of scope —
  the operator chooses the model).
- **Side-channel leaks** through the agent's response stream into the
  operator's chat history (operator-side concern; documented in
  ADR-0001 trust-boundary section).
- **Replay attacks** with captured MCP requests — already mitigated
  by the request_id audit row + session-bound state.

## Consequences

**Positive**

- The defense rationale for each ADR-0011 confirm-gate is now
  documented; future MCP tool additions have a checklist to follow.
- The operator-side rule (echo + wait for "yes") is now an
  enforceable expectation rather than implicit etiquette.
- Phase-4 work items 3-5 are queued with concrete acceptance
  criteria; they aren't "we should think about this".

**Negative**

- The "echo + wait" rule depends on operator discipline. An operator
  who routinely says "yes go ahead" without reading is back to the
  original threat surface.
- Device-name sanitisation has false-positive risk — a legitimate
  device name "Living Room TV (system test)" gets the `system`
  flagged. The list will need tuning against real-world fleet data.

**Mitigations**

- Operator-side rule is paired with a clear UI cue in MCP
  responses: the action summary is always shown, the operator's
  decision is always logged as a separate audit row with risk
  level "operator_confirm".
- Sanitisation tuning lives behind a config flag for the first
  release; operators can opt out per-pattern if their fleet
  triggers false positives.

## Related Work

- ADR-0011 §"Confirm-flow contract" describes the
  preview/confirmed dance.
- ADR-0012 §"Information Disclosure" notes the MCP-token-in-logs
  caveat that this ADR extends to general agent-context leakage.

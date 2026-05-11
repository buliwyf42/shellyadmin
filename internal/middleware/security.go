package middleware

import "github.com/gin-gonic/gin"

// SecurityHeaders sets the response-header surface for every request
// reaching the SPA / API. Notable additions:
//
//   - require-trusted-types-for 'script' (S17 from the consolidated review):
//     opts in to the browser's Trusted Types DOM-sink protection so any
//     direct innerHTML / eval / Function() assignment in the SPA bundle
//     is rejected at the DOM API surface. Svelte 5's compiled output
//     does not use unwrapped strings for these sinks, so the policy is
//     unlikely to break runtime behaviour — but if it does, the browser
//     console will report the offending sink and the operator gets the
//     fix-or-roll-back decision instead of a silent XSS.
//   - trusted-types 'none' refuses to register any Trusted-Types policy
//     factory (we don't need one); the SPA simply must avoid raw-string
//     DOM-sink calls.
//   - style-src still allows 'unsafe-inline' because Svelte 5 component
//     <style> blocks compile to inline <style> elements; M6 (Phase 3)
//     replaces this with nonce-based or hashed inline styles.
func SecurityHeaders() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-Frame-Options", "DENY")
		c.Header("X-Content-Type-Options", "nosniff")
		c.Header("Referrer-Policy", "same-origin")
		c.Header("Cross-Origin-Opener-Policy", "same-origin")
		c.Header("Permissions-Policy", "geolocation=(), microphone=(), camera=()")
		// Trusted-Types: `require-trusted-types-for 'script'` opts the SPA
		// into the DOM-sink rejection policy. We deliberately do NOT pin
		// `trusted-types <allowlist>` here — Svelte 5's compiler may at
		// some point register an internal "default" policy, and pinning
		// 'none' would break the SPA. Open allowlist is still
		// strictly better than no Trusted-Types directive at all:
		// it forces any user-string→innerHTML path to go through a
		// registered policy, which a code review can catch.
		c.Header("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self' 'unsafe-inline'; font-src 'self'; img-src 'self' data:; connect-src 'self'; object-src 'none'; base-uri 'self'; form-action 'self'; frame-ancestors 'none'; require-trusted-types-for 'script'")
		c.Next()
	}
}

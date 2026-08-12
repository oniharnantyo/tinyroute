# Security Rules

## Mandatory Security Checks

Before ANY commit:
- [ ] No hardcoded secrets (API keys, passwords, tokens)
- [ ] All user inputs validated
- [ ] SQL injection prevention (parameterized queries)
- [ ] XSS prevention (sanitized HTML)
- [ ] CSRF protection enabled
- [ ] Authentication/authorization verified
- [ ] Rate limiting on all endpoints
- [ ] Error messages don't leak sensitive data

## Secret Management

```typescript
// NEVER: Hardcoded secrets
const apiKey = "sk-proj-xxxxx"

// ALWAYS: Environment variables
const apiKey = process.env.API_KEY
if (!apiKey) throw new Error('API_KEY not configured')
```

## Security Response Protocol

If security issue found:
1. STOP immediately
2. Use `security-reviewer` agent
3. Fix CRITICAL issues before continuing
4. Rotate any exposed secrets
5. Review entire codebase for similar issues

## Custodian Storage Rules

- **`credentials.json` Permissions**: File must always be written with strict POSIX permissions `0600` (`-rw-------`).
- **Atomic Persistence**: Always use `tmp+rename` pattern to write credential files to prevent partial reads or corrupt state.
- **Zero Plaintext Leakage**:
  - Plaintext tokens (refresh tokens, access tokens) MUST NEVER be logged to `requests.jsonl`, stdout, or stderr.
  - CLI commands (`tinyroute auth status`, `tinyroute provider list`) MUST mask token values (`connected (expires ...)`).
  - Auth error responses sent to proxy clients MUST NOT return upstream refresh tokens or client secrets.

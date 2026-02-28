# Logging Specification

This document defines the logging rules for Babylon Tower. All contributors must follow these conventions when adding or modifying log statements.

## Library

Use `github.com/ipfs/go-log/v2` exclusively. Never use `fmt.Printf`, `fmt.Println`, or the standard `log` package for application logging. (`fmt.Print*` is acceptable only for direct user-facing CLI output such as help text or identity mnemonic display.)

## Logger Declaration

Each package declares **exactly one** logger variable at package level:

```go
var logger = log.Logger("babylontower/<package>")
```

**Naming rules:**
- The variable name is always `logger` — no prefixes, no suffixes.
- The subsystem string follows the pattern `"babylontower/<package>"` using the directory name under `pkg/`.
- One logger per package. Do not register multiple subsystem names within the same package.
- Do not create sub-package loggers like `"babylontower/rtc/signaling"`. Use `"babylontower/rtc"` for the entire `rtc` package.

**Registered subsystems** (exhaustive list — add new entries here and in `configureLogging` when creating a new package):

| Package | Subsystem |
|---------|-----------|
| `cmd/messenger` | `babylontower` |
| `pkg/cli` | `babylontower/cli` |
| `pkg/storage` | `babylontower/storage` |
| `pkg/identity` | `babylontower/identity` |
| `pkg/ipfsnode` | `babylontower/ipfsnode` |
| `pkg/messaging` | `babylontower/messaging` |
| `pkg/peerstore` | `babylontower/peerstore` |
| `pkg/multidevice` | `babylontower/multidevice` |
| `pkg/rtc` | `babylontower/rtc` |
| `pkg/groups` | `babylontower/groups` |
| `pkg/mailbox` | `babylontower/mailbox` |
| `pkg/reputation` | `babylontower/reputation` |
| `pkg/protocol` | `babylontower/protocol` |
| `pkg/ratchet` | `babylontower/ratchet` |

When adding a new package with logging, register the subsystem in the `configureLogging` function in `cmd/messenger/main.go`.

## Log Style: Structured Only

**Always use the structured `*w` variants.** Never use `*f` (printf-style) variants.

```go
// CORRECT — structured key-value pairs
logger.Debugw("peer connected", "peer", peerID, "addr", addr)
logger.Infow("message sent", "cid", result.CID, "recipient", pubkeyHex[:16])
logger.Warnw("failed to store message", "error", err)
logger.Errorw("subscription failed", "topic", topic, "error", err)

// WRONG — printf-style, not machine-parseable
logger.Debugf("peer %s connected at %s", peerID, addr)
logger.Infof("Published identity document to DHT: %s", dhtKey)
```

For lifecycle messages with no useful fields, the unstructured form is acceptable:

```go
logger.Info("storage initialized")
logger.Info("shutting down")
```

## Log Levels

| Level | When to use | Examples |
|-------|-------------|---------|
| `Error` | Unrecoverable failures in the current operation | Subscription loop failure, critical protocol error |
| `Warn` | Recoverable problems, degraded functionality | Storage write failed but message was still sent, config validation fell back to defaults |
| `Info` | Lifecycle events, significant state transitions | Service start/stop, identity loaded, bootstrap complete |
| `Debug` | Operational details useful for development | Peer connect/disconnect, message send/receive, DHT queries, envelope processing |

**Guidelines:**
- Most log statements should be `Debug`. Production runs at `warn` by default.
- `Info` is reserved for events that happen at most a few times per session (start, stop, identity load). Not for per-message or per-peer events.
- `Error` is rare. Most errors should be returned to the caller, not logged. Only log at `Error` in goroutine roots where there is no caller to return to.
- Never log at `Error` or `Warn` for expected conditions (e.g., peer offline, DHT lookup miss).

## Security: What Must Never Be Logged

**Never log any of the following at any level:**

- Plaintext message content (text, media bytes, reactions)
- Mnemonic phrases or seed words
- Private keys (Ed25519, X25519, ephemeral, device)
- Shared secrets, root keys, chain keys, message keys
- Decrypted payloads of any kind
- Full public keys (use truncated hex — see below)

**Safe alternatives:**

```go
// Log lengths, not content
logger.Debugw("message encrypted", "plaintext_len", len(plaintext))

// Log truncated public key prefixes (first 8 bytes = 16 hex chars)
logger.Debugw("building envelope", "recipient", fmt.Sprintf("%x", pubkey[:8]))

// Log boolean confirmations
logger.Debugw("shared secret computed", "success", true)

// Log CIDs, topic names, peer IDs (these are public identifiers)
logger.Debugw("published to topic", "topic", topicName, "cid", cid)
```

## Error Handling Patterns

### Return errors, don't log them

Functions that return errors should not also log them. The caller decides whether to log.

```go
// CORRECT — return the wrapped error
func (s *Service) SendMessage(...) error {
    if err := s.encrypt(...); err != nil {
        return fmt.Errorf("encrypt message: %w", err)
    }
    return nil
}

// WRONG — double logging when caller also logs
func (s *Service) SendMessage(...) error {
    if err := s.encrypt(...); err != nil {
        logger.Errorw("failed to encrypt", "error", err)  // DON'T
        return fmt.Errorf("encrypt message: %w", err)
    }
    return nil
}
```

### Log-and-swallow for non-critical side effects

When an error is intentionally not returned (the operation should continue), log at `Warn` with a comment explaining why:

```go
if err := s.storage.AddMessage(key, envelope); err != nil {
    logger.Warnw("failed to store received message", "error", err)
    // Don't fail the receive if storage fails — message was already delivered
}
```

### Goroutine roots: log at Error

In goroutine subscription loops or background tasks where there is no caller, log at `Error`:

```go
go func() {
    for msg := range sub.Messages() {
        if err := s.processMessage(msg); err != nil {
            logger.Errorw("failed to process message", "error", err, "from", msg.From)
        }
    }
}()
```

## Structured Field Naming Conventions

Use consistent, lowercase, underscore-separated key names:

| Key | Type | Meaning |
|-----|------|---------|
| `"error"` | `error` | Error value |
| `"peer"` | `string` | Peer ID |
| `"topic"` | `string` | PubSub topic name |
| `"size"` | `int` | Byte size of data |
| `"count"` | `int` | Number of items |
| `"cid"` | `string` | Content identifier |
| `"addr"` | `string` | Multiaddr or network address |
| `"version"` | `string` | Version string |
| `"data_dir"` | `string` | Data directory path |
| `"public_key"` | `string` | Truncated hex public key |
| `"device"` | `string` | Device ID |
| `"identity"` | `string` | Truncated hex identity key |
| `"reason"` | `string` | Human-readable reason |
| `"timeout"` | `duration` | Timeout duration |
| `"attempt"` | `int` | Retry attempt number |
| `"interval"` | `duration` | Time interval |

Do not embed user-facing advisory text in log calls. Messages like "Check your internet connection" or "Check firewall settings" belong in CLI output (`fmt.Println`), not in the structured logger.

## Configuration

Logging is configured in `cmd/messenger/main.go:configureLogging()`:

- **Priority:** CLI flag (`-log-level`) > env var (`BABYLONTOWER_LOG_LEVEL`) > default (`warn`)
- **File output:** `-log-file <path>` or `BABYLONTOWER_LOG_FILE`
- **Base level:** All subsystems default to `error`; babylontower subsystems are overridden to the configured level
- **Libp2p:** Automatically quieted to `error` (or `warn` when app level is `debug`). Users can override via `GOLOG_LOG_LEVEL`.

## Checklist for Code Review

When reviewing logging in PRs, verify:

- [ ] Uses `*w` structured calls (not `*f`)
- [ ] Logger variable is named `logger` and uses correct subsystem string
- [ ] No plaintext content, keys, secrets, or mnemonics in any log statement
- [ ] Public keys are truncated to 8 bytes in log output
- [ ] `Error` level is only used in goroutine roots, not in functions that return errors
- [ ] New subsystems are registered in `configureLogging`
- [ ] `Info` level is reserved for lifecycle events, not per-message operations
- [ ] Non-critical swallowed errors have explanatory comments

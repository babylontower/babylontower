# Multi-Instance Test Fix - HOME Environment Variable

## Problem

When running two Babylon Tower instances on the same Windows machine for testing, the second instance could not connect to the first with the following error:

```
❌ Error: Failed to connect: failed to connect to peer: failed to dial: 
failed to dial 12D3KooWGUUqiK6UHgcBtnTd4rzohrpJUpwCcvXg2nTXzb2PMA5t: 
all dials failed
  * [/ip4/192.168.1.126/tcp/61534] failed to negotiate security protocol: 
    read tcp4 192.168.1.126:63506->192.168.1.126:61534: 
    wsarecv: An existing connection was forcibly closed by the remote host.
```

### Root Cause

Both instances were generating the **same PeerID** because they shared the same IPFS repo directory (`~/.babylontower/ipfs`).

The launch scripts (`launch-instance1.sh` and `launch-instance2.sh`) set different `HOME` environment variables:
- Instance 1: `HOME=./test-data/instance1`
- Instance 2: `HOME=./test-data/instance2`

However, the code used `os.UserHomeDir()` which:
- **Ignores the `HOME` environment variable on Windows**
- Always returns the actual user's home directory (e.g., `C:\Users\vscode`)
- Caused both instances to load the same `peer.key` file
- Resulted in both instances having identical PeerIDs

When two libp2p nodes with the **same PeerID** try to connect:
1. The security handshake (noise/TLS) fails
2. Both sides present identical keys
3. libp2p rejects the connection as a "self-connection"
4. Error manifests as security protocol negotiation failure

## Solution

Modified two functions to respect the `HOME` environment variable:

### 1. `pkg/ipfsnode/node.go` - `expandPath()`

```go
// expandPath expands ~ to home directory
// Respects HOME environment variable for container/test isolation
func expandPath(path string) (string, error) {
	if len(path) == 0 {
		return "", fmt.Errorf("empty path")
	}

	if path[0] == '~' {
		// Check HOME environment variable first (for test isolation)
		// This allows multiple instances to run with different HOME dirs
		home := os.Getenv("HOME")
		if home == "" {
			var err error
			home, err = os.UserHomeDir()
			if err != nil {
				return "", fmt.Errorf("failed to get home dir: %w", err)
			}
		}
		if len(path) > 1 {
			path = filepath.Join(home, path[1:])
		} else {
			path = home
		}
	}

	return filepath.Abs(path)
}
```

### 2. `cmd/messenger/main.go` - `run()`

```go
// Setup data directory
var dataDir string
if *dataDirFlag != "" {
	dataDir = *dataDirFlag
} else {
	// Respect HOME environment variable for test isolation
	// This allows multiple instances to run with different HOME dirs
	homeDir := os.Getenv("HOME")
	if homeDir == "" {
		var err error
		homeDir, err = os.UserHomeDir()
		if err != nil {
			return fmt.Errorf("failed to get home directory: %w", err)
		}
	}
	dataDir = filepath.Join(homeDir, DefaultDataDir)
}
```

## Testing

### Unit Tests

Added `TestExpandPathRespectsHomeEnv` to verify the fix:

```bash
go test ./pkg/ipfsnode/... -v -run TestExpandPathRespectsHomeEnv
```

### Manual Testing

**On Linux/macOS:**
```bash
# Terminal 1
./scripts/test/launch-instance1.sh

# Terminal 2
./scripts/test/launch-instance2.sh
```

**On Windows (PowerShell):**
```powershell
# Terminal 1
.\scripts\test\launch-instance1.ps1

# Terminal 2
.\scripts\test\launch-instance2.ps1
```

**On Windows (Command Prompt):**
```cmd
REM Terminal 1
scripts\test\launch-instance1.bat

REM Terminal 2
scripts\test\launch-instance2.bat
```

**Then:**
1. In each terminal, run `/myid` to get the PeerID
2. Verify the PeerIDs are **different**
3. Connect instances:
   ```
   /connect <peer2_multiaddr>
   ```

## Benefits

This fix enables:
- ✅ Multiple instances on the same machine for testing
- ✅ Proper isolation of test data
- ✅ Container-friendly deployment (respects HOME env)
- ✅ Backward compatibility (falls back to UserHomeDir if HOME not set)

## Files Modified

- `pkg/ipfsnode/node.go` - `expandPath()` function
- `cmd/messenger/main.go` - `run()` function
- `pkg/ipfsnode/node_test.go` - Added `TestExpandPathRespectsHomeEnv`
- `scripts/test/launch-instance1.sh` - Added USERPROFILE export
- `scripts/test/launch-instance2.sh` - Added USERPROFILE export
- `scripts/test/launch-instance1.ps1` - New PowerShell script for Windows
- `scripts/test/launch-instance2.ps1` - New PowerShell script for Windows
- `scripts/test/launch-instance1.bat` - New batch file for Windows
- `scripts/test/launch-instance2.bat` - New batch file for Windows
- `scripts/test/README.md` - Added troubleshooting section
- `scripts/test/FIX-MULTI-INSTANCE.md` - This documentation

## Related Issues

- IPFS mDNS fails in containerized environments (QWEN.md)
- Multiple instances sharing same peer key
- Security protocol negotiation failures on localhost

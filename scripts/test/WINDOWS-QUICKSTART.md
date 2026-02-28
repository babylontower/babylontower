# Babylon Tower - Windows Quick Start

## Running Multiple Instances on Windows

To run two Babylon Tower instances on the same Windows machine, you have three options:

### Option 1: PowerShell (Recommended)

```powershell
# Terminal 1 - Launch Alice
.\scripts\test\launch-instance1.ps1

# Terminal 2 - Launch Bob
.\scripts\test\launch-instance2.ps1
```

If you get an execution policy error, run:
```powershell
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass
```

### Option 2: Command Prompt (CMD)

```cmd
REM Terminal 1 - Launch Alice
scripts\test\launch-instance1.bat

REM Terminal 2 - Launch Bob
scripts\test\launch-instance2.bat
```

### Option 3: Git Bash / WSL

```bash
# Terminal 1 - Launch Alice
./scripts/test/launch-instance1.sh

# Terminal 2 - Launch Bob
./scripts/test/launch-instance2.sh
```

## Quick Test

1. **Launch both instances** using any method above

2. **Get PeerIDs** - In each terminal:
   ```
   >>> /myid
   ```

3. **Verify different PeerIDs** - They should be different!
   - Instance 1: `12D3KooWA...`
   - Instance 2: `12D3KooWB...`

4. **Connect instances**:
   ```
   >>> /connect /ip4/127.0.0.1/tcp/PORT/p2p/PEER_ID
   ```

5. **Add contacts and chat**:
   ```
   >>> /add <public_key> Alice
   >>> /chat 1
   ```

## Troubleshooting

### "Failed to negotiate security protocol"

This means both instances have the same PeerID. Fix:

1. Close both instances
2. Clean test data: `make clean-test` or delete `test-data\` folder
3. Rebuild: `make build`
4. Restart instances

### PowerShell execution policy error

```powershell
Set-ExecutionPolicy -Scope Process -ExecutionPolicy Bypass
```

### Port already in use

The instances automatically use different ports. If you still get port conflicts:
1. Make sure old instances are closed
2. Check Task Manager for any running `messenger.exe` processes
3. Kill them and restart

## Technical Details

The scripts set environment variables to isolate instances:
- `HOME` - Points to instance-specific data directory
- `USERPROFILE` - Windows-specific home directory variable

This ensures each instance has:
- Unique PeerID
- Separate identity files
- Independent storage

See `FIX-MULTI-INSTANCE.md` for more details.

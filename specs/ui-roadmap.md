# UI Application Roadmap — Babylon Tower

## Overview

This roadmap covers the UI journey from the current early prototype to a full-featured, polished messenger. The protocol layer (Phases 8-17) is complete — the UI is now the bottleneck for usability.

**Current state:** Onboarding flow works. Main app has 2-column layout (sidebar + chat) with Babylonian theme, async core init, and connection status bar. Settings accessible via gear icon overlay. Phase UI-1 (Core Messaging) complete. Phase UI-2 (Identity & Contact Verification) mostly complete — only QR code generation remains.

**Target:** A desktop messenger with UX comparable to Signal/Element, exposing the full Protocol v1 feature set through an immersive Babylonian-themed interface.

---

## Phase UI-1: Core Messaging (Foundation) — **Done**

All items complete. Working 1:1 text chat with contacts, conversations, offline delivery, and status bar indicator.

---

## Phase UI-2: Identity & Contact Verification — **Done**

All items complete except QR code generation (requires external dependency).

### Remaining

- [ ] QR code generation for contact link (for in-person sharing) — needs QR library

**Implemented:** Identity panel, fingerprint display, safety number comparison (12 groups of 5 digits), verified badge (persisted), key change notification (gold alert banner), mnemonic backup reminder (dismissable, persisted), contact discovery via DHT.

---

## Phase UI-3: Settings Persistence & Configuration — **Done**

All items complete except profile visibility to contacts (depends on identity document publication wiring in Phase 18).

### Remaining

- [ ] Profile visible to contacts (via identity document) — depends on identity document publication

**Implemented:** Settings read from `config.yaml` on open, save via `SaveAppConfig()`, validation with inline errors, "restart required" indicator for network changes, display name/device name persistence, dark/light mode persistence and immediate apply, font size persistence and apply, window size persistence and restore, bootstrap peers/listen addresses/connection limits/NAT traversal editors.

---

## Phase UI-4: Group Messaging — **Done** (core features)

All core private group management implemented. Public group discovery, moderation UI, channels, and group messaging wiring remain.

### Remaining

- [ ] Group message sending/receiving (requires wiring group messaging into PubSub)
- [ ] Group info editor (name, description — admin only, requires backend support)
- [ ] Key rotation indicator — subtle note when sender keys rotate
- [ ] Public group discovery (search by name or topic)
- [ ] Join/leave public groups
- [ ] Moderation UI: ban, mute, delete message (moderator/admin)
- [ ] Rate limit indicator (HashCash solving animation)
- [ ] Channel creation (owner-only posting)
- [ ] Subscribe/unsubscribe channels
- [ ] Channel post list (linked-list style, newest first)

**Implemented:** Sidebar CHATS/GROUPS tab navigation, create group dialog (name, description), group list with type icons (G/P/C/B), group info card with member list, member role display (Owner/Admin/Member), add/remove members (admin only), leave group (two-click confirm), delete group (owner, two-click confirm), group detail overlay panel, `UIGroupManager` app layer with GroupInfo/GroupMemberInfo DTOs.

---

## Phase UI-5: Rich Messages — **Done** (UI ready, backend wiring pending)

All UI components implemented. Backend wiring (sending/receiving these message types via PubSub) depends on Protocol v1 integration (Track A).

### Remaining

- [ ] Wire reaction/edit/delete/receipt/typing send to messaging layer (requires `outgoing.go` support for non-text DMPayload types)
- [ ] Full emoji picker for reactions (currently defaults to thumbs-up)
- [ ] Group receipts: "Read by N" expandable
- [ ] Failed message retry button

**Implemented:** Message context menu (click → copy/reply/edit/delete/react), reaction display under message tablets (pill chips with emoji + count), edit indicator ("edited" + timestamp), delete tombstone ("This inscription was erased"), reply/quote preview (gold bar + sender + quoted text), reply compose bar (with cancel), delivery status indicators (sending/sent/delivered/read/failed as checkmarks), typing indicator ("Name is typing...") with 5s auto-clear, typing debounce for outgoing events, `MessageStatus` enum (Sending/Sent/Delivered/Read/Failed), `IncomingEventType` enum for rich event routing, privacy toggles in Settings > Privacy (send read receipts, send typing indicators), `PrivacyConfig` in AppConfig with YAML persistence.

---

## Phase UI-6: Multi-Device

**Goal:** Expose device management and cross-device sync.

### Device Management Panel

- [ ] "My Devices" list (settings > devices)
- [ ] Current device highlighted
- [ ] Device info: name, ID (truncated), registration date, last seen
- [ ] Register new device flow (display link/QR on existing device, scan on new)
- [ ] Revoke device: confirmation dialog, triggers ratchet reset
- [ ] Device limit indicator (current / max devices from config)

### Sync Status

- [ ] Sync indicator in status bar: "Syncing..." / "In sync"
- [ ] Cross-device message delivery confirmation
- [ ] Conflict resolution notification (if split-brain detected)

### Fanout Visibility

- [ ] Show "sent to N devices" for outgoing messages (debug/info mode)
- [ ] Device-specific delivery status in message details

**Dependencies:** `pkg/multidevice` (DeviceManager, SyncManager, RevocationManager, FanoutManager)
**Outcome:** Seamless multi-device experience.

---

## Phase UI-7: Voice & Video Calls

**Goal:** Basic call interface using the RTC signaling infrastructure.

### Call UI

- [ ] Call button in chat header (phone icon)
- [ ] Incoming call overlay: caller name, accept/reject buttons
- [ ] Active call bar: duration timer, mute/unmute, end call
- [ ] Call states: ringing, connecting, active, ended (visual feedback for each)
- [ ] Call history in conversation (system messages: "Call lasted 5m 23s")

### Audio Controls

- [ ] Mute/unmute microphone
- [ ] Speaker/headphone selector (if OS supports)
- [ ] Audio level indicator

### Video (Stretch)

- [ ] Video call toggle (camera on/off)
- [ ] Video preview (self-view)
- [ ] Remote video display
- [ ] Picture-in-picture mode

### Group Calls

- [ ] Group call initiation
- [ ] Participant grid layout
- [ ] Active speaker highlighting

**Dependencies:** `pkg/rtc` (SessionManager, SignalingService, CallManager). Note: actual WebRTC media is stubbed — this phase wires the signaling; media integration requires WebRTC binding (pion/webrtc).
**Outcome:** Call signaling works; actual audio/video depends on WebRTC integration.

---

## Phase UI-8: Network & Reputation Visibility

**Goal:** Let power users see what's happening under the hood.

### Network Dashboard

- [ ] Network status panel (expandable from status bar or settings)
- [ ] Connected peers list with addresses, protocols, latency
- [ ] DHT routing table size and health
- [ ] Bootstrap status: IPFS DHT, Babylon DHT, Rendezvous
- [ ] Discovery statistics: peers by source (DHT, mDNS, PeerExchange, bootstrap)
- [ ] Bandwidth usage (if available from libp2p metrics)

### Reputation Viewer

- [ ] Peer reputation scores: 5-dimension radar chart or bar display
- [ ] Tier badges: Basic, Contributor, Reliable, Trusted
- [ ] Attestation log: who attested whom, when, score
- [ ] Own reputation score and tier

### Mailbox Status

- [ ] Mailbox indicator in status bar (if acting as mailbox node)
- [ ] Mailbox stats: stored messages, capacity, oldest/newest
- [ ] Manual "Refresh Mailbox" trigger
- [ ] Mailbox node discovery status

### Diagnostics

- [ ] Log viewer (filterable by subsystem, level)
- [ ] Metrics export (Prometheus format or JSON)
- [ ] "Copy debug info" for bug reports

**Dependencies:** `pkg/reputation`, `pkg/mailbox`, `pkg/ipfsnode` (metrics, diagnostics)
**Outcome:** Full observability for developers and power users.

---

## Phase UI-9: Media & File Sharing

**Goal:** Send and receive images, files, voice messages.

### Image Messages

- [ ] Image picker / paste from clipboard
- [ ] Image preview in chat (thumbnail clay tablet)
- [ ] Full-size image viewer (click to expand)
- [ ] Image encryption: encrypt → chunk → upload to IPFS → share CID

### File Sharing

- [ ] File attachment button
- [ ] File message display: name, size, download button
- [ ] Download progress indicator
- [ ] File type icons

### Voice Messages

- [ ] Hold-to-record voice message
- [ ] Playback controls (play/pause, waveform)
- [ ] Voice message compression (Opus codec)

### Storage

- [ ] Media cache management (settings: max cache size)
- [ ] Auto-download settings (WiFi only, always, never)

**Dependencies:** `pkg/ipfsnode` (IPFS Get/Put for content-addressed storage), chunking (H2 limitation must be resolved)
**Outcome:** Rich media messaging.

---

## Phase UI-10: Search & History

**Goal:** Find messages, contacts, and conversations efficiently.

### Message Search

- [ ] Global search bar (top of contacts/conversations column)
- [ ] Search results: message preview with contact name, timestamp, highlight
- [ ] Click result to jump to message in conversation
- [ ] Filter: by contact, date range, message type

### Conversation Search

- [ ] In-conversation search (Ctrl+F equivalent)
- [ ] Navigate between matches (up/down arrows)
- [ ] Highlight matching text in clay tablets

### Message Export

- [ ] Export conversation to text/JSON
- [ ] Date range selection for export
- [ ] Include/exclude media toggle

**Dependencies:** `pkg/storage` (message indexing — may need full-text search addition)
**Outcome:** Users can find anything in their message history.

---

## Phase UI-11: Security Hardening

**Goal:** Protect local data and provide security transparency.

### Encrypted Storage

- [ ] Passphrase/PIN setup on first run (derives storage encryption key)
- [ ] Lock screen after inactivity timeout
- [ ] Passphrase change flow
- [ ] Biometric unlock (OS-level, if available)

### Security Indicators

- [ ] Encryption badge on every conversation (always E2EE)
- [ ] Protocol version indicator (PoC vs Protocol v1)
- [ ] Session info: X3DH session established, ratchet state
- [ ] Key transparency log (future: public key directory)

### Privacy Controls

- [ ] Per-contact privacy settings (read receipts, typing, online status)
- [ ] "Disappearing messages" mode (auto-delete after timer)
- [ ] Screen security: block screenshots (OS-level)
- [ ] Incognito keyboard hint (mobile, future)

**Dependencies:** `pkg/storage` encryption (C1 limitation), `pkg/ratchet` session state
**Outcome:** Security-hardened messenger with user-visible trust indicators.

---

## Phase UI-12: Polish & Immersion

**Goal:** Elevate the Babylonian theme from functional to stunning.

### Visual Refinement

- [ ] Parallax background layers: city silhouette → tower → sky (per ui-design.md spec)
- [ ] Animated torch/lantern glow in dark mode (subtle, warm point lights)
- [ ] Starfield animation in sky layer (dark mode — Babylonian astronomy theme)
- [ ] Crescent moon (moon god Sin reference)
- [ ] Smooth transitions between screens (fade, slide)

### Custom Elements

- [ ] Babylonian-motif icons: winged bull (settings), palm (contacts), ziggurat (home)
- [ ] Cuneiform-inspired decorative borders (subtle)
- [ ] Scrollbar styled as column or rope
- [ ] Unread badge: gold clay seal shape
- [ ] Notification sound: clay tablet tap, reed instrument chime

### Typography

- [ ] Custom font with cuneiform-inspired letterforms (headers only)
- [ ] Body text remains highly readable (system font)
- [ ] Inscription-style formatting for timestamps and system messages

### Animations

- [ ] Message send: clay tablet "pressed" animation
- [ ] New message arrival: tablet "placed" animation with subtle dust
- [ ] Contact online: green flame ignites
- [ ] Typing indicator: stylus scratching animation

### Light Mode

- [ ] Full light theme implementation (daylight Babylon)
- [ ] Warm sandstone backgrounds
- [ ] Desert sky gradient
- [ ] Sun-baked clay tablets

**Dependencies:** None (visual-only), Gio animation capabilities
**Outcome:** A messenger that feels like messaging through ancient Babylon.

---

## Phase UI-13: Platform & Distribution

**Goal:** Package for real-world use.

### Desktop Packaging

- [ ] Windows installer (MSI/NSIS)
- [ ] macOS .app bundle + DMG
- [ ] Linux AppImage / Flatpak / Snap
- [ ] Auto-updater (check GitHub releases)

### System Integration

- [ ] System tray / menu bar icon
- [ ] Desktop notifications (OS-native)
- [ ] Start on login option
- [ ] Deep link handling (`btower://` protocol)
- [ ] File association (import backup files)

### Performance

- [ ] Startup time optimization (<2s to usable UI)
- [ ] Memory footprint profiling and optimization
- [ ] Lazy loading for long conversation histories
- [ ] Virtual scrolling for large contact/message lists

### Accessibility

- [ ] Keyboard navigation (Tab, arrow keys, shortcuts)
- [ ] Screen reader support (ARIA-equivalent for Gio)
- [ ] High contrast mode
- [ ] Configurable font sizes (already started in settings)

**Dependencies:** Build system, OS-specific packaging tools
**Outcome:** Ready for non-technical users to install and use.

---

## Phase Summary

| Phase | Name | Key Deliverable | Status |
|-------|------|-----------------|--------|
| UI-1 | Core Messaging | Working 1:1 text chat | **Done** |
| UI-2 | Identity & Verification | Trust verification between contacts | **Done** (QR pending) |
| UI-3 | Settings Persistence | Config that actually saves | **Done** |
| UI-4 | Group Messaging | Private/public groups, channels | **Done** (core; public/channels pending) |
| UI-5 | Rich Messages | Reactions, edits, receipts, typing | **Done** (UI; backend wiring pending) |
| UI-6 | Multi-Device | Device management, sync | Planned |
| UI-7 | Voice & Video | Call interface + signaling | Planned |
| UI-8 | Network & Reputation | Diagnostics, peer scoring UI | Planned |
| UI-9 | Media & Files | Images, files, voice messages | Planned |
| UI-10 | Search & History | Full-text search, export | Planned |
| UI-11 | Security Hardening | Encrypted storage, lock screen | Planned |
| UI-12 | Polish & Immersion | Parallax, animations, Babylonian icons | Planned |
| UI-13 | Platform & Distribution | Installers, system tray, auto-update | Planned |

---

## Parallel Tracks

Some work can proceed in parallel with the main phases:

**Track A: Protocol Wiring (Phase 18 continuation)**
- Wire X3DH + Double Ratchet into main message flow (replaces PoC ECDH)
- Wire Protocol v1 envelopes for all message types
- This unblocks UI-5, UI-6, UI-7

**Track B: WebRTC Integration**
- Integrate `pion/webrtc` for actual audio/video
- This unblocks UI-7 (actual media, not just signaling)

**Track C: IPFS Content Delivery**
- Complete IPFS Get with chunking (H2 limitation)
- This unblocks UI-9 (media messages)

---

## Design Principles

1. **Progressive disclosure** — Simple by default, powerful when needed. Network diagnostics and reputation scores are accessible but not in your face.

2. **Offline-first** — UI never blocks on network. Show cached data immediately, sync in background, indicate when offline.

3. **Security-visible** — Encryption is always on, but make it visible. Every conversation shows E2EE badge. Verification status is clear.

4. **Babylonian immersion** — The theme isn't a skin, it's the identity. Clay tablets, gold accents, ancient astronomy — every element reinforces the narrative.

5. **Performance** — Gio is immediate-mode: frame budget matters. Keep layouts simple, avoid deep nesting, cache expensive computations.

---

*Last updated: March 8, 2026*
*Covers: babylon-ui (Gio desktop application)*

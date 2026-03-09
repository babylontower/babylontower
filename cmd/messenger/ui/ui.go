package ui

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"gioui.org/app"
	"gioui.org/font/gofont"
	"gioui.org/layout"
	"gioui.org/op"
	"gioui.org/text"
	"gioui.org/unit"
	"gioui.org/widget"
	"gioui.org/widget/material"

	babylonapp "babylontower/pkg/app"
	"babylontower/pkg/config"
)

// ── Event Types ─────────────────────────────────────────────────────────────

type uiEventType int

const (
	evtIncomingMessage uiEventType = iota
	evtContactsChanged
	evtDataRefreshed    // background data refresh completed
	evtOfflineStarted   // offline message retrieval started
	evtOfflineCompleted // offline message retrieval completed
	evtGroupsRefreshed  // group list refresh completed
)

type uiEvent struct {
	Type    uiEventType
	Message *babylonapp.IncomingMessage
	Data    *refreshedData           // for evtDataRefreshed
	Groups  []*babylonapp.GroupInfo   // for evtGroupsRefreshed
}

// refreshedData holds results from background data refresh.
// The main goroutine swaps these in without doing any heavy work.
type refreshedData struct {
	contacts      []*babylonapp.ContactInfo
	conversations []*babylonapp.Conversation
	chatMessages  []*babylonapp.ChatMessage // for selected chat refresh
	chatKey       string                    // which contact the chatMessages are for
}

// ── Connection Status ───────────────────────────────────────────────────────

// ConnectionStatus tracks the current connection state for the status bar.
type ConnectionStatus struct {
	mu               sync.RWMutex
	CoreReady        bool
	CoreError        string
	IPFSConnected    bool
	BabylonConnected bool
	MessagingReady   bool
	ConnectedPeers   int
	BabylonPeers     int
	RendezvousActive bool
}

// ── App ─────────────────────────────────────────────────────────────────────

// App represents the Gio UI application.
type App struct {
	window *app.Window
	theme  *material.Theme
	ui     *UIState

	// Core application (may be nil during async init)
	coreApp    babylonapp.Application
	contactMgr babylonapp.ContactManager
	chatMgr    babylonapp.ChatManager

	// Context for cancellation
	ctx    context.Context
	cancel context.CancelFunc

	// ── Data model (written only on main goroutine via event drain) ──

	// Contacts from storage, enriched with online status
	contactList []*babylonapp.ContactInfo
	// Conversations sorted by last activity (for sidebar)
	conversations []*babylonapp.Conversation
	// Message cache keyed by contact PublicKeyBase58
	messageCache map[string][]*babylonapp.ChatMessage
	// Unread counts keyed by contact PublicKeyBase58
	unreadCounts map[string]int
	// Currently selected contact key (stable across list reorders)
	selectedKey string
	// Flag to scroll chat list to bottom on next frame
	scrollToBottom bool
	// Flag that conversations need refresh (batched, not per-message)
	conversationsDirty bool
	// Error to display in chat area (e.g., send failure)
	chatError string

	// Offline message retrieval status
	offlineRetrieving bool

	// Mnemonic backup reminder dismissed
	mnemonicReminderDismissed bool

	// Key change tracking: map[pubKeyBase58] -> "dismissed"
	keyChangeAlerts map[string]bool

	// ── Rich messages (Phase UI-5) ──
	// Typing state: contact key -> is typing
	typingContacts map[string]bool
	typingMu       sync.Mutex
	// Local typing debounce
	isLocalTyping  bool
	lastTypingEvent time.Time
	// Reply/edit compose state
	replyToMessage *babylonapp.ChatMessage
	editingMessage *babylonapp.ChatMessage

	// ── Group data ──
	// Sidebar tab: "chats" or "groups"
	sidebarTab string
	// Group list from UIGroupManager
	groupList []*babylonapp.GroupInfo
	// Selected group ID (hex)
	selectedGroupID string
	// Group UI manager
	uiGroupMgr babylonapp.UIGroupManager

	// Cross-goroutine event channel (goroutines write, main goroutine reads)
	uiEvents chan uiEvent

	// Connection status for the status bar (has its own mutex)
	status ConnectionStatus

	// Config persistence
	dataDir string
	appCfg  *config.AppConfig
}

// UIState holds widget state for the Gio layout.
type UIState struct {
	// Theme
	isDarkMode bool
	theme      *Theme

	// Widgets for lists
	contactList widget.List
	chatList    widget.List

	// Input fields
	messageInput widget.Editor
	searchInput  widget.Editor

	// Buttons
	sendButton widget.Clickable

	// Contact buttons (dynamically sized)
	contactButtons []widget.Clickable

	// Settings
	settingsBtn       widget.Clickable // gear button in sidebar header
	settingsScreen    *SettingsScreen
	showSettingsPanel bool

	// Window close button (custom, since OS decorations are hidden)
	closeBtn widget.Clickable

	// Add Contact dialog
	showAddContact    bool
	addContactInput   widget.Editor
	addContactError   string
	addContactBtn     widget.Clickable // "+" button in header
	addContactSubmit  widget.Clickable
	addContactCancel  widget.Clickable

	// Identity panel
	showIdentityPanel bool
	identityPanel     identityPanelState
	identityBtn       widget.Clickable // profile button in sidebar header

	// Contact detail panel
	showContactDetail bool
	contactDetail     contactDetailState
	chatHeaderBtn     widget.Clickable // clickable contact name in chat header

	// Mnemonic backup reminder
	mnemonicDismissBtn widget.Clickable

	// Key change alert dismiss
	keyChangeDismissBtn widget.Clickable

	// ── Rich message widgets (Phase UI-5) ──
	contextMenu      contextMenuState
	messageBubbleBtns [64]widget.Clickable // clickable message bubbles for context menu
	replyCancelBtn   widget.Clickable     // cancel reply bar

	// ── Group UI widgets ──
	// Sidebar tab buttons
	chatsTabBtn  widget.Clickable
	groupsTabBtn widget.Clickable

	// Group list
	groupList    widget.List
	groupButtons []widget.Clickable

	// Create group dialog
	showCreateGroup      bool
	createGroupName      widget.Editor
	createGroupDesc      widget.Editor
	createGroupSubmit    widget.Clickable
	createGroupCancel    widget.Clickable
	createGroupError     string
	createGroupBtn       widget.Clickable // "+" button in groups tab header

	// Group detail / member panel
	showGroupDetail   bool
	groupDetailClose  widget.Clickable
	groupLeaveBtn     widget.Clickable
	groupDeleteBtn     widget.Clickable
	groupMemberList   widget.List
	showAddMember     bool
	addMemberInput    widget.Editor
	addMemberSubmit   widget.Clickable
	addMemberCancel   widget.Clickable
	addMemberError    string
	removeMemberBtns  []widget.Clickable
	groupConfirmLeave bool
	groupConfirmDelete bool
}

// GioAppConfig holds Gio UI application configuration.
type GioAppConfig struct {
	DarkMode      bool
	Title         string
	Width, Height unit.Dp
	DataDir       string
	AppConfig     *config.AppConfig
}

// ── Constructor ─────────────────────────────────────────────────────────────

// NewApp creates a new Gio UI application.
// coreApp may be nil if core services are being initialized asynchronously.
func NewApp(coreApp babylonapp.Application, cfg *GioAppConfig) *App {
	ctx, cancel := context.WithCancel(context.Background())

	window := &app.Window{}

	theme := material.NewTheme()
	theme.Shaper = text.NewShaper(text.WithCollection(gofont.Collection()))

	// Use appearance config if available, fallback to GioAppConfig.DarkMode
	darkMode := cfg.DarkMode
	if cfg.AppConfig != nil {
		darkMode = cfg.AppConfig.Appearance.DarkMode
	}

	uiTheme := NewTheme(LightTheme())
	if darkMode {
		uiTheme = NewTheme(DarkTheme())
	}

	settingsColors := DarkSettingsColors()
	if !darkMode {
		settingsColors = LightSettingsColors()
	}

	uiState := &UIState{
		isDarkMode:     darkMode,
		theme:          uiTheme,
		contactButtons: make([]widget.Clickable, 0),
		settingsScreen: NewSettingsScreen(cfg.AppConfig, settingsColors),
	}

	uiState.messageInput.SingleLine = true
	uiState.messageInput.Submit = true
	uiState.searchInput.SingleLine = true
	uiState.addContactInput.SingleLine = true
	uiState.addContactInput.Submit = true
	uiState.contactDetail.renameInput.SingleLine = true
	uiState.contactDetail.renameInput.Submit = true

	uiState.contactList.Axis = layout.Vertical
	uiState.chatList.Axis = layout.Vertical
	uiState.groupList.Axis = layout.Vertical
	uiState.groupMemberList.Axis = layout.Vertical

	uiState.createGroupName.SingleLine = true
	uiState.createGroupName.Submit = true
	uiState.createGroupDesc.SingleLine = false
	uiState.addMemberInput.SingleLine = true
	uiState.addMemberInput.Submit = true

	a := &App{
		window:       window,
		theme:        theme,
		ui:           uiState,
		ctx:          ctx,
		cancel:       cancel,
		messageCache:    make(map[string][]*babylonapp.ChatMessage),
		unreadCounts:    make(map[string]int),
		keyChangeAlerts: make(map[string]bool),
		uiEvents:       make(chan uiEvent, 256),
		dataDir:        cfg.DataDir,
		appCfg:         cfg.AppConfig,
		sidebarTab:     "chats",
		typingContacts: make(map[string]bool),
	}

	if coreApp != nil {
		a.attachCoreApp(coreApp)
	}

	return a
}

// ── Core Attachment ─────────────────────────────────────────────────────────

// SetCoreApp attaches core services after async initialization.
func (a *App) SetCoreApp(coreApp babylonapp.Application) {
	a.attachCoreApp(coreApp)

	a.status.mu.Lock()
	a.status.CoreReady = true
	a.status.CoreError = ""
	a.status.mu.Unlock()

	a.window.Invalidate()
}

// SetCoreError records a core initialization failure.
func (a *App) SetCoreError(err error) {
	a.status.mu.Lock()
	a.status.CoreError = err.Error()
	a.status.mu.Unlock()
	a.window.Invalidate()
}

// attachCoreApp wires the core application into the UI.
func (a *App) attachCoreApp(coreApp babylonapp.Application) {
	a.coreApp = coreApp
	a.contactMgr = coreApp.Contacts()
	a.chatMgr = coreApp.Chat()
	a.uiGroupMgr = coreApp.UIGroups()

	a.status.mu.Lock()
	a.status.CoreReady = true
	a.status.mu.Unlock()

	a.updateConnectionStatus()
	go a.initialLoad()
	go a.listenForMessages()
	go a.pollConnectionStatus()
}

// initialLoad fetches contacts and offline messages after core starts.
func (a *App) initialLoad() {
	// Signal contacts refresh (processed on main goroutine)
	a.uiEvents <- uiEvent{Type: evtContactsChanged}

	// Check mnemonic reminder status from storage
	if a.coreApp != nil && a.coreApp.Storage() != nil {
		val, err := a.coreApp.Storage().GetConfig("mnemonic_backup_dismissed")
		if err == nil && val == "true" {
			a.mnemonicReminderDismissed = true
		}
	}

	// Retrieve offline messages (results arrive via IncomingMessages channel)
	if a.chatMgr != nil {
		select {
		case a.uiEvents <- uiEvent{Type: evtOfflineStarted}:
		default:
		}
		a.window.Invalidate()

		if _, err := a.chatMgr.RetrieveOfflineMessages(); err != nil {
			logger.Warnw("failed to retrieve offline messages", "error", err)
		}

		select {
		case a.uiEvents <- uiEvent{Type: evtOfflineCompleted}:
		default:
		}
	}

	a.window.Invalidate()
}

// ── Message Listener ────────────────────────────────────────────────────────

func (a *App) listenForMessages() {
	if a.chatMgr == nil {
		logger.Warn("chatMgr is nil, message listener not started")
		return
	}

	ch := a.chatMgr.IncomingMessages()
	if ch == nil {
		logger.Warn("IncomingMessages channel is nil, message listener not started")
		return
	}

	logger.Info("UI message listener started")
	for {
		select {
		case <-a.ctx.Done():
			return
		case msg, ok := <-ch:
			if !ok {
				logger.Warn("IncomingMessages channel closed")
				return
			}
			logger.Debugw("UI received incoming message",
				"from", msg.ContactPubKeyBase58[:min(16, len(msg.ContactPubKeyBase58))],
				"text_len", len(msg.Text))
			// Non-blocking send to prevent backpressure deadlock.
			// If channel is full, message is still in storage and will
			// appear on next periodic refresh.
			select {
			case a.uiEvents <- uiEvent{Type: evtIncomingMessage, Message: msg}:
			default:
				logger.Warnw("uiEvents channel full, message will appear on next refresh")
			}
			a.window.Invalidate()
		}
	}
}

// ── Event Drain (called every frame on main goroutine) ──────────────────────

func (a *App) drainUIEvents() {
	for {
		select {
		case evt := <-a.uiEvents:
			switch evt.Type {
			case evtIncomingMessage:
				a.handleIncomingMessage(evt.Message)
			case evtContactsChanged:
				a.refreshContacts() // launches background goroutine
			case evtDataRefreshed:
				a.applyRefreshedData(evt.Data) // fast: just swaps pointers
			case evtOfflineStarted:
				a.offlineRetrieving = true
			case evtOfflineCompleted:
				a.offlineRetrieving = false
			case evtGroupsRefreshed:
				if evt.Groups != nil {
					a.groupList = evt.Groups
					for len(a.ui.groupButtons) < len(a.groupList) {
						a.ui.groupButtons = append(a.ui.groupButtons, widget.Clickable{})
					}
				}
			}
		default:
			// After draining all events, trigger batched conversation
			// refresh in background if any incoming messages arrived.
			if a.conversationsDirty {
				a.conversationsDirty = false
				a.refreshConversations() // launches background goroutine
			}
			return
		}
	}
}

func (a *App) handleIncomingMessage(msg *babylonapp.IncomingMessage) {
	if msg == nil {
		return
	}
	key := msg.ContactPubKeyBase58

	// Handle non-text event types
	switch msg.EventType {
	case babylonapp.IncomingTyping:
		a.setContactTyping(key, msg.IsTyping)
		return
	case babylonapp.IncomingReadReceipt:
		// Mark messages as read (update status in cache)
		a.updateMessageStatuses(key, msg.MessageIDs, babylonapp.StatusRead)
		return
	case babylonapp.IncomingDeliveryReceipt:
		a.updateMessageStatuses(key, msg.MessageIDs, babylonapp.StatusDelivered)
		return
	case babylonapp.IncomingReaction:
		a.applyReaction(key, msg.TargetMessageID, msg.Emoji, msg.RemoveReaction)
		return
	case babylonapp.IncomingEdit:
		a.applyEdit(key, msg.TargetMessageID, msg.Text)
		return
	case babylonapp.IncomingDelete:
		a.applyDelete(key, msg.TargetMessageID)
		return
	}

	logger.Debugw("handling incoming message in UI",
		"from", key[:min(16, len(key))],
		"text_len", len(msg.Text),
		"is_selected", key == a.selectedKey)

	// Clear typing indicator when a message arrives
	a.setContactTyping(key, false)

	chatMsg := &babylonapp.ChatMessage{
		Text:                msg.Text,
		Timestamp:           msg.Timestamp,
		IsOutgoing:          false,
		SenderName:          msg.ContactName,
		ContactPubKeyBase58: key,
	}

	a.messageCache[key] = append(a.messageCache[key], chatMsg)

	// Unread count (skip if this is the active chat)
	if key != a.selectedKey {
		a.unreadCounts[key]++
	} else {
		a.scrollToBottom = true
	}

	// Mark conversations as dirty — will be refreshed once after all
	// pending events are drained (avoids expensive refresh per message)
	a.conversationsDirty = true
}

// ── Data Refresh ────────────────────────────────────────────────────────────
//
// All heavy data fetching runs in background goroutines. The main goroutine
// only swaps in the results via applyRefreshedData(). This keeps the UI
// responsive even when storage/network queries are slow.

// refreshContacts triggers a background refresh of contacts + conversations.
func (a *App) refreshContacts() {
	if a.contactMgr == nil || a.chatMgr == nil {
		return
	}
	go a.backgroundRefreshContactsAndConversations()
}

// refreshConversations triggers a background refresh of conversations only.
func (a *App) refreshConversations() {
	if a.chatMgr == nil {
		return
	}
	go a.backgroundRefreshConversations()
}

// refreshCurrentChat triggers a background reload of the selected chat.
func (a *App) refreshCurrentChat() {
	key := a.selectedKey
	if key == "" || a.chatMgr == nil {
		return
	}
	go a.backgroundRefreshChat(key)
}

// backgroundRefreshContactsAndConversations fetches contacts + conversations
// off the main goroutine, then sends results back via uiEvents.
func (a *App) backgroundRefreshContactsAndConversations() {
	contacts, err := a.contactMgr.ListContacts()
	if err != nil {
		logger.Warnw("failed to list contacts", "error", err)
		return
	}

	var convs []*babylonapp.Conversation
	convs, err = a.chatMgr.GetConversations()
	if err != nil {
		logger.Debugw("failed to get conversations", "error", err)
	}

	data := &refreshedData{
		contacts:      contacts,
		conversations: convs,
	}

	select {
	case a.uiEvents <- uiEvent{Type: evtDataRefreshed, Data: data}:
	default:
		logger.Debug("skipped data refresh delivery, uiEvents channel full")
	}
	a.window.Invalidate()
}

// backgroundRefreshConversations fetches conversations without expensive
// status queries. Contact online status is updated separately by the
// contact ticker which calls backgroundRefreshContactsAndConversations.
func (a *App) backgroundRefreshConversations() {
	convs, err := a.chatMgr.GetConversationsLight()
	if err != nil {
		logger.Debugw("failed to get conversations", "error", err)
		return
	}

	data := &refreshedData{
		conversations: convs,
	}

	select {
	case a.uiEvents <- uiEvent{Type: evtDataRefreshed, Data: data}:
	default:
	}
	a.window.Invalidate()
}

// backgroundRefreshChat reloads a chat from storage.
func (a *App) backgroundRefreshChat(key string) {
	msgs, err := a.chatMgr.GetMessages(key, 500, 0)
	if err != nil {
		logger.Warnw("failed to load messages", "error", err)
		return
	}

	data := &refreshedData{
		chatMessages: msgs,
		chatKey:      key,
	}

	select {
	case a.uiEvents <- uiEvent{Type: evtDataRefreshed, Data: data}:
	default:
	}
	a.window.Invalidate()
}

// applyRefreshedData swaps in results from a background refresh.
// Called on the main goroutine only.
func (a *App) applyRefreshedData(data *refreshedData) {
	if data == nil {
		return
	}

	if data.contacts != nil {
		// Check for key changes in verified contacts
		a.detectKeyChanges(data.contacts)

		a.contactList = data.contacts
		for len(a.ui.contactButtons) < len(a.contactList) {
			a.ui.contactButtons = append(a.ui.contactButtons, widget.Clickable{})
		}
	}

	if data.conversations != nil {
		a.conversations = data.conversations
	}

	if data.chatMessages != nil && data.chatKey == a.selectedKey {
		a.messageCache[a.selectedKey] = data.chatMessages
		a.scrollToBottom = true
	}
}

// detectKeyChanges checks if any verified contact's fingerprint has changed.
// This detects potential MITM attacks or identity changes.
func (a *App) detectKeyChanges(newContacts []*babylonapp.ContactInfo) {
	if a.coreApp == nil || a.coreApp.Storage() == nil {
		return
	}
	store := a.coreApp.Storage()

	for _, contact := range newContacts {
		// Only track verified contacts
		if !a.isContactVerified(contact.PublicKeyBase58) {
			continue
		}

		// Compute current fingerprint
		fp, err := babylonapp.ContactFingerprint(contact.PublicKeyBase58, contact.X25519KeyBase58)
		if err != nil {
			continue
		}

		configKey := "keyfp:" + contact.PublicKeyBase58
		storedFP, err := store.GetConfig(configKey)
		if err != nil || storedFP == "" {
			// First time seeing this contact — store fingerprint
			_ = store.SetConfig(configKey, fp)
			continue
		}

		if storedFP != fp && !a.keyChangeAlerts[contact.PublicKeyBase58] {
			// Key has changed! Raise alert
			a.keyChangeAlerts[contact.PublicKeyBase58] = false // false = not dismissed
			// Update stored fingerprint
			_ = store.SetConfig(configKey, fp)
		}
	}
}

// hasKeyChangeAlert returns true if the selected contact has an active key change alert.
func (a *App) hasKeyChangeAlert() bool {
	if a.selectedKey == "" {
		return false
	}
	dismissed, exists := a.keyChangeAlerts[a.selectedKey]
	return exists && !dismissed
}

// ── Rich Message Helpers ─────────────────────────────────────────────────

func (a *App) updateMessageStatuses(contactKey string, messageIDs []string, status babylonapp.MessageStatus) {
	msgs := a.messageCache[contactKey]
	idSet := make(map[string]bool, len(messageIDs))
	for _, id := range messageIDs {
		idSet[id] = true
	}
	for _, msg := range msgs {
		if msg.IsOutgoing && msg.MessageID != "" && idSet[msg.MessageID] {
			msg.Status = status
		}
	}
}

func (a *App) applyReaction(contactKey, targetMessageID, emoji string, remove bool) {
	msgs := a.messageCache[contactKey]
	for _, msg := range msgs {
		if msg.MessageID == targetMessageID {
			if msg.Reactions == nil {
				msg.Reactions = make(map[string]int)
			}
			if remove {
				msg.Reactions[emoji]--
				if msg.Reactions[emoji] <= 0 {
					delete(msg.Reactions, emoji)
				}
			} else {
				msg.Reactions[emoji]++
			}
			return
		}
	}
}

func (a *App) applyEdit(contactKey, targetMessageID, newText string) {
	msgs := a.messageCache[contactKey]
	for _, msg := range msgs {
		if msg.MessageID == targetMessageID {
			msg.Text = newText
			msg.IsEdited = true
			now := time.Now()
			msg.EditedAt = &now
			return
		}
	}
}

func (a *App) applyDelete(contactKey, targetMessageID string) {
	msgs := a.messageCache[contactKey]
	for _, msg := range msgs {
		if msg.MessageID == targetMessageID {
			msg.IsDeleted = true
			msg.Text = ""
			return
		}
	}
}

// ── Actions ─────────────────────────────────────────────────────────────────

func (a *App) sendMessage() error {
	if a.selectedKey == "" || a.chatMgr == nil {
		return nil
	}

	text := a.ui.messageInput.Text()
	if text == "" {
		return nil
	}

	result, err := a.chatMgr.SendMessage(a.selectedKey, text)
	if err != nil {
		a.chatError = err.Error()
		logger.Errorw("failed to send message", "error", err)
		return err
	}
	a.chatError = ""

	// Append to local cache for instant feedback
	chatMsg := &babylonapp.ChatMessage{
		Text:                text,
		Timestamp:           result.Timestamp,
		IsOutgoing:          true,
		SenderName:          "You",
		ContactPubKeyBase58: a.selectedKey,
	}
	a.messageCache[a.selectedKey] = append(a.messageCache[a.selectedKey], chatMsg)

	a.ui.messageInput.SetText("")
	a.scrollToBottom = true
	a.refreshConversations()

	return nil
}

func (a *App) selectContact(key string) {
	a.selectedKey = key
	delete(a.unreadCounts, key)
	a.chatError = ""
	// Show any cached messages immediately, then refresh from storage
	// in background for the canonical view.
	a.scrollToBottom = true
	a.refreshCurrentChat() // async background fetch
}

func (a *App) addContact() {
	if a.contactMgr == nil {
		a.ui.addContactError = "Not connected yet"
		return
	}

	input := strings.TrimSpace(a.ui.addContactInput.Text())
	if input == "" {
		a.ui.addContactError = "Enter a contact link or public key"
		return
	}

	var contact *babylonapp.ContactInfo
	var err error

	if strings.HasPrefix(input, "btower://") || strings.HasPrefix(input, "btower:") {
		contact, err = a.contactMgr.AddContactFromLink(input)
	} else {
		contact, err = a.contactMgr.AddContact(input, "", "")
	}

	if err != nil {
		a.ui.addContactError = err.Error()
		return
	}

	a.ui.showAddContact = false
	a.ui.addContactInput.SetText("")
	a.ui.addContactError = ""
	a.refreshContacts()
	a.selectContact(contact.PublicKeyBase58)
}

// ── Selected Contact Helper ─────────────────────────────────────────────────

// selectedContactInfo returns the ContactInfo for the currently selected contact.
func (a *App) selectedContactInfo() *babylonapp.ContactInfo {
	if a.selectedKey == "" {
		return nil
	}
	for _, c := range a.contactList {
		if c.PublicKeyBase58 == a.selectedKey {
			return c
		}
	}
	// Also check conversations (contact may not be in contactList yet)
	for _, conv := range a.conversations {
		if conv.Contact.PublicKeyBase58 == a.selectedKey {
			return conv.Contact
		}
	}
	return nil
}

// selectedContactName returns the display name for the selected contact.
func (a *App) selectedContactName() string {
	info := a.selectedContactInfo()
	if info == nil {
		return ""
	}
	if info.DisplayName != "" {
		return info.DisplayName
	}
	pk := info.PublicKeyBase58
	if len(pk) > 12 {
		return pk[:6] + "..." + pk[len(pk)-6:]
	}
	return pk
}

// ── Status Bar ──────────────────────────────────────────────────────────────

func (a *App) pollConnectionStatus() {
	// Single ticker for both status + contacts refresh.
	// Messages arrive via the event pipeline (no chat refresh ticker needed).
	contactTicker := time.NewTicker(30 * time.Second)
	defer contactTicker.Stop()

	for {
		select {
		case <-a.ctx.Done():
			return
		case <-contactTicker.C:
			a.updateConnectionStatus()
			select {
			case a.uiEvents <- uiEvent{Type: evtContactsChanged}:
			default:
				logger.Debug("skipped contacts refresh, uiEvents channel full")
			}
			a.window.Invalidate()
		}
	}
}

func (a *App) updateConnectionStatus() {
	if a.coreApp == nil {
		return
	}

	network := a.coreApp.Network()
	if network == nil {
		return
	}

	info := network.GetNetworkInfo()
	bootstrap := network.GetBootstrapStatus()
	msgReady := a.chatMgr != nil

	a.status.mu.Lock()
	defer a.status.mu.Unlock()

	a.status.IPFSConnected = info.IsStarted && info.ConnectedPeerCount > 0
	a.status.ConnectedPeers = info.ConnectedPeerCount
	a.status.BabylonConnected = bootstrap.BabylonBootstrapComplete
	a.status.BabylonPeers = bootstrap.BabylonPeersConnected
	a.status.RendezvousActive = bootstrap.RendezvousActive
	a.status.MessagingReady = msgReady
}

// StatusText returns a formatted status string for the status bar.
func (a *App) StatusText() string {
	a.status.mu.RLock()
	defer a.status.mu.RUnlock()

	if a.status.CoreError != "" {
		return fmt.Sprintf("Error: %s", a.status.CoreError)
	}
	if !a.status.CoreReady {
		return "Initializing..."
	}

	parts := make([]string, 0, 3)

	if a.status.IPFSConnected {
		parts = append(parts, fmt.Sprintf("IPFS: %d peers", a.status.ConnectedPeers))
	} else {
		parts = append(parts, "IPFS: offline")
	}

	if a.status.BabylonConnected {
		parts = append(parts, fmt.Sprintf("Babylon: %d peers", a.status.BabylonPeers))
	} else if a.status.ConnectedPeers > 0 {
		parts = append(parts, "Babylon: connecting...")
	} else {
		parts = append(parts, "Babylon: offline")
	}

	if a.status.MessagingReady {
		parts = append(parts, "Messaging: ready")
	}

	if a.offlineRetrieving {
		parts = append(parts, "Retrieving offline messages...")
	}

	return strings.Join(parts, "  |  ")
}

// ── Gio Event Loop ──────────────────────────────────────────────────────────

func (a *App) Run() error {
	winW, winH := 1200, 800
	if a.appCfg != nil && a.appCfg.Appearance.WindowWidth >= 400 {
		winW = a.appCfg.Appearance.WindowWidth
	}
	if a.appCfg != nil && a.appCfg.Appearance.WindowHeight >= 300 {
		winH = a.appCfg.Appearance.WindowHeight
	}
	a.window.Option(app.Size(unit.Dp(winW), unit.Dp(winH)))
	a.window.Option(app.MinSize(unit.Dp(800), unit.Dp(600)))
	a.window.Option(app.Title("Babylon Tower"))
	a.window.Option(app.Decorated(false))

	// Apply initial font size from config
	if a.appCfg != nil && a.appCfg.Appearance.FontSize >= 8 && a.appCfg.Appearance.FontSize <= 32 {
		a.theme.TextSize = unit.Sp(float32(a.appCfg.Appearance.FontSize))
	}

	var ops op.Ops

	for {
		e := a.window.Event()
		if e == nil {
			continue
		}
		switch e := e.(type) {
		case app.DestroyEvent:
			return e.Err

		case app.FrameEvent:
			gtx := layout.Context{
				Constraints: layout.Constraints{Max: e.Size},
				Metric:      e.Metric,
				Now:         e.Now,
				Source:      e.Source,
				Ops:         &ops,
			}
			a.layout(gtx)
			e.Frame(&ops)
		}
	}
}

func (a *App) Stop() {
	a.cancel()
}

func (a *App) ToggleTheme() {
	a.ui.isDarkMode = !a.ui.isDarkMode
	if a.ui.isDarkMode {
		a.ui.theme = NewTheme(DarkTheme())
	} else {
		a.ui.theme = NewTheme(LightTheme())
	}
}

func (a *App) isDarkMode() bool {
	return a.ui.isDarkMode
}

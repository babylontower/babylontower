package cli

import (
	"fmt"
	"sort"
	"strings"

	"babylontower/pkg/reputation"
)

// handleReputation displays reputation information
func (h *CommandHandler) handleReputation(args []string) {
	if len(args) == 0 {
		h.handleReputationSummary()
		return
	}

	switch args[0] {
	case "list":
		h.handleReputationList()
	case "tier":
		h.handleReputationTier(args[1:])
	case "top":
		h.handleReputationTop(args[1:])
	default:
		h.output(FormatErrorString("Usage: /reputation [list|tier|top]"))
	}
}

// handleReputationSummary displays a summary of reputation tracking
func (h *CommandHandler) handleReputationSummary() {
	if h.messaging.ReputationTracker() == nil {
		h.output(FormatErrorString("Reputation tracker not available"))
		return
	}

	tracker := h.messaging.ReputationTracker()
	records := tracker.GetAllRecords()

	var sb strings.Builder
	sb.WriteString("\n=== Reputation Summary ===\n\n")

	if len(records) == 0 {
		sb.WriteString("No reputation records yet.\n")
		sb.WriteString("Reputation is automatically tracked as you interact with peers.\n")
	} else {
		// Count tiers
		tierCounts := make(map[reputation.Tier]int)
		totalScore := 0.0

		for _, record := range records {
			tierCounts[record.GetTier()]++
			totalScore += record.GetCompositeScore()
		}

		avgScore := totalScore / float64(len(records))

		fmt.Fprintf(&sb, "Total peers tracked: %d\n", len(records))
		fmt.Fprintf(&sb, "Average reputation score: %.2f\n", avgScore)
		sb.WriteString("\nTier distribution:\n")
		fmt.Fprintf(&sb, "  Trusted (0.8-1.0):     %d peers\n", tierCounts[reputation.TierTrusted])
		fmt.Fprintf(&sb, "  Reliable (0.6-0.8):    %d peers\n", tierCounts[reputation.TierReliable])
		fmt.Fprintf(&sb, "  Contributor (0.3-0.6): %d peers\n", tierCounts[reputation.TierContributor])
		fmt.Fprintf(&sb, "  Basic (0.0-0.3):       %d peers\n", tierCounts[reputation.TierBasic])
	}

	sb.WriteString("\nUse /reputation list to see all peers\n")
	sb.WriteString("Use /reputation tier <name> to see peers by tier\n")
	sb.WriteString("Use /reputation top [n] to see top N peers\n")

	h.output(sb.String())
}

// handleReputationList lists all peers with their reputation
func (h *CommandHandler) handleReputationList() {
	if h.messaging.ReputationTracker() == nil {
		h.output(FormatErrorString("Reputation tracker not available"))
		return
	}

	tracker := h.messaging.ReputationTracker()
	records := tracker.GetAllRecords()

	if len(records) == 0 {
		h.output(FormatInfo("No reputation records yet."))
		return
	}

	var sb strings.Builder
	sb.WriteString("\n=== Peer Reputation Records ===\n\n")

	// Sort by score descending
	type scoredRecord struct {
		peerID string
		record *reputation.Record
		score  float64
	}

	scored := make([]scoredRecord, 0, len(records))
	for pid, record := range records {
		scored = append(scored, scoredRecord{
			peerID: string(pid),
			record: record,
			score:  record.GetCompositeScore(),
		})
	}

	sort.Slice(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	for i, sr := range scored {
		metrics := sr.record.GetMetrics()
		tier := sr.record.GetTier()
		attestations := sr.record.GetAttestations()

		fmt.Fprintf(&sb, "[%d] Peer: %s\n", i+1, sr.peerID[:16])
		fmt.Fprintf(&sb, "    Score: %.3f (%s)\n", sr.score, tier.String())
		fmt.Fprintf(&sb, "    Relay: %.2f (%d/%d)\n",
			metrics.RelayReliability,
			metrics.RelaySuccessCount,
			metrics.RelayTotalCount)
		fmt.Fprintf(&sb, "    Uptime: %.2f (%dh/168h)\n",
			metrics.UptimeConsistency,
			metrics.HoursOnline7d)
		fmt.Fprintf(&sb, "    Mailbox: %.2f (%d/%d)\n",
			metrics.MailboxReliability,
			metrics.MailboxRetrievedCount,
			metrics.MailboxDepositedCount)
		fmt.Fprintf(&sb, "    DHT: %.2f (%.0fms avg)\n",
			metrics.DHTResponsiveness,
			metrics.AvgResponseMS)
		fmt.Fprintf(&sb, "    Content: %.2f (%d/%d)\n",
			metrics.ContentServing,
			metrics.BlocksServedCount,
			metrics.BlocksRequestedCount)
		if len(attestations) > 0 {
			fmt.Fprintf(&sb, "    Attestations: %d\n", len(attestations))
		}
		sb.WriteString("\n")
	}

	sb.WriteString("==============================\n")
	h.output(sb.String())
}

// handleReputationTier shows peers in a specific tier
func (h *CommandHandler) handleReputationTier(args []string) {
	if h.messaging.ReputationTracker() == nil {
		h.output(FormatErrorString("Reputation tracker not available"))
		return
	}

	if len(args) == 0 {
		h.output(FormatErrorString("Usage: /reputation tier <basic|contributor|reliable|trusted>"))
		return
	}

	var targetTier reputation.Tier
	switch strings.ToLower(args[0]) {
	case "basic":
		targetTier = reputation.TierBasic
	case "contributor":
		targetTier = reputation.TierContributor
	case "reliable":
		targetTier = reputation.TierReliable
	case "trusted":
		targetTier = reputation.TierTrusted
	default:
		h.output(FormatErrorString("Invalid tier. Use: basic, contributor, reliable, or trusted"))
		return
	}

	tracker := h.messaging.ReputationTracker()
	peers := tracker.GetPeersByTier(targetTier)

	var sb strings.Builder
	fmt.Fprintf(&sb, "\n=== %s Tier Peers ===\n\n", targetTier.String())

	if len(peers) == 0 {
		fmt.Fprintf(&sb, "No peers in %s tier.\n", targetTier.String())
	} else {
		for i, pid := range peers {
			record := tracker.GetRecord(pid)
			if record != nil {
				fmt.Fprintf(&sb, "[%d] %s - Score: %.3f\n",
					i+1, string(pid)[:16], record.GetCompositeScore())
			}
		}
	}

	sb.WriteString("\n")
	h.output(sb.String())
}

// handleReputationTop shows the top N peers by reputation
func (h *CommandHandler) handleReputationTop(args []string) {
	if h.messaging.ReputationTracker() == nil {
		h.output(FormatErrorString("Reputation tracker not available"))
		return
	}

	n := 10 // Default
	if len(args) > 0 {
		if _, err := fmt.Sscanf(args[0], "%d", &n); err != nil {
			h.output(FormatErrorString("Invalid number. Usage: /reputation top [n]"))
			return
		}
		if n <= 0 {
			n = 10
		}
	}

	tracker := h.messaging.ReputationTracker()
	topPeers := tracker.GetTopPeers(n)

	var sb strings.Builder
	fmt.Fprintf(&sb, "\n=== Top %d Peers by Reputation ===\n\n", len(topPeers))

	if len(topPeers) == 0 {
		sb.WriteString("No reputation records yet.\n")
	} else {
		for i, pid := range topPeers {
			record := tracker.GetRecord(pid)
			if record != nil {
				fmt.Fprintf(&sb, "[%d] %s - Score: %.3f (%s)\n",
					i+1, string(pid)[:16], record.GetCompositeScore(), record.GetTier().String())
			}
		}
	}

	sb.WriteString("\n")
	h.output(sb.String())
}

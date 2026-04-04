package backlog

import "context"

// ThresholdResult describes whether the backlog has enough items to trigger implementation.
type ThresholdResult struct {
	ShouldImplement bool
	SelectedItems   []*Item
	Reason          string
}

// EvaluateThreshold checks if pending items meet the configured thresholds for implementation,
// scoped to a specific repo URL.
func EvaluateThreshold(ctx context.Context, store Store, repoURL string, highThresh, mediumThresh, lowThresh, maxPerCycle int) (*ThresholdResult, error) {
	pendingStatus := StatusPending
	items, err := store.List(ctx, ListFilter{Status: &pendingStatus, RepoURL: &repoURL})
	if err != nil {
		return nil, err
	}

	var highItems, mediumItems, lowItems []*Item
	for _, item := range items {
		switch item.Priority {
		case PriorityHigh:
			highItems = append(highItems, item)
		case PriorityMedium:
			mediumItems = append(mediumItems, item)
		case PriorityLow:
			lowItems = append(lowItems, item)
		}
	}

	var selected []*Item
	reason := ""

	// High priority items are always implemented immediately
	if len(highItems) >= highThresh {
		selected = append(selected, highItems...)
		reason = "high-priority items found"
	}

	// Medium priority batch
	if len(mediumItems) >= mediumThresh {
		selected = append(selected, mediumItems...)
		if reason != "" {
			reason += "; "
		}
		reason += "medium-priority threshold met"
	}

	// Low priority batch
	if len(lowItems) >= lowThresh {
		selected = append(selected, lowItems...)
		if reason != "" {
			reason += "; "
		}
		reason += "low-priority threshold met"
	}

	// Cap at max per cycle
	if len(selected) > maxPerCycle {
		selected = selected[:maxPerCycle]
	}

	if len(selected) == 0 {
		return &ThresholdResult{
			ShouldImplement: false,
			Reason:          "no thresholds met",
		}, nil
	}

	return &ThresholdResult{
		ShouldImplement: true,
		SelectedItems:   selected,
		Reason:          reason,
	}, nil
}

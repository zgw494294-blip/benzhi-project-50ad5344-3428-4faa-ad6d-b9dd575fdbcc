package review

import "time"

// Status describes the lifecycle state of a review.
type Status string

const (
	StatusOpen     Status = "open"
	StatusApproved Status = "approved"
	StatusBlocked  Status = "blocked"
)

// Verdict is the result recorded for one required compliance check.
// A non-nil Note distinguishes a supplied empty note from an omitted note.
type Verdict struct {
	Passed bool    `json:"passed"`
	Note   *string `json:"note,omitempty"`
}

// Check is one required compliance check and its optional verdict.
type Check struct {
	Name    string   `json:"name"`
	Verdict *Verdict `json:"verdict,omitempty"`
}

// Review is the current record for an artwork review. Completed reviews are
// returned with an approved or blocked status and cannot be changed.
type Review struct {
	ID          string     `json:"id"`
	SKU         string     `json:"sku"`
	Checks      []Check    `json:"checks"`
	Status      Status     `json:"status"`
	CreatedAt   time.Time  `json:"created_at"`
	CompletedAt *time.Time `json:"completed_at,omitempty"`
}

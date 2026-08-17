package review

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"
)

var (
	ErrInvalidInput  = errors.New("invalid input")
	ErrNotFound      = errors.New("review not found")
	ErrCompleted     = errors.New("review is already completed")
	ErrVerdictExists = errors.New("check already has a verdict")
	ErrIncomplete    = errors.New("review has unrecorded checks")
)

type storedReview struct {
	id          string
	sku         string
	checks      []storedCheck
	status      Status
	createdAt   time.Time
	completedAt *time.Time
}

type storedCheck struct {
	name    string
	verdict *Verdict
}

// Store keeps reviews in process-local memory.
type Store struct {
	mu      sync.RWMutex
	nextID  uint64
	reviews map[string]*storedReview
}

// NewStore creates an empty review store.
func NewStore() *Store {
	return &Store{reviews: make(map[string]*storedReview)}
}

// Create adds a new open review with the supplied required checks.
func (s *Store) Create(ctx context.Context, sku string, checks []string) (Review, error) {
	if err := contextError(ctx); err != nil {
		return Review{}, err
	}
	if strings.TrimSpace(sku) == "" {
		return Review{}, fmt.Errorf("%w: SKU is required", ErrInvalidInput)
	}
	if len(checks) == 0 {
		return Review{}, fmt.Errorf("%w: at least one check is required", ErrInvalidInput)
	}

	copyChecks := make([]storedCheck, len(checks))
	seen := make(map[string]struct{}, len(checks))
	for i, name := range checks {
		trimmedName := strings.TrimSpace(name)
		if trimmedName == "" {
			return Review{}, fmt.Errorf("%w: checks cannot be blank", ErrInvalidInput)
		}
		if _, exists := seen[trimmedName]; exists {
			return Review{}, fmt.Errorf("%w: duplicate check %q", ErrInvalidInput, name)
		}
		seen[trimmedName] = struct{}{}
		copyChecks[i] = storedCheck{name: name}
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return Review{}, err
	}
	s.nextID++
	id := fmt.Sprintf("review-%06d", s.nextID)
	record := &storedReview{
		id:        id,
		sku:       sku,
		checks:    copyChecks,
		status:    StatusOpen,
		createdAt: time.Now().UTC(),
	}
	s.reviews[id] = record
	return cloneReview(record), nil
}

// RecordVerdict records the only verdict allowed for a required check.
func (s *Store) RecordVerdict(ctx context.Context, id, checkName string, passed bool, note *string) (Review, error) {
	if err := contextError(ctx); err != nil {
		return Review{}, err
	}
	if strings.TrimSpace(id) == "" || strings.TrimSpace(checkName) == "" {
		return Review{}, fmt.Errorf("%w: review ID and check name are required", ErrInvalidInput)
	}

	copyNote := cloneString(note)
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return Review{}, err
	}
	record, ok := s.reviews[id]
	if !ok {
		return Review{}, ErrNotFound
	}
	if record.status != StatusOpen {
		return Review{}, ErrCompleted
	}
	for i := range record.checks {
		if record.checks[i].name != checkName {
			continue
		}
		if record.checks[i].verdict != nil {
			return Review{}, ErrVerdictExists
		}
		record.checks[i].verdict = &Verdict{Passed: passed, Note: copyNote}
		return cloneReview(record), nil
	}
	return Review{}, fmt.Errorf("%w: check %q does not belong to review", ErrInvalidInput, checkName)
}

// Finalize completes a review exactly once after every check has a verdict.
func (s *Store) Finalize(ctx context.Context, id string) (Review, error) {
	if err := contextError(ctx); err != nil {
		return Review{}, err
	}
	if strings.TrimSpace(id) == "" {
		return Review{}, fmt.Errorf("%w: review ID is required", ErrInvalidInput)
	}

	s.mu.Lock()
	defer s.mu.Unlock()
	if err := contextError(ctx); err != nil {
		return Review{}, err
	}
	record, ok := s.reviews[id]
	if !ok {
		return Review{}, ErrNotFound
	}
	if record.status != StatusOpen {
		return Review{}, ErrCompleted
	}
	allPassed := true
	for _, check := range record.checks {
		if check.verdict == nil {
			return Review{}, ErrIncomplete
		}
		if !check.verdict.Passed {
			allPassed = false
		}
	}
	completedAt := time.Now().UTC()
	if allPassed {
		record.completedAt = &completedAt
		record.status = StatusApproved
	} else {
		record.status = StatusBlocked
	}
	return cloneReview(record), nil
}

// Get retrieves a defensive copy of the current review record.
func (s *Store) Get(id string) (Review, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	record, ok := s.reviews[id]
	if !ok {
		return Review{}, ErrNotFound
	}
	return cloneReview(record), nil
}

func contextError(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	default:
		return nil
	}
}

func cloneReview(record *storedReview) Review {
	checks := make([]Check, len(record.checks))
	for i, check := range record.checks {
		checks[i] = Check{Name: check.name, Verdict: cloneVerdict(check.verdict)}
	}
	return Review{
		ID:          record.id,
		SKU:         record.sku,
		Checks:      checks,
		Status:      record.status,
		CreatedAt:   record.createdAt,
		CompletedAt: cloneTime(record.completedAt),
	}
}

func cloneVerdict(verdict *Verdict) *Verdict {
	if verdict == nil {
		return nil
	}
	return &Verdict{Passed: verdict.Passed, Note: cloneString(verdict.Note)}
}

func cloneString(value *string) *string {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

func cloneTime(value *time.Time) *time.Time {
	if value == nil {
		return nil
	}
	copyValue := *value
	return &copyValue
}

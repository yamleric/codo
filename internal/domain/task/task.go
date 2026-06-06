package task

import (
	"sync"
	"time"
)

type SourceType string

const (
	SourceManual    SourceType = "manual"
	SourceRSS       SourceType = "rss"
	SourceWechatMP  SourceType = "wechat_mp"
	SourceLinuxDo   SourceType = "linux_do"
	SourceChaoxing  SourceType = "chaoxing"
	SourceEmail     SourceType = "email"
	SourceGroupChat SourceType = "group_chat"
)

type ContentType string

const (
	ContentWebPage ContentType = "webpage"
	ContentVideo   ContentType = "video"
	ContentEmail   ContentType = "email"
	ContentPost    ContentType = "post"
	ContentMessage ContentType = "message"
)

type FilterDecision string

const (
	FilterUnset   FilterDecision = "" // explicit zero value — unset is detectable
	FilterPass    FilterDecision = "pass"
	FilterSilent  FilterDecision = "silent"
	FilterDiscard FilterDecision = "discard"
)

type Status string

const (
	StatusPending   Status = "pending"
	StatusFetching  Status = "fetching"
	StatusFiltering Status = "filtering"
	StatusAnalyzing Status = "analyzing"
	StatusSaving    Status = "saving"
	StatusNotifying Status = "notifying"
	StatusDone      Status = "done"
	StatusDiscarded Status = "discarded"
	StatusFailed    Status = "failed"
)

type StepStatus string

const (
	StepOK      StepStatus = "ok"
	StepError   StepStatus = "error"
	StepSkipped StepStatus = "skipped"
)

// Step records one stage execution.
type Step struct {
	Label    string
	Status   StepStatus
	Detail   string
	Duration time.Duration
	At       time.Time
}

// Task is the aggregate root. Always use *Task — copying invalidates the mutex.
// Immutable fields (ID, Source, ContentType, URL, UserID) are safe to read
// without the lock after construction.
type Task struct {
	ID          string
	Source      SourceType
	ContentType ContentType
	URL         string
	UserID      string

	mu             sync.RWMutex
	rawContent     string
	status         Status
	filterDecision FilterDecision
	summary        string
	errMsg         string
	steps          []Step
	createdAt      time.Time
	updatedAt      time.Time
}

func New(id, userID string, src SourceType, ct ContentType, url, raw string) *Task {
	now := time.Now()
	return &Task{
		ID:             id,
		Source:         src,
		ContentType:    ct,
		URL:            url,
		UserID:         userID,
		rawContent:     raw,
		status:         StatusPending,
		filterDecision: FilterUnset, // explicit default — pipeline must set this
		createdAt:      now,
		updatedAt:      now,
	}
}

func (t *Task) RawContent() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.rawContent
}

func (t *Task) SetRawContent(content string) {
	t.mu.Lock()
	t.rawContent = content
	t.updatedAt = time.Now()
	t.mu.Unlock()
}

func (t *Task) SetStatus(s Status) {
	t.mu.Lock()
	t.status = s
	t.updatedAt = time.Now()
	t.mu.Unlock()
}

func (t *Task) SetError(msg string) {
	t.mu.Lock()
	t.errMsg = msg
	t.mu.Unlock()
}

func (t *Task) Status() Status {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.status
}

func (t *Task) FilterDecision() FilterDecision {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.filterDecision
}

func (t *Task) SetFilterDecision(d FilterDecision) {
	t.mu.Lock()
	t.filterDecision = d
	t.mu.Unlock()
}

func (t *Task) Summary() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.summary
}

func (t *Task) SetSummary(s string) {
	t.mu.Lock()
	t.summary = s
	t.mu.Unlock()
}

func (t *Task) Error() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.errMsg
}

func (t *Task) AddStep(s Step) {
	t.mu.Lock()
	t.steps = append(t.steps, s)
	t.mu.Unlock()
}

func (t *Task) Steps() []Step {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]Step, len(t.steps))
	copy(out, t.steps)
	return out
}

func (t *Task) CreatedAt() time.Time { return t.createdAt }
func (t *Task) UpdatedAt() time.Time {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.updatedAt
}

// ── Snapshot ──────────────────────────────────────────────────────────────────

type Snapshot struct {
	ID             string         `json:"id"`
	Source         SourceType     `json:"source"`
	ContentType    ContentType    `json:"content_type"`
	URL            string         `json:"url"`
	UserID         string         `json:"user_id"`
	Status         Status         `json:"status"`
	FilterDecision FilterDecision `json:"filter_decision"`
	Summary        string         `json:"summary"`
	Error          string         `json:"error"`
	Steps          []SnapshotStep `json:"steps"`
	CreatedAt      time.Time      `json:"created_at"`
	UpdatedAt      time.Time      `json:"updated_at"`
}

type SnapshotStep struct {
	Label      string     `json:"label"`
	Status     StepStatus `json:"status"`
	Detail     string     `json:"detail"`
	DurationMs int64      `json:"duration_ms"`
}

func (t *Task) Snapshot() Snapshot {
	t.mu.RLock()
	defer t.mu.RUnlock()
	steps := make([]SnapshotStep, len(t.steps))
	for i, step := range t.steps {
		steps[i] = SnapshotStep{
			Label:      step.Label,
			Status:     step.Status,
			Detail:     step.Detail,
			DurationMs: step.Duration.Milliseconds(),
		}
	}
	return Snapshot{
		ID:             t.ID,
		Source:         t.Source,
		ContentType:    t.ContentType,
		URL:            t.URL,
		UserID:         t.UserID,
		Status:         t.status,
		FilterDecision: t.filterDecision,
		Summary:        t.summary,
		Error:          t.errMsg,
		Steps:          steps,
		CreatedAt:      t.createdAt,
		UpdatedAt:      t.updatedAt,
	}
}

package task

import (
	"strings"
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

type Classification struct {
	Category string   `json:"category"`
	Tags     []string `json:"tags"`
	Reason   string   `json:"reason,omitempty"`
}

var allowedCategories = map[string]struct{}{
	"AI": {},
	"技术": {},
	"产品": {},
	"商业": {},
	"社会": {},
	"学习": {},
	"生活": {},
	"娱乐": {},
	"工具": {},
	"其他": {},
}

func NormalizeClassification(c Classification) Classification {
	c.Category = strings.TrimSpace(c.Category)
	if _, ok := allowedCategories[c.Category]; !ok {
		c.Category = "其他"
	}
	c.Tags = normalizeTags(c.Tags)
	c.Reason = truncateRunes(strings.TrimSpace(c.Reason), 80)
	return c
}

func normalizeTags(tags []string) []string {
	out := make([]string, 0, len(tags))
	seen := make(map[string]struct{}, len(tags))
	for _, tag := range tags {
		tag = truncateRunes(strings.TrimSpace(tag), 16)
		if tag == "" {
			continue
		}
		key := strings.ToLower(tag)
		if _, exists := seen[key]; exists {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, tag)
		if len(out) >= 5 {
			break
		}
	}
	if out == nil {
		return []string{}
	}
	return out
}

func truncateRunes(value string, maxRunes int) string {
	runes := []rune(value)
	if len(runes) <= maxRunes {
		return value
	}
	return string(runes[:maxRunes])
}

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
	category       string
	tags           []string
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

func (t *Task) Category() string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return t.category
}

func (t *Task) Tags() []string {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]string, len(t.tags))
	copy(out, t.tags)
	return out
}

func (t *Task) SetClassification(category string, tags []string) {
	classification := NormalizeClassification(Classification{Category: category, Tags: tags})
	t.mu.Lock()
	t.category = classification.Category
	t.tags = append([]string(nil), classification.Tags...)
	t.updatedAt = time.Now()
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
	Category       string         `json:"category"`
	Tags           []string       `json:"tags"`
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
	tags := make([]string, len(t.tags))
	copy(tags, t.tags)
	return Snapshot{
		ID:             t.ID,
		Source:         t.Source,
		ContentType:    t.ContentType,
		URL:            t.URL,
		UserID:         t.UserID,
		Status:         t.status,
		FilterDecision: t.filterDecision,
		Category:       t.category,
		Tags:           tags,
		Summary:        t.summary,
		Error:          t.errMsg,
		Steps:          steps,
		CreatedAt:      t.createdAt,
		UpdatedAt:      t.updatedAt,
	}
}

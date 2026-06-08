package pipeline

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/codo/codo/internal/domain/task"
)

// ── Fine-grained LLM interfaces ───────────────────────────────────────────────

type Filterer interface {
	Filter(ctx context.Context, userID, content string) (task.FilterDecision, string, error)
}

type Summarizer interface {
	Summarize(ctx context.Context, t *task.Task, content string) (string, error)
}

type Translator interface {
	Translate(ctx context.Context, userID, content string) (task.Translation, error)
}

type Classifier interface {
	Classify(ctx context.Context, content string) (string, error)
}

type Categorizer interface {
	Categorize(ctx context.Context, userID, content string) (task.Classification, error)
}

type Extractor interface {
	Extract(ctx context.Context, messages string) (string, error)
}

// ── Infrastructure interfaces ─────────────────────────────────────────────────

type Fetcher interface {
	Fetch(ctx context.Context, t *task.Task) (string, error)
}

type Store interface {
	// SaveTaskState persists task status and metadata (called on every status change).
	SaveTaskState(ctx context.Context, t *task.Task) error
	// SaveKnowledgeItem persists the final summary into the knowledge base.
	SaveKnowledgeItem(ctx context.Context, t *task.Task) error
	// AppendStep persists a single step immediately after execution.
	AppendStep(ctx context.Context, taskID string, s task.Step) error
	// IsDuplicate returns true if the URL already exists in the knowledge base.
	IsDuplicate(ctx context.Context, url string) (bool, error)
}

type Notifier interface {
	Send(ctx context.Context, userID, message string) error
}

type UserSettings struct {
	NotifyChannel string
	NotifyPolicy  string
}

type UserSettingsProvider interface {
	GetPipelineSettings(ctx context.Context, userID string) (UserSettings, error)
}

// OnStatusFunc receives an immutable snapshot — safe for async WebSocket/logging.
type OnStatusFunc func(snap task.Snapshot)

// ── Pipeline interface ────────────────────────────────────────────────────────

type Pipeline interface {
	ContentType() task.ContentType
	Run(ctx context.Context, t *task.Task) error
}

// ── Router ────────────────────────────────────────────────────────────────────

type Router struct {
	pipelines map[task.ContentType]Pipeline
}

// NewRouter returns an error if duplicate content types are registered.
func NewRouter(pp ...Pipeline) (*Router, error) {
	r := &Router{pipelines: make(map[task.ContentType]Pipeline, len(pp))}
	for _, p := range pp {
		if _, exists := r.pipelines[p.ContentType()]; exists {
			return nil, fmt.Errorf("duplicate pipeline for content type: %s", p.ContentType())
		}
		r.pipelines[p.ContentType()] = p
	}
	return r, nil
}

func (r *Router) Run(ctx context.Context, t *task.Task) error {
	p, ok := r.pipelines[t.ContentType]
	if !ok {
		return fmt.Errorf("no pipeline for content type: %s", t.ContentType)
	}
	return p.Run(ctx, t)
}

// ── Shared stage helpers ──────────────────────────────────────────────────────

type stages struct {
	store       Store
	notifier    Notifier
	onStatus    OnStatusFunc
	settings    UserSettingsProvider
	categorizer Categorizer
}

func newStages(st Store, n Notifier, on OnStatusFunc, c Categorizer) stages {
	settings, _ := st.(UserSettingsProvider)
	return stages{store: st, notifier: n, onStatus: on, settings: settings, categorizer: c}
}

// setStatus persists state and notifies the board.
// Returns error on store failure so callers can decide to abort.
func (s *stages) setStatus(ctx context.Context, t *task.Task, st task.Status) error {
	t.SetStatus(st)
	if err := s.store.SaveTaskState(ctx, t); err != nil {
		return fmt.Errorf("persist status %s: %w", st, err)
	}
	if s.onStatus != nil {
		s.onStatus(t.Snapshot())
	}
	return nil
}

func (s *stages) addStep(ctx context.Context, t *task.Task, label string, st task.StepStatus, detail string, start time.Time) {
	step := task.Step{Label: label, Status: st, Detail: detail, Duration: time.Since(start), At: start}
	t.AddStep(step)
	_ = s.store.AppendStep(ctx, t.ID, step) // best-effort: board still shows in-memory steps
}

// fail records the step with its real duration, sets the error, and transitions to failed.
func (s *stages) fail(ctx context.Context, t *task.Task, label string, err error, start time.Time) error {
	s.addStep(ctx, t, label, task.StepError, err.Error(), start)
	t.SetError(err.Error())
	_ = s.setStatus(ctx, t, task.StatusFailed) // best-effort after failure
	return fmt.Errorf("%s: %w", label, err)
}

func (s *stages) dedup(ctx context.Context, t *task.Task) (bool, error) {
	if t.URL == "" {
		return false, nil
	}
	start := time.Now()
	dup, err := s.store.IsDuplicate(ctx, t.URL)
	if err != nil {
		return false, err
	}
	if dup {
		s.addStep(ctx, t, "去重检查", task.StepSkipped, "URL 已存在知识库", start)
		_ = s.setStatus(ctx, t, task.StatusDiscarded)
		return true, nil
	}
	s.addStep(ctx, t, "去重检查", task.StepOK, "", start)
	return false, nil
}

func (s *stages) categorize(ctx context.Context, t *task.Task, content string) {
	if s.categorizer == nil {
		return
	}
	start := time.Now()
	classification, err := s.categorizer.Categorize(ctx, t.UserID, content)
	if err != nil {
		s.addStep(ctx, t, "内容分类", task.StepError, err.Error(), start)
		return
	}
	classification = task.NormalizeClassification(classification)
	t.SetClassification(classification.Category, classification.Tags)
	if err := s.store.SaveTaskState(ctx, t); err != nil {
		s.addStep(ctx, t, "内容分类", task.StepError, err.Error(), start)
		return
	}
	detail := classification.Category
	if len(classification.Tags) > 0 {
		detail += ": " + strings.Join(classification.Tags, " / ")
	}
	s.addStep(ctx, t, "内容分类", task.StepOK, detail, start)
	if s.onStatus != nil {
		s.onStatus(t.Snapshot())
	}
}

func (s *stages) saveAndNotify(ctx context.Context, t *task.Task, summary string) error {
	if err := s.setStatus(ctx, t, task.StatusSaving); err != nil {
		return s.fail(ctx, t, "更新状态", err, time.Now())
	}
	start := time.Now()
	if err := s.store.SaveKnowledgeItem(ctx, t); err != nil {
		return s.fail(ctx, t, "存入知识库", err, start)
	}
	s.addStep(ctx, t, "存入知识库", task.StepOK, "", start)

	if t.FilterDecision() == task.FilterPass {
		notify, skipReason := s.shouldNotify(ctx, t.UserID)
		if !notify {
			s.addStep(ctx, t, "推送通知", task.StepSkipped, skipReason, time.Now())
			_ = s.setStatus(ctx, t, task.StatusDone)
			return nil
		}
		if err := s.setStatus(ctx, t, task.StatusNotifying); err != nil {
			// non-fatal: content already saved; record and continue to Done
			s.addStep(ctx, t, "更新通知状态", task.StepError, err.Error(), time.Now())
		} else {
			start = time.Now()
			if err := s.notifier.Send(ctx, t.UserID, summary); err != nil {
				s.addStep(ctx, t, "推送通知", task.StepError, err.Error(), start)
			} else {
				s.addStep(ctx, t, "推送通知", task.StepOK, "", start)
			}
		}
	}

	// StatusDone failure is non-fatal — content is saved and notification sent.
	_ = s.setStatus(ctx, t, task.StatusDone)
	return nil
}

func (s *stages) shouldNotify(ctx context.Context, userID string) (bool, string) {
	settings := UserSettings{NotifyChannel: "telegram", NotifyPolicy: "pass_only"}
	if s.settings != nil {
		userSettings, err := s.settings.GetPipelineSettings(ctx, userID)
		if err == nil {
			settings = normalizeUserSettings(userSettings)
		}
	}
	if settings.NotifyChannel == "none" {
		return false, "通知通道已关闭"
	}
	if settings.NotifyPolicy == "save_only" {
		return false, "用户设置为静默归档"
	}
	return true, ""
}

func normalizeUserSettings(settings UserSettings) UserSettings {
	if settings.NotifyChannel != "telegram" && settings.NotifyChannel != "email" && settings.NotifyChannel != "none" {
		settings.NotifyChannel = "telegram"
	}
	if settings.NotifyPolicy != "pass_only" && settings.NotifyPolicy != "save_only" {
		settings.NotifyPolicy = "pass_only"
	}
	return settings
}

func (s *stages) useExplicitSourceDecision(ctx context.Context, t *task.Task) bool {
	decision, reason, ok := explicitSourceDecision(t.Source)
	if !ok {
		return false
	}
	t.SetFilterDecision(decision)
	s.addStep(ctx, t, "过滤判断", task.StepSkipped, fmt.Sprintf("%s: %s", decision, reason), time.Now())
	return true
}

func explicitSourceDecision(source task.SourceType) (task.FilterDecision, string, bool) {
	switch source {
	case task.SourceManual:
		return task.FilterPass, "用户主动提交链接，直接保存并推送摘要", true
	case task.SourceBookmark, task.SourceLinuxDo:
		return task.FilterSilent, "用户收藏来源，跳过丢弃过滤并静默入库", true
	default:
		return task.FilterUnset, "", false
	}
}

// ── WebPagePipeline ───────────────────────────────────────────────────────────

type WebPagePipeline struct {
	stages
	fetcher    Fetcher
	filterer   Filterer
	summarizer Summarizer
	translator Translator
}

func NewWebPage(f Fetcher, fl Filterer, su Summarizer, st Store, n Notifier, on OnStatusFunc) *WebPagePipeline {
	categorizer, _ := su.(Categorizer)
	translator, _ := su.(Translator)
	return &WebPagePipeline{stages: newStages(st, n, on, categorizer), fetcher: f, filterer: fl, summarizer: su, translator: translator}
}

func (p *WebPagePipeline) ContentType() task.ContentType { return task.ContentWebPage }

func (p *WebPagePipeline) Run(ctx context.Context, t *task.Task) error {
	if err := p.setStatus(ctx, t, task.StatusFetching); err != nil {
		return err
	}

	if dup, err := p.dedup(ctx, t); err != nil {
		return p.fail(ctx, t, "去重检查", err, time.Now())
	} else if dup {
		return nil
	}

	start := time.Now()
	content, err := p.fetcher.Fetch(ctx, t)
	if err != nil {
		return p.fail(ctx, t, "抓取网页", err, start)
	}
	t.SetRawContent(content)
	p.addStep(ctx, t, "抓取网页", task.StepOK, fmt.Sprintf("%d 字", len([]rune(content))), start)

	if err := p.setStatus(ctx, t, task.StatusFiltering); err != nil {
		return err
	}
	if !p.useExplicitSourceDecision(ctx, t) {
		start = time.Now()
		decision, reason, err := p.filterer.Filter(ctx, t.UserID, content)
		if err != nil {
			return p.fail(ctx, t, "过滤判断", err, start)
		}
		t.SetFilterDecision(decision)
		p.addStep(ctx, t, "过滤判断", task.StepOK, fmt.Sprintf("%s: %s", decision, reason), start)
		if decision == task.FilterDiscard {
			return p.setStatus(ctx, t, task.StatusDiscarded)
		}
	}

	if err := p.setStatus(ctx, t, task.StatusAnalyzing); err != nil {
		return err
	}
	analysisContent := content
	if translation, ok := p.translate(ctx, t, content); ok && useTranslationForSummary(translation) {
		analysisContent = translation.Content
	}
	start = time.Now()
	summary, err := p.summarizer.Summarize(ctx, t, analysisContent)
	if err != nil {
		return p.fail(ctx, t, "生成摘要", err, start)
	}
	t.SetSummary(summary)
	p.addStep(ctx, t, "生成摘要", task.StepOK, "", start)
	p.categorize(ctx, t, analysisContent+"\n\n摘要：\n"+summary)

	return p.saveAndNotify(ctx, t, summary)
}

func (p *WebPagePipeline) translate(ctx context.Context, t *task.Task, content string) (task.Translation, bool) {
	if p.translator == nil {
		return task.Translation{}, false
	}
	start := time.Now()
	translation, err := p.translator.Translate(ctx, t.UserID, content)
	if err != nil {
		p.addStep(ctx, t, "自动翻译", task.StepError, err.Error(), start)
		return task.Translation{}, false
	}
	switch translation.Status {
	case "translated":
		t.SetMetadataValue("translation", translation)
		p.addStep(ctx, t, "自动翻译", task.StepOK, translationStepDetail(translation), start)
		return translation, true
	case "skipped":
		detail := translation.Reason
		if detail == "not english" {
			detail = "非英文内容"
		}
		p.addStep(ctx, t, "自动翻译", task.StepSkipped, detail, start)
	case "disabled", "":
	default:
		p.addStep(ctx, t, "自动翻译", task.StepSkipped, translation.Reason, start)
	}
	return translation, false
}

func useTranslationForSummary(translation task.Translation) bool {
	if strings.TrimSpace(translation.Content) == "" {
		return false
	}
	return translation.Scope == "summary" || translation.Scope == "summary_knowledge"
}

func translationStepDetail(translation task.Translation) string {
	target := translation.TargetLang
	if target == "" {
		target = "zh-CN"
	}
	detail := fmt.Sprintf("英文 -> %s，%d 字", target, translation.TranslatedChars)
	if translation.Truncated {
		detail += "，已按上限截断"
	}
	return detail
}

// ── VideoPipeline ─────────────────────────────────────────────────────────────

type VideoPipeline struct {
	stages
	fetcher    Fetcher
	filterer   Filterer
	summarizer Summarizer
}

func NewVideo(f Fetcher, fl Filterer, su Summarizer, st Store, n Notifier, on OnStatusFunc) *VideoPipeline {
	categorizer, _ := su.(Categorizer)
	return &VideoPipeline{stages: newStages(st, n, on, categorizer), fetcher: f, filterer: fl, summarizer: su}
}

func (p *VideoPipeline) ContentType() task.ContentType { return task.ContentVideo }

func (p *VideoPipeline) Run(ctx context.Context, t *task.Task) error {
	if err := p.setStatus(ctx, t, task.StatusFetching); err != nil {
		return err
	}

	if dup, err := p.dedup(ctx, t); err != nil {
		return p.fail(ctx, t, "去重检查", err, time.Now())
	} else if dup {
		return nil
	}

	start := time.Now()
	transcript, err := p.fetcher.Fetch(ctx, t)
	if err != nil {
		return p.fail(ctx, t, "获取视频文字稿", err, start)
	}
	t.SetRawContent(transcript)
	p.addStep(ctx, t, "获取视频文字稿", task.StepOK, fmt.Sprintf("%d 字", len([]rune(transcript))), start)

	if err := p.setStatus(ctx, t, task.StatusFiltering); err != nil {
		return err
	}
	if !p.useExplicitSourceDecision(ctx, t) {
		start2 := time.Now()
		decision, reason, ferr := p.filterer.Filter(ctx, t.UserID, transcript)
		if ferr != nil {
			return p.fail(ctx, t, "过滤判断", ferr, start2)
		}
		t.SetFilterDecision(decision)
		p.addStep(ctx, t, "过滤判断", task.StepOK, fmt.Sprintf("%s: %s", decision, reason), start2)
		if decision == task.FilterDiscard {
			return p.setStatus(ctx, t, task.StatusDiscarded)
		}
	}
	if err := p.setStatus(ctx, t, task.StatusAnalyzing); err != nil {
		return err
	}
	start = time.Now()
	summary, err := p.summarizer.Summarize(ctx, t, transcript)
	if err != nil {
		return p.fail(ctx, t, "生成摘要", err, start)
	}
	t.SetSummary(summary)
	p.addStep(ctx, t, "生成摘要", task.StepOK, "", start)
	p.categorize(ctx, t, transcript+"\n\n摘要：\n"+summary)

	return p.saveAndNotify(ctx, t, summary)
}

// ── EmailPipeline ─────────────────────────────────────────────────────────────

type emailCategory string

const (
	emailImportant emailCategory = "important"
	emailNotify    emailCategory = "notify"
	emailSpam      emailCategory = "spam"
)

type EmailPipeline struct {
	stages
	classifier Classifier
	summarizer Summarizer
}

func NewEmail(cl Classifier, su Summarizer, st Store, n Notifier, on OnStatusFunc) *EmailPipeline {
	categorizer, _ := su.(Categorizer)
	return &EmailPipeline{stages: newStages(st, n, on, categorizer), classifier: cl, summarizer: su}
}

func (p *EmailPipeline) ContentType() task.ContentType { return task.ContentEmail }

func (p *EmailPipeline) Run(ctx context.Context, t *task.Task) error {
	if err := p.setStatus(ctx, t, task.StatusFiltering); err != nil {
		return err
	}

	start := time.Now()
	raw, err := p.classifier.Classify(ctx, t.RawContent())
	if err != nil {
		return p.fail(ctx, t, "邮件分类", err, start)
	}
	cat := emailCategory(strings.ToLower(strings.TrimSpace(raw)))
	if cat != emailImportant && cat != emailNotify && cat != emailSpam {
		cat = emailNotify // safe fallback for unexpected model output
	}
	if cat == emailImportant && !emailImportantExtractionEnabled(t.RawContent()) {
		cat = emailNotify
	}
	p.addStep(ctx, t, "邮件分类", task.StepOK, string(cat), start)

	if cat == emailSpam {
		t.SetFilterDecision(task.FilterDiscard)
		return p.setStatus(ctx, t, task.StatusDiscarded)
	}
	decision := task.FilterPass
	if cat == emailNotify {
		t.SetFilterDecision(task.FilterSilent)
		decision = task.FilterSilent
	} else {
		t.SetFilterDecision(task.FilterPass)
	}

	if err := p.setStatus(ctx, t, task.StatusAnalyzing); err != nil {
		return err
	}
	start = time.Now()
	summary, err := p.summarizer.Summarize(ctx, t, t.RawContent())
	if err != nil {
		return p.fail(ctx, t, "生成摘要", err, start)
	}
	t.SetSummary(summary)
	p.addStep(ctx, t, "生成摘要", task.StepOK, "", start)
	p.categorize(ctx, t, t.RawContent()+"\n\n摘要：\n"+summary)
	t.SetFilterDecision(decision)

	return p.saveAndNotify(ctx, t, summary)
}

func emailImportantExtractionEnabled(content string) bool {
	header := content
	if idx := strings.Index(content, "\n\n"); idx >= 0 {
		header = content[:idx]
	}
	for _, line := range strings.Split(header, "\n") {
		parts := strings.SplitN(line, "：", 2)
		if len(parts) != 2 {
			parts = strings.SplitN(line, ":", 2)
		}
		if len(parts) != 2 {
			continue
		}
		if strings.TrimSpace(parts[0]) == "重要邮件单独提取" {
			return strings.TrimSpace(strings.ToLower(parts[1])) != "false"
		}
	}
	return true
}

// ── MessagePipeline ───────────────────────────────────────────────────────────

type MessagePipeline struct {
	stages
	extractor Extractor
}

func NewMessage(ex Extractor, st Store, n Notifier, on OnStatusFunc) *MessagePipeline {
	categorizer, _ := ex.(Categorizer)
	return &MessagePipeline{stages: newStages(st, n, on, categorizer), extractor: ex}
}

func (p *MessagePipeline) ContentType() task.ContentType { return task.ContentMessage }

func (p *MessagePipeline) Run(ctx context.Context, t *task.Task) error {
	if err := p.setStatus(ctx, t, task.StatusAnalyzing); err != nil {
		return err
	}

	start := time.Now()
	digest, err := p.extractor.Extract(ctx, t.RawContent())
	if err != nil {
		return p.fail(ctx, t, "提炼关键信息", err, start)
	}
	t.SetSummary(digest)
	t.SetFilterDecision(task.FilterPass)
	p.addStep(ctx, t, "提炼关键信息", task.StepOK, "", start)
	p.categorize(ctx, t, t.RawContent()+"\n\n摘要：\n"+digest)

	return p.saveAndNotify(ctx, t, digest)
}

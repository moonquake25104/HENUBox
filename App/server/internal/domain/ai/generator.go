package ai

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"

	"kairos/server/internal/domain/calendar"
	"kairos/server/internal/domain/termphase"
	"kairos/server/internal/store"
)

// ErrInvalidDate is returned for a malformed ?date= value.
var ErrInvalidDate = errors.New("日期格式无效，应为 YYYY-MM-DD")

// Generator orchestrates the daily brief: per-date cache lookup, in-flight
// guard, briefing aggregation, AI/rule dispatch and the final upsert. It is
// safe for concurrent use. The exported GenerateBrief is the seam shared by the
// HTTP handler and the future W9 central scheduler.
type Generator struct {
	Q       *store.Queries
	DataDir string
	Client  *http.Client
	Now     func() time.Time

	guard *inflightGuard
}

// NewGenerator builds a generator with the default HTTP client and clock.
func NewGenerator(q *store.Queries, dataDir string) *Generator {
	return &Generator{
		Q:       q,
		DataDir: dataDir,
		Now:     time.Now,
		guard:   newInflightGuard(),
	}
}

// GenerateBrief generates and persists the brief for date (empty = today
// +08:00). force bypasses the daily cache. A non-nil onDelta switches to the
// SSE streaming provider and receives each content delta as it arrives.
func (g *Generator) GenerateBrief(ctx context.Context, date string, force bool, onDelta func(string)) (*store.AiMorningBrief, error) {
	resolvedDate, now, err := g.resolveDate(date)
	if err != nil {
		return nil, err
	}

	if !force {
		if brief, err := g.Q.GetMorningBrief(ctx, store.DateOf(resolvedDate)); err == nil {
			return &brief, nil
		} else if !errors.Is(err, pgx.ErrNoRows) {
			return nil, err
		}
	}

	if !g.guard.tryAcquire(resolvedDate) {
		if brief, err := g.Q.GetMorningBrief(ctx, store.DateOf(resolvedDate)); err == nil {
			return &brief, nil
		}
		return nil, ErrGenerating
	}
	defer g.guard.release(resolvedDate)

	briefing, err := g.collectBriefing(ctx, now)
	if err != nil {
		return nil, err
	}
	config, err := g.Q.GetAiConfig(ctx)
	if err != nil {
		return nil, err
	}

	result := g.dispatch(ctx, briefing, config, onDelta)

	generatedAt := g.now()
	saved, err := g.Q.UpsertMorningBrief(ctx, store.UpsertMorningBriefParams{
		Date:        store.DateOf(resolvedDate),
		Markdown:    result.Summary,
		Source:      string(result.Source),
		Model:       result.Model,
		GeneratedAt: pgtype.Timestamptz{Time: generatedAt, Valid: true},
	})
	if err != nil {
		return nil, err
	}
	return &saved, nil
}

func (g *Generator) dispatch(ctx context.Context, briefing *calendar.TodayBriefingResponse, config store.AiConfig, onDelta func(string)) AiBriefResult {
	rule := RuleBasedService{}

	provider, err := g.resolveProvider(config)
	if err != nil {
		slog.Warn("AI provider 不可用，降级本地规则", "error", err)
		return rule.Generate(briefing)
	}
	if provider == nil {
		return rule.Generate(briefing)
	}

	if onDelta != nil {
		text, err := provider.GenerateStreaming(ctx, briefing, onDelta)
		if err == nil && ValidateAIOutput(text) {
			return AiBriefResult{Summary: text, Source: SourceAI, Model: config.Model}
		}
		if err != nil {
			slog.Warn("AI 流式生成失败，降级本地规则", "error", err)
		} else {
			slog.Warn("AI 流式摘要结构校验失败，降级本地规则", "length", len([]rune(text)))
		}
		return rule.Generate(briefing)
	}

	res, err := provider.Generate(ctx, briefing)
	if err == nil && ValidateAIOutput(res.Summary) {
		return res
	}
	if err != nil {
		slog.Warn("AI 生成失败，降级本地规则", "error", err)
	} else {
		slog.Warn("AI 摘要结构校验失败，降级本地规则", "length", len([]rune(res.Summary)))
	}
	return rule.Generate(briefing)
}

func (g *Generator) resolveProvider(config store.AiConfig) (*Provider, error) {
	if !config.Enabled || config.ApiKeyEncrypted == "" {
		return nil, nil
	}
	key, err := EnsureKeyFile(g.DataDir)
	if err != nil {
		return nil, err
	}
	return ResolveProvider(config, key, g.Client)
}

func (g *Generator) collectBriefing(ctx context.Context, now time.Time) (*calendar.TodayBriefingResponse, error) {
	courses, err := g.Q.ListCourses(ctx, pgtype.Text{})
	if err != nil {
		return nil, err
	}
	contexts, err := g.Q.ListSemesterContexts(ctx)
	if err != nil {
		return nil, err
	}
	tasks, err := g.Q.ListTasks(ctx, store.ListTasksParams{})
	if err != nil {
		return nil, err
	}
	exams, err := g.Q.ListExams(ctx)
	if err != nil {
		return nil, err
	}
	phaseStatus, err := termphase.GetPhaseStatusForDate(ctx, g.Q, termphase.DefaultSource, now)
	if err != nil {
		return nil, err
	}
	pomodoro := buildPomodoroBriefing(ctx, g.Q, now)
	resp := calendar.BuildTodayBriefing(courses, contexts, tasks, exams, phaseStatus, pomodoro, now)
	return &resp, nil
}

func (g *Generator) resolveDate(date string) (string, time.Time, error) {
	now := g.now().In(calendar.ChinaTZ)
	today := now.Format("2006-01-02")
	if date == "" {
		return today, now, nil
	}
	parsed, err := time.ParseInLocation("2006-01-02", date, calendar.ChinaTZ)
	if err != nil {
		return "", time.Time{}, ErrInvalidDate
	}
	if date == today {
		return today, now, nil
	}
	return date, time.Date(parsed.Year(), parsed.Month(), parsed.Day(), 12, 0, 0, 0, calendar.ChinaTZ), nil
}

func (g *Generator) now() time.Time {
	if g.Now == nil {
		return time.Now()
	}
	return g.Now()
}

// TodayChina returns the current date in +08:00 as YYYY-MM-DD.
func TodayChina() string {
	return time.Now().In(calendar.ChinaTZ).Format("2006-01-02")
}

func buildPomodoroBriefing(ctx context.Context, q *store.Queries, now time.Time) calendar.PomodoroBriefing {
	start := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, calendar.ChinaTZ)
	completed, err := q.CountCompletedWorkSessions(ctx, store.CountCompletedWorkSessionsParams{
		WindowStart: pgtype.Timestamptz{Time: start.UTC(), Valid: true},
		WindowEnd:   pgtype.Timestamptz{Time: start.AddDate(0, 0, 1).UTC(), Valid: true},
	})
	if err != nil {
		completed = 0
	}

	state, err := q.GetRuntimeState(ctx)
	if errors.Is(err, pgx.ErrNoRows) {
		return calendar.PomodoroBriefing{Phase: "work", CompletedSessions: completed}
	}
	if err != nil {
		return calendar.PomodoroBriefing{Phase: "work", CompletedSessions: completed}
	}
	return calendar.PomodoroBriefing{
		IsRunning:         state.IsRunning,
		Phase:             state.Phase,
		RemainingSeconds:  int64(state.RemainingSeconds),
		CompletedSessions: completed,
	}
}

type inflightGuard struct {
	mu       sync.Mutex
	inFlight map[string]struct{}
}

func newInflightGuard() *inflightGuard {
	return &inflightGuard{inFlight: map[string]struct{}{}}
}

func (g *inflightGuard) tryAcquire(date string) bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if _, ok := g.inFlight[date]; ok {
		return false
	}
	g.inFlight[date] = struct{}{}
	return true
}

func (g *inflightGuard) release(date string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.inFlight, date)
}

// Package httpapi assembles the chi router, middleware chain and handlers.
package httpapi

import (
	"context"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"

	"kairos/server/internal/domain/academic"
	"kairos/server/internal/domain/ai"
	"kairos/server/internal/domain/ntfy"
	"kairos/server/internal/domain/pomodoro"
	"kairos/server/internal/domain/push"
	"kairos/server/internal/domain/sync"
	"kairos/server/internal/httpapi/handlers"
	"kairos/server/internal/httpapi/middleware"
	"kairos/server/internal/store"
)

// Options configures the router.
type Options struct {
	NativePush   *ntfy.Client
	Push         *push.Service
	Log          *slog.Logger
	Username     string
	PasswordHash string
	JWTSecret    []byte
	JWTTTL       time.Duration
	// DB is optional; when set, /healthz pings it.
	DB interface {
		Ping(ctx context.Context) error
	}
	// Store is optional; when set, the authenticated business routes are enabled.
	Store *store.Queries
	// Pool is the transaction-capable connection used by the sync module; when
	// set together with Store, the /api/sync routes are enabled.
	Pool sync.TxBeginner
	// DataDir is the server-side directory for the AI sync DEK and API-key
	// key files.
	DataDir string
	// PomodoroNotifier is the optional W9 email hook for finished pomodoro
	// phases; nil disables notifications.
	PomodoroNotifier pomodoro.Notifier
	// NotifyScheduler recomputes notification timers after mutations; nil
	// disables recomputation.
	NotifyScheduler handlers.NotifyRecomputer
	// AIGenerator, when set, is shared by the AI handler and the notify
	// scheduler so the morning-brief cache stays coherent.
	AIGenerator *ai.Generator
	// Henu, when set, enables the phase C direct-connection endpoints with
	// the given environment (key file, local interactive login, helper).
	Henu *handlers.HenuConnector
	// CORSAllowedOrigins is an exact-match allowlist for the /api group
	// (mobile app origin, e.g. https://localhost). Empty keeps the historic
	// no-CORS behavior for the desktop web client.
	CORSAllowedOrigins []string
}

// New builds the chi router with the full middleware chain and routes.
func New(opts Options) http.Handler {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(middleware.Logger(opts.Log))
	r.Use(middleware.Recover(opts.Log))

	r.Get("/healthz", handlers.Healthz(opts.DB))
	r.Get("/healthz/live", handlers.Healthz(nil))

	auth := &handlers.Auth{
		Username:     opts.Username,
		PasswordHash: opts.PasswordHash,
		JWTSecret:    opts.JWTSecret,
		TTL:          opts.JWTTTL,
		Log:          opts.Log,
	}

	r.Route("/api", func(api chi.Router) {
		// Preflight never carries credentials; it must answer before auth.
		api.Use(middleware.CORS(opts.CORSAllowedOrigins))
		api.Post("/auth/login", auth.Login)
		api.Group(func(protected chi.Router) {
			protected.Use(middleware.Auth(opts.JWTSecret))
			protected.Post("/auth/logout", auth.Logout)
			protected.Get("/auth/me", auth.Me)

			if opts.Store != nil {
				registerCoreRoutes(protected, opts)
			}
		})
	})

	return r
}

func registerCoreRoutes(protected chi.Router, opts Options) {
	native := &handlers.NativePush{Client: opts.NativePush}
	protected.Get("/ntfy/config", native.Config)
	protected.Post("/ntfy/setup", native.Setup)
	protected.Post("/ntfy/test", native.Test)
	if opts.Push != nil {
		p := &handlers.Push{Service: opts.Push}
		protected.Get("/push/config", p.Config)
		protected.Post("/push/subscriptions", p.Subscribe)
		protected.Get("/push/subscriptions/{id}", p.Device)
		protected.Delete("/push/subscriptions/{id}", p.Device)
		protected.Post("/push/test", p.Test)
		protected.Get("/push/tests", p.Tests)
		protected.Post("/push/tests/{id}/confirm", p.Confirm)
	}
	tasks := &handlers.Tasks{Q: opts.Store, Log: opts.Log, Scheduler: opts.NotifyScheduler}
	protected.Route("/tasks", func(rt chi.Router) {
		rt.Get("/", tasks.List)
		rt.Post("/", tasks.Create)
		rt.Patch("/{id}", tasks.Update)
		rt.Delete("/{id}", tasks.Delete)
		rt.Post("/{id}/complete", tasks.Complete)
		rt.Post("/{id}/uncomplete", tasks.Uncomplete)
	})

	academicService := &academic.Service{Q: opts.Store, Pool: opts.Pool, Log: opts.Log}

	courses := &handlers.Courses{Q: opts.Store, Log: opts.Log, Scheduler: opts.NotifyScheduler, Academic: academicService}
	protected.Route("/courses", func(rt chi.Router) {
		rt.Get("/", courses.List)
		rt.Post("/", courses.Create)
		rt.Patch("/{id}", courses.Update)
		rt.Delete("/{id}", courses.Delete)
		rt.Post("/import-text", courses.ImportText)
		rt.Post("/reset-semester-dates", courses.ResetSemesterDates)
	})

	exams := &handlers.Exams{Q: opts.Store, Log: opts.Log, Scheduler: opts.NotifyScheduler}
	protected.Route("/exams", func(rt chi.Router) {
		rt.Get("/", exams.List)
		rt.Post("/", exams.Create)
		rt.Patch("/{id}", exams.Update)
		rt.Delete("/{id}", exams.Delete)
		rt.Post("/import-text", exams.ImportText)
	})

	semester := &handlers.Semester{Q: opts.Store, Log: opts.Log, Scheduler: opts.NotifyScheduler}
	protected.Get("/semesters", semester.ListSemesters)
	protected.Route("/term-phases", func(rt chi.Router) {
		rt.Get("/", semester.ListPhases)
		rt.Post("/", semester.CreatePhase)
		rt.Patch("/{id}", semester.UpdatePhase)
		rt.Delete("/{id}", semester.DeletePhase)
		rt.Get("/current-status", semester.CurrentStatus)
	})

	academicHandlers := &handlers.Academic{
		Q:       opts.Store,
		Pool:    opts.Pool,
		Log:     opts.Log,
		DataDir: opts.DataDir,
		Henu:    opts.Henu,
	}
	if opts.Pool != nil {
		protected.Route("/academic", func(rt chi.Router) {
			rt.Get("/semester-settings", academicHandlers.GetSemesterSettings)
			rt.Put("/semester-settings", academicHandlers.PutSemesterSettings)
			rt.Post("/henu/previews", academicHandlers.CreatePreview)
			rt.Get("/henu/previews/{preview_id}", academicHandlers.GetPreview)
			rt.Post("/henu/previews/{preview_id}/rebase", academicHandlers.RebasePreview)
			rt.Post("/henu/previews/{preview_id}/commit", academicHandlers.CommitPreview)
			rt.Get("/commits/{commit_id}", academicHandlers.GetCommit)
			rt.Patch("/sessions/{session_id}/overrides", academicHandlers.PatchSessionOverrides)
			rt.Get("/timetable", academicHandlers.Timetable)
			rt.Get("/henu/connection", academicHandlers.GetConnection)
			rt.Delete("/henu/connection", academicHandlers.DeleteConnection)
			rt.Post("/henu/login-sessions", academicHandlers.CreateLoginSession)
			rt.Get("/henu/login-sessions/{login_id}", academicHandlers.GetLoginSession)
			rt.Delete("/henu/login-sessions/{login_id}", academicHandlers.DeleteLoginSession)
			rt.Post("/henu/sync-jobs", academicHandlers.CreateSyncJob)
			rt.Get("/henu/sync-jobs/{job_id}", academicHandlers.GetSyncJob)
		})
		// Tasks orphaned by a restart fail fast; login profile directories
		// from earlier processes are cleaned before anything can recreate one.
		if opts.Henu != nil {
			opts.Henu.RecoverInterrupted(context.Background(), opts.Store)
		}
	}

	calendar := &handlers.Calendar{Q: opts.Store, Log: opts.Log}
	protected.Route("/calendar", func(rt chi.Router) {
		rt.Get("/week", calendar.Week)
		rt.Get("/week-events", calendar.WeekEvents)
		rt.Get("/day", calendar.Day)
	})

	briefing := &handlers.Briefing{Q: opts.Store, Log: opts.Log}
	protected.Get("/briefing/today", briefing.Today)

	aiHandlers := handlers.NewAIWithGenerator(opts.Store, opts.Log, opts.DataDir, opts.AIGenerator)
	protected.Route("/ai", func(rt chi.Router) {
		rt.Get("/config", aiHandlers.GetConfig)
		rt.Patch("/config", aiHandlers.UpdateConfig)
		rt.Get("/morning-brief", aiHandlers.GetMorningBrief)
		rt.Post("/morning-brief/generate", aiHandlers.GenerateMorningBrief)
		rt.Get("/sync-recovery-key", aiHandlers.GetRecoveryKey)
		rt.Post("/sync-recovery-key", aiHandlers.SetRecoveryKey)
	})

	notifyHandlers := handlers.NewNotify(opts.Store, opts.Log, opts.NotifyScheduler)
	protected.Route("/notify", func(rt chi.Router) {
		rt.Get("/config", notifyHandlers.GetConfig)
		rt.Patch("/config", notifyHandlers.UpdateConfig)
	})

	pomodoroHandlers := handlers.NewPomodoro(opts.Store, opts.Log, opts.PomodoroNotifier)
	protected.Route("/pomodoro", func(rt chi.Router) {
		rt.Get("/state", pomodoroHandlers.State)
		rt.Post("/start", pomodoroHandlers.Start)
		rt.Post("/pause", pomodoroHandlers.Pause)
		rt.Post("/reset", pomodoroHandlers.Reset)
		rt.Post("/interrupt", pomodoroHandlers.Interrupt)
		rt.Post("/finish-phase", pomodoroHandlers.FinishPhase)
		rt.Get("/config", pomodoroHandlers.GetConfig)
		rt.Patch("/config", pomodoroHandlers.UpdateConfig)
		rt.Get("/profiles", pomodoroHandlers.ListProfiles)
		rt.Post("/profiles", pomodoroHandlers.CreateProfile)
		rt.Patch("/profiles/{id}", pomodoroHandlers.UpdateProfile)
		rt.Delete("/profiles/{id}", pomodoroHandlers.DeleteProfile)
	})

	if opts.Pool != nil {
		syncHandlers := handlers.NewSync(opts.Store, opts.Pool, opts.Log, opts.DataDir, opts.Username)
		protected.Route("/sync", func(rt chi.Router) {
			rt.Get("/config", syncHandlers.GetConfig)
			rt.Patch("/config", syncHandlers.UpdateConfig)
			rt.Post("/test", syncHandlers.TestConnection)
			rt.Post("/now", syncHandlers.SyncNow)
			rt.Get("/ai-recovery-key", syncHandlers.GetRecoveryKey)
			rt.Post("/ai-recovery-key", syncHandlers.SetRecoveryKey)
		})
	}
}

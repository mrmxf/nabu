package httpapi

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/mrmxf/nabu/internal/agentsvc"
)

// defaultMaxDepth is the starting per-group concurrent-job cap when a group is
// auto-created on first submit. Teachers can raise/lower it later.
const defaultMaxDepth = 3

// defaultImagesBudget is the starting per-group image quota (an image agent's "make an
// image" mode) used when the lesson doesn't author an images_budget. Teachers can
// adjust a group's remaining count live from the dashboard.
const defaultImagesBudget = 15

// Job TTL policy. A job carries a resolved TTL (seconds); the worker enforces it
// on its own clock, and the sweeper enforces dispatched_at + ttl + grace on
// nabu's clock — both skew-free (see the design discussion). The authored
// t_agent.timeout is clamped to [minTTL, maxTTL]; maxTTL is env-tunable so an
// operator can raise the ceiling for a heavier course without editing lessons.
const (
	defaultTTLSeconds = 60
	minTTLSeconds     = 5
	fallbackMaxTTL    = 120 // ceiling if AGENT_MAX_TTL unset
	ttlGraceSeconds   = 10  // slack before the sweeper reaps a running job
	sweepInterval     = 15 * time.Second
)

// maxTTLSeconds is the clamp ceiling, from AGENT_MAX_TTL or the fallback.
func maxTTLSeconds() int {
	if v := os.Getenv("AGENT_MAX_TTL"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return fallbackMaxTTL
}

// resolveTTL parses an authored timeout ("60s", "1m", or bare seconds) and clamps
// it to [minTTL, maxTTL]. An empty/invalid value falls back to the default.
func resolveTTL(s string) int {
	secs := defaultTTLSeconds
	s = strings.TrimSpace(s)
	if s != "" {
		if d, err := time.ParseDuration(s); err == nil {
			secs = int(d.Seconds())
		} else if n, err := strconv.Atoi(s); err == nil {
			secs = n
		}
	}
	if secs < minTTLSeconds {
		secs = minTTLSeconds
	}
	if m := maxTTLSeconds(); secs > m {
		secs = m
	}
	return secs
}

// submitBody is the job-submit request from the agent widget.
type submitBody struct {
	SessionID string         `json:"session_id"`
	AgentID   string         `json:"agent_id"` // the slide's t_agent id (job step)
	Agent     string         `json:"agent"`    // worker-advertised tool name
	Inputs    map[string]any `json:"inputs"`
	Timeout   string         `json:"timeout"`
	Slide     string         `json:"slide"`
}

// jobSubmitHandler accepts a job from a student, enforces the per-group queue
// cap, records the job, and dispatches it to the worker in the background. The
// widget then follows the job over PocketBase realtime.
//
// The server is the authority for what actually runs: it reloads the slide's
// authored t_agent config (agent, timeout, inputs + per-key policy) and rebuilds
// the input set, so a student can only change group-editable keys within their
// declared bounds (see agentcfg.go). The client's `agent`/`timeout` and any
// fixed/hidden input value are advisory only.
func jobSubmitHandler(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.UnauthorizedError("authentication required", nil)
	}
	var body submitBody
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	if body.SessionID == "" {
		return e.BadRequestError("session_id is required", nil)
	}

	app := e.App

	// resolve the session (must belong to the caller) and its group.
	sess, err := app.FindRecordById("sessions", body.SessionID)
	if err != nil {
		return e.NotFoundError("session not found", err)
	}
	if sess.GetString("user") != e.Auth.Id {
		return e.ForbiddenError("not your session", nil)
	}

	// authored config is the source of truth for agent/timeout/inputs. When it
	// resolves, the client's agent/timeout/fixed-inputs are ignored; otherwise we
	// fall back to the client-sent values (legacy/unauthored slides).
	agent := body.Agent
	timeout := body.Timeout
	inputs := body.Inputs
	if aj := authoredAgentJob(app, sess.GetString("lesson"), body.Slide, body.AgentID); aj != nil {
		agent = aj.Agent
		timeout = aj.Timeout
		inputs = effectiveInputs(aj, body.Inputs)
		// An authored agent may be a `{{choice:…}}` token — e.g. the group's chosen
		// controller (pip|orca|buzz). Only the client can resolve it from the
		// group's live choices (which live in the group's Yjs doc, not the DB), so
		// trust the client's resolved name here. It stays gated by the online-agents
		// check below, so only a real, worker-advertised agent can ever run.
		if strings.Contains(aj.Agent, "{{") && body.Agent != "" {
			agent = body.Agent
		}
	}
	if agent == "" {
		return e.BadRequestError("agent is required", nil)
	}
	if strings.Contains(agent, "{{") {
		// An unresolved token means the group hasn't made the choice this agent
		// depends on (e.g. no controller picked yet). Say so plainly.
		return e.BadRequestError("this agent depends on a group choice that hasn't been made yet", nil)
	}

	// worker must be up and advertising this agent.
	h := prober.Health(e.Request.Context())
	if !h.Alive {
		return e.JSON(http.StatusServiceUnavailable, map[string]any{"error": "server_unreachable"})
	}
	if !contains(h.Agents, agent) {
		return e.JSON(http.StatusBadRequest, map[string]any{"error": "agent_unavailable", "agent": agent})
	}

	group, err := resolveGroup(app, sess)
	if err != nil {
		return e.InternalServerError("could not resolve group", err)
	}

	// per-group quota: concurrent queued+running jobs.
	active, err := app.FindRecordsByFilter("jobs",
		"group = {:g} && (status = 'queued' || status = 'running')", "", 0, 0,
		dbx.Params{"g": group.Id})
	if err != nil {
		return e.InternalServerError("could not check queue", err)
	}
	maxDepth := group.GetInt("max_depth")
	if maxDepth <= 0 {
		maxDepth = defaultMaxDepth
	}
	if group.GetBool("paused") {
		return e.JSON(http.StatusLocked, map[string]any{"error": "submissions_paused"})
	}
	if len(active) >= maxDepth {
		return e.JSON(http.StatusTooManyRequests, map[string]any{
			"error": "queue_full", "group_active": len(active), "group_max": maxDepth,
		})
	}

	// image generation gate: when the Stable Diffusion back-end is disabled
	// (app_config `images_enabled` = false, seeded from ai.stable-diffusion.enabled),
	// refuse image submits outright — the SPA hides the surface, but a stale client
	// or a direct call must not sneak an image job through either.
	imageJob := isImageJob(agent, inputs)
	if imageJob && !imagesEnabled(app) {
		return e.JSON(http.StatusForbidden, map[string]any{"error": "images_disabled"})
	}

	// per-group image budget: an image job reserves one of the group's remaining
	// images up front (atomically), and is refunded below if the job never runs.
	if imageJob {
		exhausted, err := reserveImage(app, group.Id)
		if err != nil {
			return e.InternalServerError("could not reserve image", err)
		}
		if exhausted {
			return e.JSON(http.StatusTooManyRequests, map[string]any{
				"error": "images_exhausted", "images_remaining": 0,
			})
		}
	}

	// record the job.
	col, err := app.FindCollectionByNameOrId("jobs")
	if err != nil {
		return e.InternalServerError("jobs collection missing", err)
	}
	job := core.NewRecord(col)
	job.Set("request_id", security.RandomString(15))
	job.Set("student", e.Auth.Id)
	job.Set("group", group.Id)
	job.Set("session", sess.Id)
	job.Set("course", sess.GetString("course"))
	job.Set("lesson", sess.GetString("lesson"))
	job.Set("slide", body.Slide)
	ttl := resolveTTL(timeout)
	job.Set("agent", agent)
	job.Set("inputs", inputs)
	job.Set("timeout_s", ttl)
	job.Set("status", "queued")
	job.Set("queue_pos", len(active))
	if err := app.Save(job); err != nil {
		if imageJob {
			refundImage(app, group.Id) // reserved above but no job to run
		}
		return e.InternalServerError("could not create job", err)
	}
	logEvent(app, job.Id, "info", "queued", "job accepted")

	// dispatch in the background; the widget follows status over realtime.
	go dispatchJob(app, job.Id, agent, inputs, ttl)

	return e.JSON(http.StatusOK, map[string]any{
		"job": map[string]any{"id": job.Id, "status": "queued", "queue_pos": len(active)},
	})
}

// dispatchJob runs a queued job on the worker and records the outcome. Runs in
// its own goroutine with a background (request-independent) context, so the
// student can navigate away while it completes.
func dispatchJob(app core.App, jobID, agent string, inputs map[string]any, ttl int) {
	// nabu's own deadline is ttl + grace: the worker (ttl) should abort first and
	// report a clean timeout; this is the backstop if the transport wedges.
	ctx, cancel := context.WithTimeout(context.Background(),
		time.Duration(ttl+ttlGraceSeconds)*time.Second)
	defer cancel()

	job, err := app.FindRecordById("jobs", jobID)
	if err != nil {
		slog.Error("dispatch: job vanished", "job", jobID, "err", err)
		return
	}
	job.Set("status", "running")
	job.Set("dispatched_at", types.NowDateTime())
	job.Set("worker", prober.WorkerLabel())
	if err := app.Save(job); err != nil {
		slog.Error("dispatch: could not mark running", "job", jobID, "err", err)
	}
	logEvent(app, jobID, "info", "running", "dispatched to worker")

	res, derr := prober.Dispatch(ctx, agentsvc.DispatchRequest{
		JobID: jobID, Agent: agent, Inputs: inputs, TTLSec: ttl,
	})

	// reload before writing the terminal state (the record may be stale).
	job, err = app.FindRecordById("jobs", jobID)
	if err != nil {
		slog.Error("dispatch: job vanished before completion", "job", jobID, "err", err)
		return
	}
	// a job the sweeper already reaped is terminal — don't resurrect it.
	if s := job.GetString("status"); s == "failed" || s == "done" {
		return
	}
	job.Set("completed_at", types.NowDateTime())
	if derr != nil {
		timedOut := ctx.Err() == context.DeadlineExceeded || strings.Contains(strings.ToLower(derr.Error()), "timeout")
		event := "fail"
		if timedOut {
			event = "timeout"
		}
		job.Set("status", "failed")
		job.Set("error", derr.Error())
		_ = app.Save(job)
		// an image job that reserved a token but never produced an image gets it back.
		if isImageJob(agent, inputs) {
			refundImage(app, job.GetString("group"))
		}
		logEvent(app, jobID, "warn", event, derr.Error())
		slog.Warn("job "+event, "job", jobID, "agent", agent, "err", derr)
		return
	}
	job.Set("status", "done")
	job.Set("result_html", res.ResultHTML)
	if err := app.Save(job); err != nil {
		slog.Error("dispatch: could not mark done", "job", jobID, "err", err)
	}
	logEvent(app, jobID, "info", "complete", "job complete")
}

// resolveGroup finds (or auto-creates on first join) the group for an attempt,
// keyed by (lesson, session, tag) — a group belongs to ONE Session (the attempt's
// schedule_session cohort window), so the same tag `g1` in two Sessions of one
// lesson is two independent groups with independent shared state (matching the
// Yjs room ${lesson}:${session}:${group}; see docs/glossary.md).
//
// An unscheduled attempt (no schedule_session) or one with no group name is SOLO:
// its own private group keyed by a per-attempt tag, never shared. So a shared
// group requires BOTH a scheduled Session and a group name.
func resolveGroup(app core.App, sess *core.Record) (*core.Record, error) {
	lesson := sess.GetString("lesson")
	session := sess.GetString("schedule_session")
	tag := normalizeTag(sess.GetString("group_name"))
	if session == "" || tag == "" {
		// solo: unique per attempt, unscheduled, session left empty.
		return findOrCreateGroup(app, lesson, "", "solo-"+sess.Id)
	}
	return findOrCreateGroup(app, lesson, session, tag)
}

// findOrCreateGroup returns the group for (lesson, session, tag), creating it with
// the default quota on first use. Idempotent and race-safe: a lost create
// re-fetches the winner. Callers pass an already-normalized tag; `session` is the
// schedule_sessions id, or "" for a solo/unscheduled group. Shared by the job
// submit path (resolveGroup) and the student "start a new group" endpoint — both
// create groups in app context, which bypasses the staff-only collection create
// rule, while max_depth/paused stay server-controlled (never client-set).
func findOrCreateGroup(app core.App, lesson, session, tag string) (*core.Record, error) {
	// An empty `session` relation isn't matched by `session = {:s}`, so for a solo
	// group (session unset) look up by (lesson, tag) alone — the "solo-<attemptId>"
	// tag is already unique, so there's no ambiguity.
	params := dbx.Params{"l": lesson, "t": tag}
	filter := "lesson = {:l} && tag = {:t} && session = {:s}"
	if session == "" {
		filter = "lesson = {:l} && tag = {:t}"
	} else {
		params["s"] = session
	}
	if existing, err := app.FindFirstRecordByFilter("groups", filter, params); err == nil {
		return existing, nil
	}
	col, err := app.FindCollectionByNameOrId("groups")
	if err != nil {
		return nil, err
	}
	g := core.NewRecord(col)
	g.Set("tag", tag)
	g.Set("lesson", lesson)
	g.Set("session", session)
	g.Set("max_depth", defaultMaxDepth)
	g.Set("images_remaining", lessonImagesBudget(app, lesson))
	if err := app.Save(g); err != nil {
		// lost a create race: re-fetch the winner.
		if again, e2 := app.FindFirstRecordByFilter("groups", filter, params); e2 == nil {
			return again, nil
		}
		return nil, err
	}
	return g, nil
}

// normalizeTag lowercases and strips whitespace so "S7 G1" and "s7g1" collapse to
// one quota bucket.
func normalizeTag(s string) string {
	return strings.ToLower(strings.Join(strings.Fields(s), ""))
}

// isImageJob reports whether a dispatch makes an image: any agent whose authored
// `format` input is "image" (the SPA applies the same test). Image jobs draw down
// the group's images_remaining budget. agent is unused but kept so callers read
// naturally; the format, not the agent's name, decides.
func isImageJob(_ string, inputs map[string]any) bool {
	f, _ := inputs["format"].(string)
	return f == "image"
}

// imagesEnabled reports whether image-making agents are exposed, reading the
// app_config `images_enabled` row (seeded from nabu-config.yaml
// `ai.stable-diffusion.enabled`). Defaults to false — off unless explicitly
// enabled — when the row is absent or unparseable, matching the SPA gate.
func imagesEnabled(app core.App) bool {
	rec, err := app.FindFirstRecordByData("app_config", "key", "images_enabled")
	if err != nil || rec == nil {
		return false
	}
	return rec.GetString("value") == "true"
}

// reserveImage atomically decrements a group's images_remaining by one. It reports
// exhausted=true (without decrementing) when the group is already at zero, so the
// caller can reject the submit. The transaction makes two concurrent image submits
// from one group draw down the budget by exactly two, never one.
func reserveImage(app core.App, groupID string) (exhausted bool, err error) {
	err = app.RunInTransaction(func(txApp core.App) error {
		g, err := txApp.FindRecordById("groups", groupID)
		if err != nil {
			return err
		}
		if g.GetInt("images_remaining") <= 0 {
			exhausted = true
			return nil
		}
		g.Set("images_remaining", g.GetInt("images_remaining")-1)
		return txApp.Save(g)
	})
	return exhausted, err
}

// refundImage returns one image to a group's budget after a reserved image job
// never completed (create failed, or the worker errored/timed out), so a crash
// doesn't cost the group a token. Best-effort: a lost refund is logged, not fatal.
func refundImage(app core.App, groupID string) {
	if groupID == "" {
		return
	}
	err := app.RunInTransaction(func(txApp core.App) error {
		g, err := txApp.FindRecordById("groups", groupID)
		if err != nil {
			return err
		}
		g.Set("images_remaining", g.GetInt("images_remaining")+1)
		return txApp.Save(g)
	})
	if err != nil {
		slog.Warn("could not refund image budget", "group", groupID, "err", err)
	}
}

// refundImageJob refunds the budget for a failed job record if it was an image
// job. Used by the failure paths that only have the job record (sweeper, restart
// reconcile), which reserved a token at submit that no image will ever spend.
func refundImageJob(app core.App, j *core.Record) {
	var inputs map[string]any
	if err := j.UnmarshalJSONField("inputs", &inputs); err != nil {
		return
	}
	if isImageJob(j.GetString("agent"), inputs) {
		refundImage(app, j.GetString("group"))
	}
}

// lessonImagesBudget returns the authored per-group image quota for a lesson,
// falling back to defaultImagesBudget when the lesson is missing or leaves
// images_budget unset (0). Used to seed a new group's images_remaining.
func lessonImagesBudget(app core.App, lesson string) int {
	if lesson == "" {
		return defaultImagesBudget
	}
	rec, err := app.FindRecordById("lessons", lesson)
	if err != nil {
		return defaultImagesBudget
	}
	if b := rec.GetInt("images_budget"); b > 0 {
		return b
	}
	return defaultImagesBudget
}

// ReconcileOrphanJobs fails any job left queued/running from a previous process:
// if nabu restarts mid-dispatch, those jobs would otherwise consume their group's
// quota forever. Call once on serve startup. Best-effort and non-fatal.
func ReconcileOrphanJobs(app core.App) {
	orphans, err := app.FindRecordsByFilter("jobs",
		"status = 'queued' || status = 'running'", "", 0, 0)
	if err != nil {
		slog.Warn("orphan reconcile: query failed", "err", err)
		return
	}
	for _, j := range orphans {
		j.Set("status", "failed")
		j.Set("error", "interrupted by server restart")
		j.Set("completed_at", types.NowDateTime())
		if err := app.Save(j); err != nil {
			slog.Warn("orphan reconcile: save failed", "job", j.Id, "err", err)
			continue
		}
		refundImageJob(app, j)
		logEvent(app, j.Id, "warn", "fail", "interrupted by server restart")
	}
	if len(orphans) > 0 {
		slog.Info("reconciled orphaned jobs on startup", "count", len(orphans))
	}
}

// StartJobSweeper runs a periodic reaper that fails any running job past its
// deadline (dispatched_at + ttl + grace), on nabu's clock. In the current
// synchronous model dispatchJob usually reaps its own job first; the sweeper is
// the belt-and-suspenders — and it becomes the PRIMARY timeout enforcer once
// dispatch is async (worker completes via callback, so no dispatchJob goroutine
// is holding the job). Call once on serve startup.
func StartJobSweeper(app core.App) {
	go func() {
		t := time.NewTicker(sweepInterval)
		defer t.Stop()
		for range t.C {
			sweepStuckJobs(app)
		}
	}()
}

// sweepStuckJobs fails running jobs whose deadline has passed. Idempotent: a job
// dispatchJob already completed is no longer "running", so it's skipped.
func sweepStuckJobs(app core.App) {
	running, err := app.FindRecordsByFilter("jobs", "status = 'running'", "", 0, 0)
	if err != nil {
		slog.Warn("sweep: query failed", "err", err)
		return
	}
	now := time.Now()
	for _, j := range running {
		disp := j.GetDateTime("dispatched_at").Time()
		if disp.IsZero() {
			continue // not actually dispatched yet
		}
		ttl := j.GetInt("timeout_s")
		if ttl <= 0 {
			ttl = defaultTTLSeconds
		}
		deadline := disp.Add(time.Duration(ttl+ttlGraceSeconds) * time.Second)
		if now.Before(deadline) {
			continue
		}
		j.Set("status", "failed")
		j.Set("error", fmt.Sprintf("timed out after %ds", ttl))
		j.Set("completed_at", types.NowDateTime())
		if err := app.Save(j); err != nil {
			slog.Warn("sweep: save failed", "job", j.Id, "err", err)
			continue
		}
		refundImageJob(app, j)
		logEvent(app, j.Id, "warn", "timeout", fmt.Sprintf("swept: exceeded ttl %ds", ttl))
		slog.Warn("swept stuck job", "job", j.Id, "ttl_s", ttl)
	}
}

// logEvent appends a job_events row — the slog stream surfaced to the widget and
// the teacher dashboard. Best-effort: a lost event never fails the job.
func logEvent(app core.App, jobID, level, event, msg string) {
	col, err := app.FindCollectionByNameOrId("job_events")
	if err != nil {
		return
	}
	ev := core.NewRecord(col)
	ev.Set("job", jobID)
	ev.Set("level", level)
	ev.Set("event", event)
	ev.Set("msg", msg)
	if err := app.Save(ev); err != nil {
		slog.Warn("could not write job_event", "job", jobID, "event", event, "err", err)
	}
}

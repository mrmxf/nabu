package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

// init registers the Phase 3 execution schema — the control-plane side of the
// real-agent job pipeline (see docs/phase3-tracking-spec.md):
//
//	groups      one workshop group (sXgY), the unit rate limiting is scoped to
//	jobs        one agent job submitted by a student, with queue + result state
//	job_events  append-only per-job event/log stream (the slog stream, surfaced
//	            to the student widget and the teacher dashboard)
//	artifacts   the take-home output of a finished job (HTML | JSON | PDF)
//
// Division of responsibility (see the spec): nabu owns identity, the jobs table,
// and per-group rate limiting; the tailnet worker is identity-free and only runs
// jobs + reports capacity. So jobs/job_events/artifacts are written by the server
// (superuser context) — students never create them directly. Their Create/Update
// rules are therefore nil (superuser-only); students reach the pipeline through
// POST /api/job/submit, and read their own rows via the View/List rules below.
//
// A slide also gains a `t_agent` JSON field: the list of agent-job configs an
// {{< LMS t="agent" id="…" >}} widget dispatches, mirroring `t_choice`.
func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return fmt.Errorf("find users collection: %w", err)
		}
		classes, err := app.FindCollectionByNameOrId("classes")
		if err != nil {
			return fmt.Errorf("find classes collection: %w", err)
		}
		lessons, err := app.FindCollectionByNameOrId("lessons")
		if err != nil {
			return fmt.Errorf("find lessons collection: %w", err)
		}
		sessions, err := app.FindCollectionByNameOrId("sessions")
		if err != nil {
			return fmt.Errorf("find sessions collection: %w", err)
		}

		// Common rule fragments.
		authed := "@request.auth.id != \"\""
		staff := "@request.auth.role = \"teacher\" || @request.auth.role = \"admin\""

		// ---- groups --------------------------------------------------------
		// One workshop group keyed by its sXgY tag. `max_depth` is the per-group
		// queue cap the teacher can adjust live from the dashboard; `paused` is a
		// per-group stop switch. Students may read groups (the serverstatus widget
		// shows their group tag + depth); only staff may create/adjust them.
		groups := core.NewBaseCollection("groups")
		groups.Fields.Add(
			&core.TextField{Name: "tag", Required: true}, // e.g. "s7g1"
			&core.RelationField{Name: "class", CollectionId: classes.Id, MaxSelect: 1},
			// max_depth: concurrent (queued+running) jobs allowed for this group.
			// Default cap is applied at creation by the submit path; 3 is the
			// starting value used across the demo.
			&core.NumberField{Name: "max_depth", OnlyInt: true},
			&core.BoolField{Name: "paused"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		groups.AddIndex("idx_groups_tag", true, "tag", "")
		groups.ListRule = types.Pointer(authed)
		groups.ViewRule = types.Pointer(authed)
		groups.CreateRule = types.Pointer(staff)
		groups.UpdateRule = types.Pointer(staff)
		groups.DeleteRule = types.Pointer(staff)
		if err := app.Save(groups); err != nil {
			return fmt.Errorf("save groups: %w", err)
		}

		// ---- jobs ----------------------------------------------------------
		// One agent job. Every id needed for rate limiting + audit is stamped at
		// submit: request_id (idempotency), student, group, session, class,
		// lesson, slide, agent. Rate limiting counts rows where
		// group = X AND status IN (queued, running) against groups.max_depth.
		ownerOrStaff := fmt.Sprintf("%s && (student = @request.auth.id || %s)", authed, staff)
		jobs := core.NewBaseCollection("jobs")
		jobs.Fields.Add(
			// server-generated idempotency key; dedupes double-clicks.
			&core.TextField{Name: "request_id", Required: true},
			&core.RelationField{Name: "student", Required: true, CollectionId: users.Id, MaxSelect: 1, CascadeDelete: true},
			&core.RelationField{Name: "group", CollectionId: groups.Id, MaxSelect: 1},
			&core.RelationField{Name: "session", CollectionId: sessions.Id, MaxSelect: 1},
			&core.RelationField{Name: "class", CollectionId: classes.Id, MaxSelect: 1},
			&core.RelationField{Name: "lesson", CollectionId: lessons.Id, MaxSelect: 1},
			&core.TextField{Name: "slide"},      // slide slug the widget sits on
			&core.TextField{Name: "agent", Required: true}, // worker-advertised agent name
			&core.JSONField{Name: "inputs"},     // resolved payload sent to the worker
			&core.NumberField{Name: "timeout_s", OnlyInt: true},
			&core.SelectField{Name: "status", MaxSelect: 1,
				Values: []string{"queued", "running", "done", "failed"}},
			&core.NumberField{Name: "queue_pos", OnlyInt: true},
			&core.TextField{Name: "worker"}, // which worker picked it up
			// result_html is the rendered job result shown inline in the widget. A
			// take-home artifact record is a later slice; this keeps the running
			// framework simple (one field the widget reads over realtime).
			&core.EditorField{Name: "result_html"},
			&core.TextField{Name: "error"},
			&core.NumberField{Name: "retries", OnlyInt: true},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.DateField{Name: "dispatched_at"},
			&core.DateField{Name: "completed_at"},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		// the quota count filters on (group, status); the widget subscribes by student.
		jobs.AddIndex("idx_jobs_group_status", false, "`group`, status", "")
		jobs.AddIndex("idx_jobs_student", false, "student", "")
		jobs.AddIndex("idx_jobs_request", true, "request_id", "")
		jobs.ListRule = types.Pointer(ownerOrStaff)
		jobs.ViewRule = types.Pointer(ownerOrStaff)
		// Create/Update/Delete are server-only: students submit via /api/job/submit,
		// and the worker's callbacks update rows, both in superuser context.
		if err := app.Save(jobs); err != nil {
			return fmt.Errorf("save jobs: %w", err)
		}

		// ---- job_events ----------------------------------------------------
		// Append-only per-job event/log stream — this IS the slog stream (see the
		// logging section of CLAUDE.md), surfaced to the student widget and the
		// teacher dashboard. Readable by the job's owner or staff (relation rule);
		// written only by the server.
		eventsRule := fmt.Sprintf("%s && (job.student = @request.auth.id || %s)", authed, staff)
		jobEvents := core.NewBaseCollection("job_events")
		jobEvents.Fields.Add(
			&core.RelationField{Name: "job", Required: true, CollectionId: jobs.Id, MaxSelect: 1, CascadeDelete: true},
			&core.SelectField{Name: "level", MaxSelect: 1,
				Values: []string{"debug", "info", "warn", "error"}},
			&core.TextField{Name: "event"}, // machine name: queued|dispatched|log|complete|fail
			&core.TextField{Name: "msg"},
			&core.JSONField{Name: "fields"}, // structured slog fields
			&core.AutodateField{Name: "created", OnCreate: true},
		)
		jobEvents.AddIndex("idx_job_events_job", false, "job, created", "")
		jobEvents.ListRule = types.Pointer(eventsRule)
		jobEvents.ViewRule = types.Pointer(eventsRule)
		if err := app.Save(jobEvents); err != nil {
			return fmt.Errorf("save job_events: %w", err)
		}

		// ---- artifacts -----------------------------------------------------
		// The take-home output of a finished job. `file` holds it for the demo;
		// an S3 URL path can replace it later (per CLAUDE.md archival). A job's
		// artifact is found by querying artifacts where job = X (no back-relation
		// on jobs, to avoid a circular schema dependency).
		artifacts := core.NewBaseCollection("artifacts")
		artifacts.Fields.Add(
			&core.RelationField{Name: "job", Required: true, CollectionId: jobs.Id, MaxSelect: 1, CascadeDelete: true},
			&core.RelationField{Name: "student", CollectionId: users.Id, MaxSelect: 1},
			&core.SelectField{Name: "kind", MaxSelect: 1, Values: []string{"html", "json", "pdf"}},
			&core.TextField{Name: "title"},
			&core.FileField{Name: "file", MaxSelect: 1},
			&core.AutodateField{Name: "created", OnCreate: true},
		)
		artifacts.AddIndex("idx_artifacts_job", false, "job", "")
		artifactsRule := fmt.Sprintf("%s && (student = @request.auth.id || %s)", authed, staff)
		artifacts.ListRule = types.Pointer(artifactsRule)
		artifacts.ViewRule = types.Pointer(artifactsRule)
		if err := app.Save(artifacts); err != nil {
			return fmt.Errorf("save artifacts: %w", err)
		}

		// ---- slides.t_agent ------------------------------------------------
		// The list of agent-job configs a slide can dispatch, mirroring t_choice:
		// bulky config in frontmatter, placed by {{< LMS t="agent" id="…" >}}.
		slides, err := app.FindCollectionByNameOrId("slides")
		if err != nil {
			return fmt.Errorf("find slides collection: %w", err)
		}
		slides.Fields.Add(&core.JSONField{Name: "t_agent"})
		if err := app.Save(slides); err != nil {
			return fmt.Errorf("update slides: %w", err)
		}

		return nil
	}, func(app core.App) error {
		// down: drop t_agent, then delete collections in reverse dependency order.
		if slides, err := app.FindCollectionByNameOrId("slides"); err == nil {
			slides.Fields.RemoveByName("t_agent")
			if err := app.Save(slides); err != nil {
				return fmt.Errorf("revert slides: %w", err)
			}
		}
		for _, name := range []string{"artifacts", "job_events", "jobs", "groups"} {
			c, err := app.FindCollectionByNameOrId(name)
			if err != nil {
				continue // already gone
			}
			if err := app.Delete(c); err != nil {
				return fmt.Errorf("delete %s: %w", name, err)
			}
		}
		return nil
	})
}

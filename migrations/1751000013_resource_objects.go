package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init creates the `resource_objects` collection: nabu's fast, durable index of
// the objects living in each lesson's S3 bucket. It is the source of truth for
// browsing/usage so the hot path never lists S3 (object storage is not a
// queryable index). nabu keeps it fresh by writing a row on every upload/import/
// delete it mediates (plus a client `confirm` for direct presigned PUTs the
// server never sees), and a periodic reconcile sweep corrects any drift.
//
// One row per (bucket, key). `scope` partitions a lesson's namespace:
//
//	resources -> {prefix}/resources/...        (class/staff)
//	group     -> {prefix}/group/{sXgY}/...     (a group's shared work)
//	student   -> {prefix}/student/{uid}/...    (a student's own work)
//
// Writes are server-only; only staff read the collection directly (student reads
// go through server endpoints that scope by the session record, mirroring
// captures/decisions).
func init() {
	m.Register(func(app core.App) error {
		lessons, err := app.FindCollectionByNameOrId("lessons")
		if err != nil {
			return fmt.Errorf("find lessons collection: %w", err)
		}

		objects := core.NewBaseCollection("resource_objects")
		objects.Fields.Add(
			&core.RelationField{Name: "lesson", Required: true, CollectionId: lessons.Id, MaxSelect: 1, CascadeDelete: true},
			&core.TextField{Name: "bucket", Required: true},
			&core.TextField{Name: "key", Required: true},
			&core.SelectField{Name: "scope", Required: true, MaxSelect: 1, Values: []string{"resources", "group", "student"}},
			&core.TextField{Name: "owner"}, // uid for student scope
			&core.TextField{Name: "group"}, // sXgY tag for group scope
			&core.NumberField{Name: "size", OnlyInt: true},
			&core.TextField{Name: "content_type"},
			&core.DateField{Name: "last_modified"},
			&core.TextField{Name: "public_url"},
			&core.AutodateField{Name: "created", OnCreate: true},
			&core.AutodateField{Name: "updated", OnCreate: true, OnUpdate: true},
		)
		objects.AddIndex("idx_resource_objects_bucket_key", true, "bucket, key", "")
		objects.AddIndex("idx_resource_objects_lesson_scope", false, "lesson, scope", "")

		// Staff read the index directly (dashboard/teacher views); student-scoped
		// reads are mediated by server endpoints. Writes are server-only.
		read := `@request.auth.role = "teacher" || @request.auth.role = "admin"`
		objects.ListRule = &read
		objects.ViewRule = &read

		if err := app.Save(objects); err != nil {
			return fmt.Errorf("save resource_objects collection: %w", err)
		}
		return nil
	}, func(app core.App) error {
		objects, err := app.FindCollectionByNameOrId("resource_objects")
		if err != nil {
			return nil // already gone
		}
		return app.Delete(objects)
	})
}

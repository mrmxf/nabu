package migrations

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

// init adds the S3 resource-store config to the lessons collection: `bucket` (the
// object-store bucket for this lesson's resources) and `prefix` (an optional
// lesson-root key prefix under which nabu composes role subpaths). Both are
// authored in a lesson's _index.md frontmatter (params.bucket / params.prefix).
// See internal/s3svc and docs/secrets.md.
func init() {
	m.Register(func(app core.App) error {
		lessons, err := app.FindCollectionByNameOrId("lessons")
		if err != nil {
			return fmt.Errorf("find lessons collection: %w", err)
		}
		lessons.Fields.Add(
			&core.TextField{Name: "bucket"},
			&core.TextField{Name: "prefix"},
		)
		if err := app.Save(lessons); err != nil {
			return fmt.Errorf("update lessons: %w", err)
		}
		return nil
	}, func(app core.App) error {
		lessons, err := app.FindCollectionByNameOrId("lessons")
		if err != nil {
			return fmt.Errorf("find lessons collection: %w", err)
		}
		lessons.Fields.RemoveByName("bucket")
		lessons.Fields.RemoveByName("prefix")
		if err := app.Save(lessons); err != nil {
			return fmt.Errorf("revert lessons: %w", err)
		}
		return nil
	})
}

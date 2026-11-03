// Package lmsfs is nabu's source of truth for lesson resources. It maintains a
// PocketBase index (the `resource_objects` collection) of the objects in each
// lesson's S3 bucket, so browsing and usage reads hit the database instead of
// listing S3 on every request (object storage is not a queryable index — that
// listing is the upload-page lag this package removes).
//
// Freshness model: nabu writes an index row on every write it mediates
// (import/multipart-complete/delete, plus a client `confirm` for direct
// presigned PUTs it never observes); a periodic Reconcile sweep corrects any
// drift. The slow bucket is therefore touched only to move bytes and during
// reconcile, never on the browse hot path.
//
// The index also underpins the storage-independent `lms://` reference scheme
// (see Resolve): a lesson's single prefix namespaces role subtrees —
// {prefix}/resources/… (course/staff), {prefix}/group/{sXgY}/… (group work),
// {prefix}/student/{uid}/… (a student's own work).
package lmsfs

import (
	"context"
	"fmt"
	"log/slog"
	"path"
	"strings"
	"time"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"

	"github.com/mrmxf/nabu/internal/s3svc"
)

// URIScheme is the storage-independent reference scheme for lesson resources:
// lms://<scope>/<path>. It lets lesson text point at a resource by role subtree
// (e.g. lms://resources/clip.mp4) without hard-coding the bucket/endpoint, so the
// storage backend can change without touching authored content.
const URIScheme = "lms://"

// ParseRef splits an lms:// reference into its scope and remaining path. ok is
// false if ref is not a well-formed lms:// URI with a known scope and non-empty
// path. Callers decide which scopes they can resolve (only resources is resolved
// statically at render time today).
func ParseRef(ref string) (scope Scope, relPath string, ok bool) {
	if !strings.HasPrefix(ref, URIScheme) {
		return "", "", false
	}
	seg, rel, found := strings.Cut(strings.TrimPrefix(ref, URIScheme), "/")
	if !found || rel == "" {
		return "", "", false
	}
	switch Scope(seg) {
	case ScopeResources, ScopeGroup, ScopeStudent:
		return Scope(seg), rel, true
	}
	return "", "", false
}

// KeyForRef composes the bucket key an lms:// reference resolves to under a
// lesson prefix: {prefix}/{scope}/{...}. relPath is the reference path after the
// scope (for group/student it begins with the tag/uid segment).
func KeyForRef(prefix string, scope Scope, relPath string) string {
	return cleanKey(prefix, string(scope), relPath)
}

// cleanKey joins path parts into a safe S3 key: no leading slash, no "..".
func cleanKey(parts ...string) string {
	return strings.TrimLeft(path.Clean("/"+path.Join(parts...)), "/")
}

// Scope partitions a lesson's key namespace. The value is stored verbatim in the
// `scope` field and is the segment right after the lesson prefix.
type Scope string

const (
	ScopeResources Scope = "resources" // {prefix}/resources/...   (course/staff)
	ScopeGroup     Scope = "group"     // {prefix}/group/{sXgY}/...  (group work)
	ScopeStudent   Scope = "student"   // {prefix}/student/{uid}/... (own work)

	collection = "resource_objects"
)

// reconcileTimeout bounds one lesson's bucket listing during a reconcile sweep.
const reconcileTimeout = 2 * time.Minute

// RecordedObject is the metadata nabu writes to the index when it mediates a
// write. Owner is set for student scope, Group for group scope.
type RecordedObject struct {
	LessonID     string
	Bucket       string
	Key          string
	Scope        Scope
	Owner        string
	Group        string
	Size         int64
	ContentType  string
	LastModified time.Time
	PublicURL    string
}

// Fs is the resource index. It is stateless beyond the shared S3 service and is
// safe for concurrent use; every method takes the PocketBase app explicitly.
type Fs struct {
	s3 *s3svc.Service
}

// New builds an Fs over the shared S3 service.
func New(s3 *s3svc.Service) *Fs { return &Fs{s3: s3} }

// List returns the indexed objects for a lesson under one scope, newest first.
// owner scopes to a single student (student scope); group scopes to one group
// tag (group scope); both empty means "all under this scope" (staff view).
func (f *Fs) List(app core.App, lessonID string, scope Scope, owner, group string) ([]s3svc.Object, error) {
	filter := "lesson = {:l} && scope = {:s}"
	params := dbx.Params{"l": lessonID, "s": string(scope)}
	if owner != "" {
		filter += " && owner = {:o}"
		params["o"] = owner
	}
	if group != "" {
		filter += " && group = {:g}"
		params["g"] = group
	}
	recs, err := app.FindRecordsByFilter(collection, filter, "-last_modified", 0, 0, params)
	if err != nil {
		return nil, fmt.Errorf("list resource index: %w", err)
	}
	out := make([]s3svc.Object, 0, len(recs))
	for _, r := range recs {
		out = append(out, rowToObject(r))
	}
	return out, nil
}

// Usage totals bytes + object count across every scope for a lesson.
func (f *Fs) Usage(app core.App, lessonID string) (s3svc.Usage, error) {
	recs, err := app.FindRecordsByFilter(collection, "lesson = {:l}", "", 0, 0, dbx.Params{"l": lessonID})
	if err != nil {
		return s3svc.Usage{}, fmt.Errorf("usage from index: %w", err)
	}
	var u s3svc.Usage
	for _, r := range recs {
		u.Bytes += int64(r.GetInt("size"))
		u.Count++
	}
	return u, nil
}

// Record upserts one index row (keyed by bucket+key). Called after any write nabu
// mediates. Empty ContentType/LastModified are not written, so a reconcile can't
// clobber a content-type captured at upload time.
func (f *Fs) Record(app core.App, o RecordedObject) error {
	rec, err := app.FindFirstRecordByFilter(collection, "bucket = {:b} && key = {:k}",
		dbx.Params{"b": o.Bucket, "k": o.Key})
	if err != nil {
		col, cerr := app.FindCollectionByNameOrId(collection)
		if cerr != nil {
			return fmt.Errorf("find %s collection: %w", collection, cerr)
		}
		rec = core.NewRecord(col)
		rec.Set("bucket", o.Bucket)
		rec.Set("key", o.Key)
	}
	rec.Set("lesson", o.LessonID)
	rec.Set("scope", string(o.Scope))
	rec.Set("owner", o.Owner)
	rec.Set("group", o.Group)
	rec.Set("size", o.Size)
	rec.Set("public_url", o.PublicURL)
	if o.ContentType != "" {
		rec.Set("content_type", o.ContentType)
	}
	if !o.LastModified.IsZero() {
		rec.Set("last_modified", o.LastModified)
	}
	if err := app.Save(rec); err != nil {
		return fmt.Errorf("save resource index row: %w", err)
	}
	return nil
}

// Rename repoints the index row from oldKey to newKey (updating the public URL),
// preserving size/content-type/scope/owner/group. Used after a bucket copy+delete
// rename. A missing source row is not an error — the next reconcile will index the
// new object.
func (f *Fs) Rename(app core.App, bucket, oldKey, newKey, newPublicURL string) error {
	rec, err := app.FindFirstRecordByFilter(collection, "bucket = {:b} && key = {:k}",
		dbx.Params{"b": bucket, "k": oldKey})
	if err != nil {
		return nil // not indexed; reconcile will pick up the new key
	}
	rec.Set("key", newKey)
	rec.Set("public_url", newPublicURL)
	if err := app.Save(rec); err != nil {
		return fmt.Errorf("rename resource index row: %w", err)
	}
	return nil
}

// Remove drops the index row for a deleted object. Missing is not an error.
func (f *Fs) Remove(app core.App, bucket, key string) error {
	rec, err := app.FindFirstRecordByFilter(collection, "bucket = {:b} && key = {:k}",
		dbx.Params{"b": bucket, "k": key})
	if err != nil {
		return nil // already absent
	}
	if err := app.Delete(rec); err != nil {
		return fmt.Errorf("delete resource index row: %w", err)
	}
	return nil
}

// Reconcile re-syncs the index against the live bucket for every lesson that has
// one: it lists each lesson's prefix, upserts a row per object, and deletes index
// rows whose object has vanished. This is the only routine S3 lister and the
// safety net for direct-PUT confirms that never arrived. Best-effort per lesson.
func (f *Fs) Reconcile(app core.App) error {
	if !f.s3.Configured() {
		return nil
	}
	lessons, err := app.FindRecordsByFilter("lessons", "bucket != ''", "", 0, 0)
	if err != nil {
		return fmt.Errorf("find lessons with buckets: %w", err)
	}
	for _, lesson := range lessons {
		if err := f.reconcileLesson(app, lesson); err != nil {
			// non-fatal: one unreachable/slow bucket shouldn't stop the rest.
			slog.Warn("resource reconcile: lesson failed",
				"lesson", lesson.Id, "bucket", lesson.GetString("bucket"), "err", err)
			continue
		}
	}
	return nil
}

func (f *Fs) reconcileLesson(app core.App, lesson *core.Record) error {
	bucket := lesson.GetString("bucket")
	prefix := lesson.GetString("prefix")
	ctx, cancel := context.WithTimeout(context.Background(), reconcileTimeout)
	defer cancel()

	objs, err := f.s3.List(ctx, bucket, prefix)
	if err != nil {
		return err
	}
	live := make(map[string]struct{}, len(objs))
	for _, o := range objs {
		live[o.Key] = struct{}{}
		scope, owner, group := classify(prefix, o.Key)
		if scope == "" {
			continue // key outside the known role subtrees; ignore
		}
		if err := f.Record(app, RecordedObject{
			LessonID:     lesson.Id,
			Bucket:       bucket,
			Key:          o.Key,
			Scope:        scope,
			Owner:        owner,
			Group:        group,
			Size:         o.Size,
			LastModified: o.LastModified,
			PublicURL:    o.PublicURL,
		}); err != nil {
			return err
		}
	}
	// Drop index rows whose object is gone.
	rows, err := app.FindRecordsByFilter(collection, "bucket = {:b}", "", 0, 0, dbx.Params{"b": bucket})
	if err != nil {
		return err
	}
	for _, r := range rows {
		if _, ok := live[r.GetString("key")]; !ok {
			if err := app.Delete(r); err != nil {
				return err
			}
		}
	}
	return nil
}

// rowToObject maps an index record to the wire shape the resource endpoints
// already return, so switching the read path to the index is transparent.
func rowToObject(r *core.Record) s3svc.Object {
	return s3svc.Object{
		Key:          r.GetString("key"),
		Size:         int64(r.GetInt("size")),
		LastModified: r.GetDateTime("last_modified").Time(),
		PublicURL:    r.GetString("public_url"),
	}
}

// classify derives the scope (and owner/group) of a key from the segment right
// after the lesson prefix. Returns an empty scope for keys outside the known
// role subtrees, which the caller skips.
func classify(prefix, key string) (scope Scope, owner, group string) {
	rel := key
	if p := strings.TrimSuffix(prefix, "/"); p != "" {
		rel = strings.TrimPrefix(key, p+"/")
	}
	parts := strings.SplitN(rel, "/", 3)
	switch parts[0] {
	case string(ScopeResources):
		return ScopeResources, "", ""
	case string(ScopeGroup):
		if len(parts) >= 2 && parts[1] != "" {
			return ScopeGroup, "", parts[1]
		}
	case string(ScopeStudent):
		if len(parts) >= 2 && parts[1] != "" {
			return ScopeStudent, parts[1], ""
		}
	}
	return "", "", ""
}

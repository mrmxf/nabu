package httpapi

import (
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/pocketbase/pocketbase/core"

	"github.com/mrmxf/nabu/internal/content"
	"github.com/mrmxf/nabu/internal/lmsfs"
	"github.com/mrmxf/nabu/internal/s3svc"
)

// withoutControlFiles drops objects whose key contains a control segment (a
// leading underscore anywhere in the path — see content.IsControlPath). Applied
// on the STUDENT-facing list routes only: staff routes show control files so a
// teacher can manage them, and the client styles them distinctly.
//
// This runs server-side, before serialisation, so the names never reach a
// student's browser at all — filtering in the component would still ship them in
// the response body.
func withoutControlFiles(objs []s3svc.Object) []s3svc.Object {
	out := make([]s3svc.Object, 0, len(objs))
	for _, o := range objs {
		if content.IsControlPath(o.Key) {
			continue
		}
		out = append(out, o)
	}
	return out
}

// s3client is the shared S3 service for the resource routes (constructed once so
// its usage cache is shared). Configured from ENV; see internal/s3svc.
var s3client = s3svc.New()

// lmsFs is the PocketBase-backed resource index: the source of truth for browse +
// usage reads so the hot path never lists S3. nabu updates it on every write it
// mediates; ReconcileResources/StartResourceSync keep it honest. See internal/lmsfs.
var lmsFs = lmsfs.New(s3client)

// resourceSyncInterval is how often the reconcile sweep re-syncs the index with
// the live buckets (the safety net for direct-PUT confirms that never arrive).
const resourceSyncInterval = 10 * time.Minute

// recordResource upserts a staff (course) resource into the index after a write.
func recordResource(app core.App, lessonID, bucket, key, contentType string, size int64) {
	recordScoped(app, lessonID, bucket, key, contentType, size, lmsfs.ScopeResources, "", "")
}

// recordScoped upserts any object (course/group/student) into the index after a
// write. Best-effort: a failure is logged and corrected by the next reconcile sweep.
func recordScoped(app core.App, lessonID, bucket, key, contentType string, size int64, scope lmsfs.Scope, owner, group string) {
	if err := lmsFs.Record(app, lmsfs.RecordedObject{
		LessonID:     lessonID,
		Bucket:       bucket,
		Key:          key,
		Scope:        scope,
		Owner:        owner,
		Group:        group,
		Size:         size,
		ContentType:  contentType,
		LastModified: time.Now(),
		PublicURL:    s3client.PublicURL(bucket, key),
	}); err != nil {
		slog.Warn("resource index update failed", "key", key, "err", err)
	}
}

// keyUnder cleans a client-supplied key and confirms it sits under base+"/".
// Returns ok=false otherwise, so scoped routes that take a key from the client
// can never touch an object outside the caller's own subtree.
func keyUnder(base, clientKey string) (string, bool) {
	key := composeKey(clientKey)
	if key == "" || !strings.HasPrefix(key, base+"/") {
		return "", false
	}
	return key, true
}

// renameObject renames an object in place: it changes only the final path segment
// (S3 has no rename, so this is copy + delete), keeping the object in the same
// directory — and therefore the same already-validated subtree. The index row is
// repointed, preserving its metadata. oldKey must already be validated by the
// caller (resolveResourceKey / keyUnder). Returns the new key.
func renameObject(e *core.RequestEvent, bucket, oldKey, newName string) (string, error) {
	if strings.TrimSpace(newName) == "" {
		return "", e.BadRequestError("new name required", nil)
	}
	newKey := composeKey(path.Dir(oldKey), s3svc.SafeFilename(newName))
	if newKey == oldKey {
		return oldKey, nil // no change
	}
	ctx := e.Request.Context()
	if err := s3client.Copy(ctx, bucket, oldKey, newKey); err != nil {
		return "", e.InternalServerError("rename failed", err)
	}
	if err := s3client.Delete(ctx, bucket, oldKey); err != nil {
		return "", e.InternalServerError("rename cleanup failed", err)
	}
	if err := lmsFs.Rename(e.App, bucket, oldKey, newKey, s3client.PublicURL(bucket, newKey)); err != nil {
		slog.Warn("resource index rename failed", "old", oldKey, "new", newKey, "err", err)
	}
	return newKey, nil
}

// ReconcileResources re-syncs the whole index against the live buckets. Call once
// on serve startup (drift from a previous run) and periodically via StartResourceSync.
func ReconcileResources(app core.App) {
	if err := lmsFs.Reconcile(app); err != nil {
		slog.Warn("resource index reconcile failed", "err", err)
	}
}

// StartResourceSync builds the index from the live buckets once, then re-syncs on
// a ticker. Both run in the background so a slow/unreachable S3 never blocks boot
// (the initial reconcile used to run synchronously and delayed startup). Call once
// on serve startup.
func StartResourceSync(app core.App) {
	go func() {
		ReconcileResources(app) // initial build
		t := time.NewTicker(resourceSyncInterval)
		defer t.Stop()
		for range t.C {
			ReconcileResources(app)
		}
	}()
}

// presignTTL bounds how long a browser has to complete a direct upload.
const presignTTL = 10 * time.Minute

// partTTL bounds how long a browser has to upload one multipart part. Each part
// gets its own window, so a whole-file deadline never applies — this only needs
// to cover a single part on a slow link.
const partTTL = 30 * time.Minute

// maxParts caps the S3 multipart part number (S3's own limit is 10000).
const maxParts = 10000

// resourcePresignHandler mints a presigned PUT URL for a teacher upload. The
// bucket + prefix come from the LESSON record (never the client); the key is
// server-derived: {prefix}/resources/{safe-filename}.
func resourcePresignHandler(e *core.RequestEvent) error {
	if !s3client.Configured() {
		return e.JSON(http.StatusServiceUnavailable, map[string]any{"error": "s3 not configured"})
	}
	var body struct {
		LessonID    string `json:"lesson_id"`
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	bucket, prefix, herr := resolveLessonBucket(e, body.LessonID)
	if herr != nil {
		return herr
	}
	key := composeKey(prefix, "resources", s3svc.SafeFilename(body.Filename))
	putURL, err := s3client.PresignPut(e.Request.Context(), bucket, key, body.ContentType, presignTTL)
	if err != nil {
		return e.InternalServerError("presign failed", err)
	}
	return e.JSON(http.StatusOK, map[string]any{
		"put_url":    putURL,
		"public_url": s3client.PublicURL(bucket, key),
		"key":        key,
	})
}

// resourceConfirmHandler indexes an object the browser uploaded directly via a
// presigned PUT — the one write nabu never observes. The client posts the
// server-issued key plus the size/content-type it just uploaded; the key is
// re-validated against the lesson before it is recorded. This replaces the old
// fresh=1 S3 re-scan, so a direct upload never triggers a bucket listing.
func resourceConfirmHandler(e *core.RequestEvent) error {
	var body struct {
		LessonID    string `json:"lesson_id"`
		Key         string `json:"key"`
		Size        int64  `json:"size"`
		ContentType string `json:"content_type"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	bucket, key, herr := resolveResourceKey(e, body.LessonID, body.Key)
	if herr != nil {
		return herr
	}
	recordResource(e.App, body.LessonID, bucket, key, body.ContentType, body.Size)
	return e.JSON(http.StatusOK, map[string]any{"key": key})
}

// resourceImportURLHandler fetches a URL server-side and stores it under the
// lesson's resources/ prefix, returning its public URL.
func resourceImportURLHandler(e *core.RequestEvent) error {
	if !s3client.Configured() {
		return e.JSON(http.StatusServiceUnavailable, map[string]any{"error": "s3 not configured"})
	}
	var body struct {
		LessonID string `json:"lesson_id"`
		URL      string `json:"url"`
		Filename string `json:"filename"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	if body.URL == "" {
		return e.BadRequestError("url required", nil)
	}
	bucket, prefix, herr := resolveLessonBucket(e, body.LessonID)
	if herr != nil {
		return herr
	}
	name := body.Filename
	if name == "" {
		name = filenameFromURL(body.URL)
	}
	key := composeKey(prefix, "resources", s3svc.SafeFilename(name))
	pub, size, contentType, err := s3client.PutFromURL(e.Request.Context(), bucket, key, body.URL)
	if err != nil {
		if errors.Is(err, s3svc.ErrImportTimeout) {
			return e.JSON(http.StatusGatewayTimeout, map[string]any{"message": "source fetch timed out"})
		}
		return e.InternalServerError("import failed", err)
	}
	recordResource(e.App, body.LessonID, bucket, key, contentType, size)
	return e.JSON(http.StatusOK, map[string]any{"public_url": pub, "key": key})
}

// resourceListHandler lists the lesson's resources. Teachers browse the whole
// lesson prefix; an optional ?sub= scopes deeper.
func resourceListHandler(e *core.RequestEvent) error {
	if !s3client.Configured() {
		return e.JSON(http.StatusServiceUnavailable, map[string]any{"error": "s3 not configured"})
	}
	q := e.Request.URL.Query()
	lessonID := q.Get("lesson_id")
	// Guard: confirms the lesson exists and has a bucket configured. The listing
	// itself comes from the index, not S3.
	if _, _, herr := resolveLessonBucket(e, lessonID); herr != nil {
		return herr
	}
	// Staff may browse any scope: course resources (default), a group's shared work
	// (optionally filtered to one ?group= tag), or a student's own work (?owner=).
	scope := lmsfs.ScopeResources
	switch q.Get("scope") {
	case "", "resources":
		scope = lmsfs.ScopeResources
	case "group":
		scope = lmsfs.ScopeGroup
	case "student":
		scope = lmsfs.ScopeStudent
	default:
		return e.BadRequestError("scope must be resources, group or student", nil)
	}
	objs, err := lmsFs.List(e.App, lessonID, scope, q.Get("owner"), q.Get("group"))
	if err != nil {
		return e.InternalServerError("list failed", err)
	}
	return e.JSON(http.StatusOK, map[string]any{"objects": objs})
}

// globalResourcesPrefix is the COURSE-GLOBAL resources folder, at the bucket root
// rather than under a lesson prefix. Teachers upload course-wide assets here
// (school logo, video clips, …); it is shared by every lesson in that bucket and
// is distinct from a lesson's own {prefix}/resources/ store.
const globalResourcesPrefix = "resources/"

// resourceGlobalListHandler lists the course-global resources folder for a lesson's
// bucket. It reads S3 directly (the folder is not lesson-scoped, so the per-lesson
// index doesn't cover it) and is exposed to ANY authenticated user: these are
// read-only, public course assets, and the lesson only supplies its bucket. Folder
// placeholder keys (ending in "/") are dropped.
func resourceGlobalListHandler(e *core.RequestEvent) error {
	if !s3client.Configured() {
		return e.JSON(http.StatusServiceUnavailable, map[string]any{"error": "s3 not configured"})
	}
	bucket, _, herr := resolveLessonBucket(e, e.Request.URL.Query().Get("lesson_id"))
	if herr != nil {
		return herr
	}
	all, err := s3client.List(e.Request.Context(), bucket, globalResourcesPrefix)
	if err != nil {
		return e.InternalServerError("list failed", err)
	}
	objs := make([]s3svc.Object, 0, len(all))
	for _, o := range all {
		if strings.HasSuffix(o.Key, "/") {
			continue // directory placeholder, not a file
		}
		objs = append(objs, o)
	}
	// This route is requireAuth, so students reach it too. Staff see control files
	// (and the client marks them as such); students never do.
	if !isAdmin(e) && !isTeacher(e) {
		objs = withoutControlFiles(objs)
	}
	return e.JSON(http.StatusOK, map[string]any{"objects": objs})
}

// resourceUsageHandler returns total bytes + object count for the lesson's bucket
// (the whole lesson prefix). Item 8's dashboard metric.
func resourceUsageHandler(e *core.RequestEvent) error {
	if !s3client.Configured() {
		return e.JSON(http.StatusServiceUnavailable, map[string]any{"error": "s3 not configured"})
	}
	lessonID := e.Request.URL.Query().Get("lesson_id")
	if _, _, herr := resolveLessonBucket(e, lessonID); herr != nil {
		return herr
	}
	// Usage is summed from the index, which nabu keeps current on every write, so
	// there is no cache to bypass — the old fresh=1 param is now a no-op.
	u, err := lmsFs.Usage(e.App, lessonID)
	if err != nil {
		return e.InternalServerError("usage failed", err)
	}
	return e.JSON(http.StatusOK, u)
}

// resourceDeleteHandler deletes one object from the lesson's resources area. The
// bucket and allowed prefix come from the LESSON record; the client supplies only
// the key, which is cleaned and must sit under {prefix}/resources/ — so a teacher
// can never delete anything outside their own lesson's uploaded files.
func resourceDeleteHandler(e *core.RequestEvent) error {
	if !s3client.Configured() {
		return e.JSON(http.StatusServiceUnavailable, map[string]any{"error": "s3 not configured"})
	}
	var body struct {
		LessonID string `json:"lesson_id"`
		Key      string `json:"key"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	bucket, key, herr := resolveResourceKey(e, body.LessonID, body.Key)
	if herr != nil {
		return herr
	}
	if err := s3client.Delete(e.Request.Context(), bucket, key); err != nil {
		return e.InternalServerError("delete failed", err)
	}
	if err := lmsFs.Remove(e.App, bucket, key); err != nil {
		slog.Warn("resource index remove failed", "key", key, "err", err)
	}
	return e.JSON(http.StatusOK, map[string]any{"deleted": key})
}

// resourceRenameHandler renames one course resource (copy + delete under the hood).
func resourceRenameHandler(e *core.RequestEvent) error {
	if !s3client.Configured() {
		return e.JSON(http.StatusServiceUnavailable, map[string]any{"error": "s3 not configured"})
	}
	var body struct {
		LessonID string `json:"lesson_id"`
		Key      string `json:"key"`
		NewName  string `json:"new_name"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	bucket, key, herr := resolveResourceKey(e, body.LessonID, body.Key)
	if herr != nil {
		return herr
	}
	newKey, herr := renameObject(e, bucket, key, body.NewName)
	if herr != nil {
		return herr
	}
	return e.JSON(http.StatusOK, map[string]any{"key": newKey, "public_url": s3client.PublicURL(bucket, newKey)})
}

// resourceMultipartCreateHandler starts a multipart upload for a large direct
// browser upload. Like presign, the key is server-derived from the lesson; the
// browser gets back the upload id + key to drive the part uploads.
func resourceMultipartCreateHandler(e *core.RequestEvent) error {
	if !s3client.Configured() {
		return e.JSON(http.StatusServiceUnavailable, map[string]any{"error": "s3 not configured"})
	}
	var body struct {
		LessonID    string `json:"lesson_id"`
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	bucket, prefix, herr := resolveLessonBucket(e, body.LessonID)
	if herr != nil {
		return herr
	}
	key := composeKey(prefix, "resources", s3svc.SafeFilename(body.Filename))
	uploadID, err := s3client.CreateMultipart(e.Request.Context(), bucket, key, body.ContentType)
	if err != nil {
		return e.InternalServerError("create multipart failed", err)
	}
	return e.JSON(http.StatusOK, map[string]any{
		"upload_id":  uploadID,
		"key":        key,
		"public_url": s3client.PublicURL(bucket, key),
	})
}

// resourceMultipartSignHandler mints a presigned PUT URL for one part. The key is
// re-validated against the lesson so a client can't sign parts for an arbitrary
// object, even though it originally came from create.
func resourceMultipartSignHandler(e *core.RequestEvent) error {
	if !s3client.Configured() {
		return e.JSON(http.StatusServiceUnavailable, map[string]any{"error": "s3 not configured"})
	}
	var body struct {
		LessonID   string `json:"lesson_id"`
		Key        string `json:"key"`
		UploadID   string `json:"upload_id"`
		PartNumber int32  `json:"part_number"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	if body.UploadID == "" {
		return e.BadRequestError("upload_id required", nil)
	}
	if body.PartNumber < 1 || body.PartNumber > maxParts {
		return e.BadRequestError("part_number out of range", nil)
	}
	bucket, key, herr := resolveResourceKey(e, body.LessonID, body.Key)
	if herr != nil {
		return herr
	}
	putURL, err := s3client.PresignUploadPart(e.Request.Context(), bucket, key, body.UploadID, body.PartNumber, partTTL)
	if err != nil {
		return e.InternalServerError("presign part failed", err)
	}
	return e.JSON(http.StatusOK, map[string]any{"put_url": putURL})
}

// resourceMultipartCompleteHandler finalises a multipart upload from the parts the
// browser uploaded.
func resourceMultipartCompleteHandler(e *core.RequestEvent) error {
	if !s3client.Configured() {
		return e.JSON(http.StatusServiceUnavailable, map[string]any{"error": "s3 not configured"})
	}
	var body struct {
		LessonID    string                `json:"lesson_id"`
		Key         string                `json:"key"`
		UploadID    string                `json:"upload_id"`
		Parts       []s3svc.CompletedPart `json:"parts"`
		Size        int64                 `json:"size"`         // total file size, from the client
		ContentType string                `json:"content_type"` // from the client
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	if body.UploadID == "" {
		return e.BadRequestError("upload_id required", nil)
	}
	if len(body.Parts) == 0 {
		return e.BadRequestError("parts required", nil)
	}
	bucket, key, herr := resolveResourceKey(e, body.LessonID, body.Key)
	if herr != nil {
		return herr
	}
	pub, err := s3client.CompleteMultipart(e.Request.Context(), bucket, key, body.UploadID, body.Parts)
	if err != nil {
		return e.InternalServerError("complete multipart failed", err)
	}
	recordResource(e.App, body.LessonID, bucket, key, body.ContentType, body.Size)
	return e.JSON(http.StatusOK, map[string]any{"public_url": pub, "key": key})
}

// resourceMultipartAbortHandler cancels an in-progress multipart upload (browser
// gave up or hit an error). Best-effort; a failure here just leaves parts for the
// bucket's own multipart-cleanup lifecycle rule to reap.
func resourceMultipartAbortHandler(e *core.RequestEvent) error {
	if !s3client.Configured() {
		return e.JSON(http.StatusServiceUnavailable, map[string]any{"error": "s3 not configured"})
	}
	var body struct {
		LessonID string `json:"lesson_id"`
		Key      string `json:"key"`
		UploadID string `json:"upload_id"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	if body.UploadID == "" {
		return e.BadRequestError("upload_id required", nil)
	}
	bucket, key, herr := resolveResourceKey(e, body.LessonID, body.Key)
	if herr != nil {
		return herr
	}
	if err := s3client.AbortMultipart(e.Request.Context(), bucket, key, body.UploadID); err != nil {
		return e.InternalServerError("abort multipart failed", err)
	}
	return e.JSON(http.StatusOK, map[string]any{"aborted": key})
}

// resolveLessonBucket loads the lesson and returns its (bucket, prefix). The
// bucket is always taken from the record, never from the client.
func resolveLessonBucket(e *core.RequestEvent, lessonID string) (string, string, error) {
	if lessonID == "" {
		return "", "", e.BadRequestError("lesson_id required", nil)
	}
	rec, err := e.App.FindRecordById("lessons", lessonID)
	if err != nil {
		return "", "", e.NotFoundError("lesson not found", err)
	}
	bucket := rec.GetString("bucket")
	if bucket == "" {
		return "", "", e.BadRequestError("lesson has no resource bucket configured", nil)
	}
	return bucket, rec.GetString("prefix"), nil
}

// resolveResourceKey resolves the lesson's bucket and validates that clientKey
// sits under {prefix}/resources/ — so delete/multipart routes, which take a key
// from the client, can only ever touch this lesson's own uploaded files. It
// returns the cleaned key. composeKey strips a leading slash and resolves any
// ".." so traversal can't escape the allowed prefix.
func resolveResourceKey(e *core.RequestEvent, lessonID, clientKey string) (string, string, error) {
	bucket, prefix, herr := resolveLessonBucket(e, lessonID)
	if herr != nil {
		return "", "", herr
	}
	key := composeKey(clientKey)
	allowed := composeKey(prefix, "resources") + "/"
	if key == "" || !strings.HasPrefix(key, allowed) {
		return "", "", e.BadRequestError("key is not within this lesson's resources", nil)
	}
	return bucket, key, nil
}

// composeKey joins path parts into a clean S3 key: no leading slash, no "..".
func composeKey(parts ...string) string {
	joined := path.Join(parts...)
	return strings.TrimLeft(path.Clean("/"+joined), "/")
}

// filenameFromURL extracts a base filename from a URL's path.
func filenameFromURL(raw string) string {
	u, err := url.Parse(raw)
	if err != nil {
		return "import"
	}
	base := path.Base(u.Path)
	if base == "" || base == "." || base == "/" {
		return "import"
	}
	return base
}

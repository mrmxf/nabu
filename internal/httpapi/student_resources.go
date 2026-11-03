package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"github.com/pocketbase/pocketbase/core"

	"github.com/mrmxf/nabu/internal/lmsfs"
	"github.com/mrmxf/nabu/internal/s3svc"
)

// Student-facing resource routes. A student uploads/browses their own work or
// their group's shared work; the bucket, lesson and group are derived from the
// caller's session record (never the client), mirroring captures. Keys
// land under the lesson prefix in role subtrees:
//
//	{prefix}/group/{sXgY}/...    scope=group    (shared with the group)
//	{prefix}/student/{uid}/...   scope=student  (the caller's own)

// ownedSession loads the session and verifies the caller owns it — the guard
// shared by every student route (a student may only touch their own session's
// resources), mirroring captures.
func ownedSession(e *core.RequestEvent, sessionID string) (*core.Record, error) {
	if e.Auth == nil {
		return nil, e.UnauthorizedError("authentication required", nil)
	}
	if sessionID == "" {
		return nil, e.BadRequestError("session_id required", nil)
	}
	sess, err := e.App.FindRecordById("sessions", sessionID)
	if err != nil {
		return nil, e.NotFoundError("session not found", err)
	}
	if sess.GetString("user") != e.Auth.Id {
		return nil, e.ForbiddenError("not your session", nil)
	}
	return sess, nil
}

// groupTagOf returns the session's normalized group tag, with a solo fallback so
// an ungrouped student still has a private group subtree.
func groupTagOf(sess *core.Record) string {
	tag := normalizeTag(sess.GetString("group_name"))
	if tag == "" {
		tag = "solo-" + sess.Id
	}
	return tag
}

// studentScope resolves the caller's session into the bucket, lesson id, the
// index scope, and the key base (allowed prefix) for a WRITABLE subtree. Only
// group and student scopes are writable by students; course resources are
// read-only (see studentListHandler), so scope="resources" is rejected here.
func studentScope(e *core.RequestEvent, sessionID, scope string) (bucket, lessonID, keyBase string, sc lmsfs.Scope, owner, group string, herr error) {
	sess, err := ownedSession(e, sessionID)
	if err != nil {
		return "", "", "", "", "", "", err
	}
	lessonID = sess.GetString("lesson")
	bkt, prefix, herr2 := resolveLessonBucket(e, lessonID)
	if herr2 != nil {
		return "", "", "", "", "", "", herr2
	}
	switch scope {
	case "group":
		tag := groupTagOf(sess)
		return bkt, lessonID, composeKey(prefix, string(lmsfs.ScopeGroup), tag), lmsfs.ScopeGroup, "", tag, nil
	case "student":
		uid := e.Auth.GetString("uid")
		if uid == "" {
			return "", "", "", "", "", "", e.BadRequestError("your account has no uid", nil)
		}
		return bkt, lessonID, composeKey(prefix, string(lmsfs.ScopeStudent), uid), lmsfs.ScopeStudent, uid, "", nil
	default:
		return "", "", "", "", "", "", e.BadRequestError("scope must be group or student", nil)
	}
}

// studentListHandler returns indexed objects the caller may read: course resources
// (read-only), their group's shared work, or their own work.
func studentListHandler(e *core.RequestEvent) error {
	q := e.Request.URL.Query()
	sess, err := ownedSession(e, q.Get("session_id"))
	if err != nil {
		return err
	}
	lessonID := sess.GetString("lesson")
	if _, _, herr := resolveLessonBucket(e, lessonID); herr != nil {
		return herr
	}
	var (
		sc           lmsfs.Scope
		owner, group string
	)
	switch q.Get("scope") {
	case "resources":
		sc = lmsfs.ScopeResources
	case "group":
		sc, group = lmsfs.ScopeGroup, groupTagOf(sess)
	case "student":
		uid := e.Auth.GetString("uid")
		if uid == "" {
			return e.BadRequestError("your account has no uid", nil)
		}
		sc, owner = lmsfs.ScopeStudent, uid
	default:
		return e.BadRequestError("scope must be resources, group or student", nil)
	}
	objs, err := lmsFs.List(e.App, lessonID, sc, owner, group)
	if err != nil {
		return e.InternalServerError("list failed", err)
	}
	// Control files are not enumerable by students. Filtering here — before the
	// list is serialised — is the point: hiding them client-side would still ship
	// the names in the response body. Staff use /api/resources, which does not
	// filter (see content.IsControlPath).
	return e.JSON(http.StatusOK, map[string]any{"objects": withoutControlFiles(objs)})
}

// studentPresignHandler mints a presigned PUT for a direct student upload. The key
// is server-composed under the caller's subtree; the browser confirms afterwards.
func studentPresignHandler(e *core.RequestEvent) error {
	if !s3client.Configured() {
		return e.JSON(http.StatusServiceUnavailable, map[string]any{"error": "s3 not configured"})
	}
	var body struct {
		SessionID   string `json:"session_id"`
		Scope       string `json:"scope"`
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	bucket, _, keyBase, _, _, _, herr := studentScope(e, body.SessionID, body.Scope)
	if herr != nil {
		return herr
	}
	key := composeKey(keyBase, s3svc.SafeFilename(body.Filename))
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

// studentConfirmHandler indexes an object the student uploaded via a direct PUT.
func studentConfirmHandler(e *core.RequestEvent) error {
	var body struct {
		SessionID   string `json:"session_id"`
		Scope       string `json:"scope"`
		Key         string `json:"key"`
		Size        int64  `json:"size"`
		ContentType string `json:"content_type"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	bucket, lessonID, keyBase, sc, owner, group, herr := studentScope(e, body.SessionID, body.Scope)
	if herr != nil {
		return herr
	}
	key, ok := keyUnder(keyBase, body.Key)
	if !ok {
		return e.BadRequestError("key is not within your resources", nil)
	}
	recordScoped(e.App, lessonID, bucket, key, body.ContentType, body.Size, sc, owner, group)
	return e.JSON(http.StatusOK, map[string]any{"key": key})
}

// studentImportURLHandler imports a URL server-side into the caller's subtree.
func studentImportURLHandler(e *core.RequestEvent) error {
	if !s3client.Configured() {
		return e.JSON(http.StatusServiceUnavailable, map[string]any{"error": "s3 not configured"})
	}
	var body struct {
		SessionID string `json:"session_id"`
		Scope     string `json:"scope"`
		URL       string `json:"url"`
		Filename  string `json:"filename"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	if body.URL == "" {
		return e.BadRequestError("url required", nil)
	}
	bucket, lessonID, keyBase, sc, owner, group, herr := studentScope(e, body.SessionID, body.Scope)
	if herr != nil {
		return herr
	}
	name := body.Filename
	if name == "" {
		name = filenameFromURL(body.URL)
	}
	key := composeKey(keyBase, s3svc.SafeFilename(name))
	pub, size, contentType, err := s3client.PutFromURL(e.Request.Context(), bucket, key, body.URL)
	if err != nil {
		if errors.Is(err, s3svc.ErrImportTimeout) {
			return e.JSON(http.StatusGatewayTimeout, map[string]any{"message": "source fetch timed out"})
		}
		return e.InternalServerError("import failed", err)
	}
	recordScoped(e.App, lessonID, bucket, key, contentType, size, sc, owner, group)
	return e.JSON(http.StatusOK, map[string]any{"public_url": pub, "key": key})
}

// studentRenameHandler renames one object in the caller's subtree.
func studentRenameHandler(e *core.RequestEvent) error {
	if !s3client.Configured() {
		return e.JSON(http.StatusServiceUnavailable, map[string]any{"error": "s3 not configured"})
	}
	var body struct {
		SessionID string `json:"session_id"`
		Scope     string `json:"scope"`
		Key       string `json:"key"`
		NewName   string `json:"new_name"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	bucket, _, keyBase, _, _, _, herr := studentScope(e, body.SessionID, body.Scope)
	if herr != nil {
		return herr
	}
	key, ok := keyUnder(keyBase, body.Key)
	if !ok {
		return e.BadRequestError("key is not within your resources", nil)
	}
	newKey, herr := renameObject(e, bucket, key, body.NewName)
	if herr != nil {
		return herr
	}
	return e.JSON(http.StatusOK, map[string]any{"key": newKey, "public_url": s3client.PublicURL(bucket, newKey)})
}

// studentDeleteHandler removes one object from the caller's subtree.
func studentDeleteHandler(e *core.RequestEvent) error {
	if !s3client.Configured() {
		return e.JSON(http.StatusServiceUnavailable, map[string]any{"error": "s3 not configured"})
	}
	var body struct {
		SessionID string `json:"session_id"`
		Scope     string `json:"scope"`
		Key       string `json:"key"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	bucket, _, keyBase, _, _, _, herr := studentScope(e, body.SessionID, body.Scope)
	if herr != nil {
		return herr
	}
	key, ok := keyUnder(keyBase, body.Key)
	if !ok {
		return e.BadRequestError("key is not within your resources", nil)
	}
	if err := s3client.Delete(e.Request.Context(), bucket, key); err != nil {
		return e.InternalServerError("delete failed", err)
	}
	if err := lmsFs.Remove(e.App, bucket, key); err != nil {
		return e.InternalServerError("index remove failed", err)
	}
	return e.JSON(http.StatusOK, map[string]any{"deleted": key})
}

// studentMultipartCreateHandler starts a multipart upload in the caller's subtree.
func studentMultipartCreateHandler(e *core.RequestEvent) error {
	if !s3client.Configured() {
		return e.JSON(http.StatusServiceUnavailable, map[string]any{"error": "s3 not configured"})
	}
	var body struct {
		SessionID   string `json:"session_id"`
		Scope       string `json:"scope"`
		Filename    string `json:"filename"`
		ContentType string `json:"content_type"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	bucket, _, keyBase, _, _, _, herr := studentScope(e, body.SessionID, body.Scope)
	if herr != nil {
		return herr
	}
	key := composeKey(keyBase, s3svc.SafeFilename(body.Filename))
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

// studentMultipartSignHandler signs one part for a multipart upload in the subtree.
func studentMultipartSignHandler(e *core.RequestEvent) error {
	if !s3client.Configured() {
		return e.JSON(http.StatusServiceUnavailable, map[string]any{"error": "s3 not configured"})
	}
	var body struct {
		SessionID  string `json:"session_id"`
		Scope      string `json:"scope"`
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
	bucket, _, keyBase, _, _, _, herr := studentScope(e, body.SessionID, body.Scope)
	if herr != nil {
		return herr
	}
	key, ok := keyUnder(keyBase, body.Key)
	if !ok {
		return e.BadRequestError("key is not within your resources", nil)
	}
	putURL, err := s3client.PresignUploadPart(e.Request.Context(), bucket, key, body.UploadID, body.PartNumber, partTTL)
	if err != nil {
		return e.InternalServerError("presign part failed", err)
	}
	return e.JSON(http.StatusOK, map[string]any{"put_url": putURL})
}

// studentMultipartCompleteHandler finalises a multipart upload in the subtree.
func studentMultipartCompleteHandler(e *core.RequestEvent) error {
	if !s3client.Configured() {
		return e.JSON(http.StatusServiceUnavailable, map[string]any{"error": "s3 not configured"})
	}
	var body struct {
		SessionID   string                `json:"session_id"`
		Scope       string                `json:"scope"`
		Key         string                `json:"key"`
		UploadID    string                `json:"upload_id"`
		Parts       []s3svc.CompletedPart `json:"parts"`
		Size        int64                 `json:"size"`
		ContentType string                `json:"content_type"`
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
	bucket, lessonID, keyBase, sc, owner, group, herr := studentScope(e, body.SessionID, body.Scope)
	if herr != nil {
		return herr
	}
	key, ok := keyUnder(keyBase, body.Key)
	if !ok {
		return e.BadRequestError("key is not within your resources", nil)
	}
	pub, err := s3client.CompleteMultipart(e.Request.Context(), bucket, key, body.UploadID, body.Parts)
	if err != nil {
		return e.InternalServerError("complete multipart failed", err)
	}
	recordScoped(e.App, lessonID, bucket, key, body.ContentType, body.Size, sc, owner, group)
	return e.JSON(http.StatusOK, map[string]any{"public_url": pub, "key": key})
}

// studentMultipartAbortHandler cancels a multipart upload in the subtree.
func studentMultipartAbortHandler(e *core.RequestEvent) error {
	if !s3client.Configured() {
		return e.JSON(http.StatusServiceUnavailable, map[string]any{"error": "s3 not configured"})
	}
	var body struct {
		SessionID string `json:"session_id"`
		Scope     string `json:"scope"`
		Key       string `json:"key"`
		UploadID  string `json:"upload_id"`
	}
	if err := json.NewDecoder(e.Request.Body).Decode(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	if body.UploadID == "" {
		return e.BadRequestError("upload_id required", nil)
	}
	bucket, _, keyBase, _, _, _, herr := studentScope(e, body.SessionID, body.Scope)
	if herr != nil {
		return herr
	}
	key, ok := keyUnder(keyBase, body.Key)
	if !ok {
		return e.BadRequestError("key is not within your resources", nil)
	}
	if err := s3client.AbortMultipart(e.Request.Context(), bucket, key, body.UploadID); err != nil {
		return e.InternalServerError("abort multipart failed", err)
	}
	return e.JSON(http.StatusOK, map[string]any{"aborted": key})
}

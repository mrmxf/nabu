package httpapi

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

// Room codes: a sign-in for a lesson in the room, halfway between an emailed
// code and the name-only classroom login. The teacher's page shows a short code
// for each course with an open Session; a student types their username (or
// email) and that code. The roster stays the allow-list:
//   - only an EXISTING account, role student, enrolled in the course, gets in;
//     nothing is created;
//   - only while one of the course's Sessions is enrollable;
//   - the code changes every roomCodeStep and the previous one is still
//     accepted, so a code works for 10 to 20 minutes;
//   - off unless the app_config `room_code_login` row is "true" (seeded from
//     nabu-config.yaml `room_code_login`);
//   - wrong codes are limited per account.
//
// Use it where emailed codes do not arrive (school mail filters) or would take
// too long. The tradeoff: anyone who can see the screen and knows a classmate's
// username can sign in as them, so show the code only in the room.
//
// The code is an HMAC of the course id and the time step, keyed by the users
// collection's token secret: nothing is stored, and rotating that secret
// changes every code.

const (
	roomCodeStep    = 10 * time.Minute
	roomCodeDigits  = 6
	roomCodeMaxFail = 5
	roomCodeFailWin = 5 * time.Minute
)

// roomCode is one course's current code, for the teacher's page.
type roomCode struct {
	CourseID string    `json:"course_id"`
	Course   string    `json:"course"`
	Sessions []string  `json:"sessions"`
	Code     string    `json:"code"`
	Expires  time.Time `json:"expires"`
}

// roomCodeLoginBody is POST /api/auth/room-code.
type roomCodeLoginBody struct {
	Identity string `json:"identity"`
	Code     string `json:"code"`
}

// roomCodeFor derives the code for a course at a time step.
func roomCodeFor(secret, courseID string, step int64) string {
	mac := hmac.New(sha256.New, []byte(secret))
	_, _ = fmt.Fprintf(mac, "nabu-room-code:%s:%d", courseID, step)
	n := binary.BigEndian.Uint32(mac.Sum(nil)[:4]) % 1_000_000
	return fmt.Sprintf("%0*d", roomCodeDigits, n)
}

func roomCodeStepAt(t time.Time) int64 { return t.Unix() / int64(roomCodeStep/time.Second) }

// roomCodeMatches accepts the current step's code or the previous one.
func roomCodeMatches(secret, courseID, code string, now time.Time) bool {
	step := roomCodeStepAt(now)
	for _, s := range []int64{step, step - 1} {
		if hmac.Equal([]byte(roomCodeFor(secret, courseID, s)), []byte(code)) {
			return true
		}
	}
	return false
}

// roomCodeEnabled reads the app_config `room_code_login` row; missing => off.
func roomCodeEnabled(app core.App) bool {
	rec, err := app.FindFirstRecordByData("app_config", "key", "room_code_login")
	return err == nil && rec != nil && rec.GetString("value") == "true"
}

// roomCodeSecret is the users collection's auth token secret.
func roomCodeSecret(app core.App) (string, error) {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return "", fmt.Errorf("find users collection: %w", err)
	}
	if users.AuthToken.Secret == "" {
		return "", fmt.Errorf("users collection has no token secret")
	}
	return users.AuthToken.Secret, nil
}

// openCourses groups the enrollable Sessions by course, sorted by course title.
func openCourses(app core.App) ([]roomCode, error) {
	sessions, err := enrollableSessions(app)
	if err != nil {
		return nil, err
	}
	byCourse := map[string]*roomCode{}
	for _, s := range sessions {
		if s.CourseID == "" {
			continue
		}
		rc := byCourse[s.CourseID]
		if rc == nil {
			rc = &roomCode{CourseID: s.CourseID, Course: s.Course}
			byCourse[s.CourseID] = rc
		}
		rc.Sessions = append(rc.Sessions, s.Lesson+" / "+s.Name)
	}
	out := make([]roomCode, 0, len(byCourse))
	for _, rc := range byCourse {
		out = append(out, *rc)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].Course != out[j].Course {
			return out[i].Course < out[j].Course
		}
		return out[i].CourseID < out[j].CourseID
	})
	return out, nil
}

// roomCodesHandler answers GET /api/room-codes (staff): the current code for
// each course with an open Session. Enabled=false means the switch is off.
func roomCodesHandler(e *core.RequestEvent) error {
	out := struct {
		Enabled bool       `json:"enabled"`
		Courses []roomCode `json:"courses"`
	}{Enabled: roomCodeEnabled(e.App), Courses: []roomCode{}}
	if !out.Enabled {
		return e.JSON(http.StatusOK, out)
	}
	secret, err := roomCodeSecret(e.App)
	if err != nil {
		return e.InternalServerError("room codes unavailable", err)
	}
	courses, err := openCourses(e.App)
	if err != nil {
		return e.InternalServerError("could not read the schedule", err)
	}
	now := time.Now()
	step := roomCodeStepAt(now)
	expires := time.Unix((step+1)*int64(roomCodeStep/time.Second), 0).UTC()
	for i := range courses {
		courses[i].Code = roomCodeFor(secret, courses[i].CourseID, step)
		courses[i].Expires = expires
	}
	out.Courses = courses
	return e.JSON(http.StatusOK, out)
}

// roomCodeStatusHandler answers GET /api/auth/room-code (public): whether the
// login page should offer the room-code form. It says nothing else.
func roomCodeStatusHandler(e *core.RequestEvent) error {
	return e.JSON(http.StatusOK, map[string]bool{"enabled": roomCodeEnabled(e.App)})
}

// roomCodeFailures limits wrong codes per typed identity (in memory: a restart
// clears it, which a restart's minute of downtime makes harmless).
var roomCodeFailures = struct {
	sync.Mutex
	at map[string][]time.Time
}{at: map[string][]time.Time{}}

// roomCodeBlocked reports whether key has used up its wrong guesses, pruning old ones.
func roomCodeBlocked(key string, now time.Time) bool {
	roomCodeFailures.Lock()
	defer roomCodeFailures.Unlock()
	kept := roomCodeFailures.at[key][:0]
	for _, t := range roomCodeFailures.at[key] {
		if now.Sub(t) < roomCodeFailWin {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(roomCodeFailures.at, key)
		return false
	}
	roomCodeFailures.at[key] = kept
	return len(kept) >= roomCodeMaxFail
}

// roomCodeFail records a wrong guess for key.
func roomCodeFail(key string, now time.Time) {
	roomCodeFailures.Lock()
	defer roomCodeFailures.Unlock()
	roomCodeFailures.at[key] = append(roomCodeFailures.at[key], now)
}

// roomCodeLoginHandler answers POST /api/auth/room-code: a username or email
// and the code on the classroom screen. Every refusal for a bad name or code
// reads the same, so the endpoint does not reveal who is on the roster.
func roomCodeLoginHandler(e *core.RequestEvent) error {
	if !roomCodeEnabled(e.App) {
		return e.ForbiddenError("room codes are not enabled", nil)
	}
	var body roomCodeLoginBody
	if err := e.BindBody(&body); err != nil {
		return e.BadRequestError("invalid body", err)
	}
	identity := strings.TrimSpace(body.Identity)
	code := strings.Join(strings.Fields(body.Code), "")
	if identity == "" || len(identity) > 255 || code == "" {
		return e.BadRequestError("enter your username and the code on the screen", nil)
	}
	const refused = "that name and code do not match an open lesson"

	users, err := e.App.FindCollectionByNameOrId("users")
	if err != nil {
		return e.InternalServerError("", err)
	}
	// Wrong guesses are counted per typed identity, known or not, so the limit
	// itself does not reveal who is on the roster.
	key := strings.ToLower(identity)
	now := time.Now()
	if roomCodeBlocked(key, now) {
		return e.TooManyRequestsError("too many tries: wait a few minutes", nil)
	}
	user := findByIdentity(e.App, users, identity)
	if user == nil {
		roomCodeFail(key, now)
		slog.Info("room code refused: no account", "identity_kind", identityKind(identity))
		return e.BadRequestError(refused, nil)
	}
	uid := user.GetString("uid")
	if user.GetString("role") != "student" {
		// staff sign in with an emailed code or a password, never a shared code.
		slog.Info("room code refused: not a student", "uid", uid)
		return e.BadRequestError(refused, nil)
	}
	secret, err := roomCodeSecret(e.App)
	if err != nil {
		return e.InternalServerError("room codes unavailable", err)
	}
	courses, err := openCourses(e.App)
	if err != nil {
		return e.InternalServerError("could not read the schedule", err)
	}
	for _, c := range courses {
		if !roomCodeMatches(secret, c.CourseID, code, now) {
			continue
		}
		en, _ := e.App.FindFirstRecordByFilter("enrolments", "user = {:u} && course = {:c}",
			map[string]any{"u": user.Id, "c": c.CourseID})
		if en == nil {
			break // a right code for a course this student is not on
		}
		slog.Info("room code sign-in", "uid", uid, "course", c.CourseID)
		return apis.RecordAuthResponse(e, user, "password", nil)
	}
	roomCodeFail(key, now)
	slog.Info("room code refused: wrong code or not enrolled", "uid", uid)
	return e.BadRequestError(refused, nil)
}

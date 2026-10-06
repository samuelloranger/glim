package serve

import (
	"crypto/subtle"
	"fmt"
	"html"
	"net/http"
	"strings"

	"github.com/samuelloranger/glim/internal/auth"
	"github.com/samuelloranger/glim/internal/store"
)

// UnlockCookiePrefix names the per-preview cookie that proves a visitor knows
// the preview's password: glim_unlock_<slug>.
const UnlockCookiePrefix = "glim_unlock_"

// maxUnlockForm caps the unlock form body read from a visitor.
const maxUnlockForm = 4 << 10

// Unlock configures password-protected previews.
type Unlock struct {
	// Token returns the cookie value for a slug and its password hash (see
	// auth.DB.UnlockToken). An empty result never matches, so a nil Unlock or a
	// failing secret keeps locked previews locked.
	Token func(slug, passwordHash string) string
	// Limiter throttles password attempts per client address and slug.
	Limiter *auth.Limiter
	// Secure marks the unlock cookie Secure and SameSite=None, which browsers
	// need before they send it with a sandboxed (opaque-origin) preview's
	// sub-resource requests. Over plain http it is SameSite=Lax, so a locked
	// directory preview's sub-resources are not unlocked there.
	Secure bool
}

func (u *Unlock) token(slug, hash string) string {
	if u == nil || u.Token == nil {
		return ""
	}
	return u.Token(slug, hash)
}

func (u *Unlock) valid(r *http.Request, slug, hash string) bool {
	want := u.token(slug, hash)
	if want == "" {
		return false
	}
	c, err := r.Cookie(UnlockCookiePrefix + slug)
	if err != nil {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(c.Value), []byte(want)) == 1
}

// isDocumentPath reports whether a request path is an HTML page, as opposed to
// an asset.
func isDocumentPath(p string) bool {
	return strings.HasSuffix(p, "/") || strings.HasSuffix(strings.ToLower(p), ".html")
}

// guardLocked enforces a preview's password. It returns true when the request
// may proceed to the preview's files; otherwise it has written the response.
func guardLocked(w http.ResponseWriter, r *http.Request, slug string, m store.Manifest, views *Views, u *Unlock, base string) bool {
	if views != nil && views.IsOwner != nil {
		if c, err := r.Cookie(auth.OwnerCookie); err == nil && views.IsOwner(c.Value) {
			return true
		}
	}
	if u.valid(r, slug, m.PasswordHash) {
		return true
	}
	w.Header().Set("Cache-Control", "no-store")
	if r.Method == http.MethodPost {
		attemptUnlock(w, r, slug, m, u, base)
		return false
	}
	if !isDocumentPath(r.URL.Path) {
		http.Error(w, "password required", http.StatusUnauthorized)
		return false
	}
	writeLockPage(w, r, base, http.StatusUnauthorized, "")
	return false
}

func attemptUnlock(w http.ResponseWriter, r *http.Request, slug string, m store.Manifest, u *Unlock, base string) {
	if u == nil || u.Token == nil || u.Limiter == nil {
		writeLockPage(w, r, base, http.StatusServiceUnavailable, "Unlocking is not available on this server.")
		return
	}
	ip := auth.ClientIP(r)
	key := ip + "\x00" + slug
	if wait := u.Limiter.Check(key, ip); wait > 0 {
		secs := int((wait + 999_999_999) / 1_000_000_000)
		w.Header().Set("Retry-After", fmt.Sprint(secs))
		writeLockPage(w, r, base, http.StatusTooManyRequests, fmt.Sprintf("Too many attempts. Try again in %d seconds.", secs))
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxUnlockForm)
	if err := r.ParseForm(); err != nil || !auth.CheckPassword(m.PasswordHash, r.PostForm.Get("password")) {
		u.Limiter.Fail(key, ip)
		writeLockPage(w, r, base, http.StatusUnauthorized, "Wrong password.")
		return
	}
	u.Limiter.Succeed(key)
	c := &http.Cookie{
		Name: UnlockCookiePrefix + slug, Value: u.token(slug, m.PasswordHash),
		Path: "/" + slug + "/", HttpOnly: true,
	}
	if !m.Pinned {
		c.Expires = m.Expires
	}
	if u.Secure {
		c.Secure, c.SameSite = true, http.SameSiteNoneMode
	} else {
		c.SameSite = http.SameSiteLaxMode
	}
	http.SetCookie(w, c)
	http.Redirect(w, r, r.URL.RequestURI(), http.StatusSeeOther)
}

const lockedCardText = "Password-protected preview"

// lockCardTags is the link card for a locked preview: it reveals nothing about
// the preview, not even its title or project.
func lockCardTags(imageURL string) string {
	tags := [][2]string{
		{"og:title", lockedCardText},
		{"og:description", lockedCardText},
		{"og:site_name", "glim"},
		{"og:type", "website"},
		{"og:image", imageURL},
		{"twitter:card", "summary"},
	}
	var sb strings.Builder
	for _, t := range tags {
		attr := "property"
		if strings.HasPrefix(t[0], "twitter:") {
			attr = "name"
		}
		sb.WriteString("<meta " + attr + `="` + t[0] + `" content="` + html.EscapeString(t[1]) + `">` + "\n")
	}
	return sb.String()
}

const lockPage = `<!doctype html>
<html lang="en"><head><meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>Password-protected preview</title>
<style>
:root{color-scheme:dark}
*{box-sizing:border-box}
body{margin:0;min-height:100vh;display:grid;place-items:center;padding:24px;background:radial-gradient(900px 500px at 50% 30%,#1a2a3d,#0e1224);color:#e8edf7;font:16px/1.5 system-ui,-apple-system,Segoe UI,sans-serif}
main{width:100%;max-width:360px;padding:28px;border-radius:14px;background:#141a2e;border:1px solid #26304d}
h1{margin:0 0 4px;font-size:1.15rem}
p{margin:0 0 18px;color:#9aa6c2;font-size:.9rem}
input{width:100%;padding:10px 12px;border-radius:8px;border:1px solid #33405f;background:#0e1224;color:inherit;font:inherit}
button{width:100%;margin-top:12px;padding:10px 12px;border:0;border-radius:8px;background:#78e0c8;color:#06201a;font:inherit;font-weight:600;cursor:pointer}
.err{margin:12px 0 0;color:#ff9a9a}
</style></head>
<body><main>
<h1>Password-protected preview</h1>
<p>Enter the password to view this page.</p>
<form method="post" action="{{ACTION}}">
<input type="password" name="password" autocomplete="current-password" autofocus required aria-label="Password">
<button type="submit">Unlock</button>
</form>{{ERR}}
</main></body></html>
`

func writeLockPage(w http.ResponseWriter, r *http.Request, base string, status int, msg string) {
	errHTML := ""
	if msg != "" {
		errHTML = `<p class="err" role="alert">` + html.EscapeString(msg) + `</p>`
	}
	page := strings.NewReplacer("{{ACTION}}", html.EscapeString(r.URL.RequestURI()), "{{ERR}}", errHTML).Replace(lockPage)
	doc := injectHTML([]byte(page), lockCardTags(base+ogImagePath), "")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.WriteHeader(status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(doc)
	}
}

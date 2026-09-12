#!/usr/bin/env python3
from pathlib import Path

# The main patch generates this test. Keep the assertion inside the server
# package instead of reaching into auth.LoginLimiter's private fields.
p = Path("internal/server/security_test.go")
text = p.read_text(encoding="utf-8")
old = '''func TestLoginRejectsOversizedUsernameWithoutLargeLimiterKey(t *testing.T) {
\ts, h := newSecurityTestServer(t)
\tbody, _ := json.Marshal(map[string]string{"username":strings.Repeat("x", 5000),"password":"bad"})
\treq := httptest.NewRequest(http.MethodPost, "http://panel/api/login", bytes.NewReader(body)); req.RemoteAddr="203.0.113.9:1234"
\trec := httptest.NewRecorder(); h.ServeHTTP(rec, req)
\tif rec.Code != http.StatusUnauthorized { t.Fatalf("got %d", rec.Code) }
\ts.loginLimiter.mu.Lock()
\tdefer s.loginLimiter.mu.Unlock()
\tfor key := range s.loginLimiter.entries { if len(key) > 96 { t.Fatalf("unexpected long limiter key: %d", len(key)) } }
}
'''
new = '''func TestLoginRejectsOversizedUsernameWithoutLargeLimiterKey(t *testing.T) {
\t_, h := newSecurityTestServer(t)
\tusername := strings.Repeat("x", 5000)
\tbody, _ := json.Marshal(map[string]string{"username":username,"password":"bad"})
\treq := httptest.NewRequest(http.MethodPost, "http://panel/api/login", bytes.NewReader(body)); req.RemoteAddr="203.0.113.9:1234"
\trec := httptest.NewRecorder(); h.ServeHTTP(rec, req)
\tif rec.Code != http.StatusUnauthorized { t.Fatalf("got %d", rec.Code) }
\tif key := loginRateKey("203.0.113.9", username); len(key) > 96 { t.Fatalf("unexpected long limiter key: %d", len(key)) }
}
'''
if old not in text:
    raise SystemExit("generated security test block not found")
p.write_text(text.replace(old, new, 1), encoding="utf-8")

# Reject a second JSON value rather than using Decoder.More(), which is only
# meaningful while traversing an array/object.
p = Path("internal/server/server.go")
text = p.read_text(encoding="utf-8")
old = 'func decodeJSONLimit(w http.ResponseWriter,r *http.Request,v any,limit int64)error{d:=json.NewDecoder(http.MaxBytesReader(w,r.Body,limit));d.DisallowUnknownFields();if e:=d.Decode(v);e!=nil{writeError(w,400,"invalid request");return e};if d.More(){writeError(w,400,"invalid request");return &json.SyntaxError{Offset:0}};return nil}'
new = 'func decodeJSONLimit(w http.ResponseWriter,r *http.Request,v any,limit int64)error{d:=json.NewDecoder(http.MaxBytesReader(w,r.Body,limit));d.DisallowUnknownFields();if e:=d.Decode(v);e!=nil{writeError(w,400,"invalid request");return e};var extra any;if e:=d.Decode(&extra);e!=io.EOF{writeError(w,400,"invalid request");if e==nil{return errors.New("multiple JSON values are not allowed")};return e};return nil}'
if old not in text:
    raise SystemExit("decodeJSONLimit block not found")
text = text.replace(old, new, 1)
# Add imports needed by the stricter trailing-value check.
text = text.replace('"encoding/json"\n\t"io/fs"', '"encoding/json"\n\t"errors"\n\t"io"\n\t"io/fs"', 1)
p.write_text(text, encoding="utf-8")

print("security hardening post-patch applied")

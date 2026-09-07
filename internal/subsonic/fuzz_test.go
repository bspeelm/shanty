package subsonic

import "testing"

// The decode boundary is where every byte the server chose first becomes a Go
// value. It may return an error for anything; it may not panic, hang, or
// return a response the caller would act on when the server said no.
func FuzzDecode(f *testing.F) {
	f.Add([]byte(`{"subsonic-response":{"status":"ok","version":"1.16.1"}}`))
	f.Add([]byte(`{"subsonic-response":{"status":"failed","error":{"code":40,"message":"no"}}}`))
	f.Add([]byte(`{"subsonic-response":{"status":"ok","artists":{"index":[{"name":"A","artist":[{"id":"1"}]}]}}}`))
	f.Add([]byte(`{"subsonic-response":{"status":"ok","album":{"id":"1","song":[{"id":"2","track":1}]}}}`))
	f.Add([]byte(`{"subsonic-response":{}}`))
	f.Add([]byte(`{}`))
	f.Add([]byte(``))

	f.Fuzz(func(t *testing.T, body []byte) {
		res, err := decode(body)
		if err != nil {
			if res != nil {
				t.Errorf("decode returned both a response and an error for %q", body)
			}
			return
		}
		if res == nil {
			t.Fatalf("decode returned no response and no error for %q", body)
		}
		// A caller only ever reaches a payload after decode said ok, so an
		// error slipping through as success is the one outcome that matters.
		if res.Status != "ok" {
			t.Errorf("decode accepted status %q for %q", res.Status, body)
		}
	})
}

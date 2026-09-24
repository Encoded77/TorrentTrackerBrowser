package core

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ntfy topic URLs take the message as the body and the metadata as headers;
// a JSON body would be displayed verbatim on the phone.
func TestWebhookUsesNtfyTopicConvention(t *testing.T) {
	var gotTitle, gotPrio, gotTags, gotBody, gotQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		gotBody, gotQuery = string(b), r.URL.RawQuery
		gotTitle, gotPrio, gotTags = r.Header.Get("Title"), r.Header.Get("Priority"), r.Header.Get("Tags")
	}))
	defer srv.Close()

	n := NewWebhook(srv.URL + "/homelab?auth=abc")
	n.Notify(context.Background(), "Download failed", "Dune: boom", true)
	if gotBody != "Dune: boom" || gotTitle != "Download failed" || gotPrio != "4" || gotTags != "torrent" || gotQuery != "auth=abc" {
		t.Errorf("request = body %q title %q prio %q tags %q query %q", gotBody, gotTitle, gotPrio, gotTags, gotQuery)
	}
	n.Notify(context.Background(), "Download finished", "x", false)
	if gotPrio != "3" {
		t.Errorf("success priority = %q", gotPrio)
	}
	NewWebhook("").Notify(context.Background(), "t", "m", false) // no-op, must not panic
}

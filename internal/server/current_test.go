package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestCurrentPagesRejectRemovedRevisionSelectors(t *testing.T) {
	for _, path := range []string{"/technical/erd?revision=r1", "/technical/component/store?revision=r1", "/technical/whiteboards/architecture?revision=r1", "/prototypes/demo?revision=r1"} {
		response := httptest.NewRecorder()
		(&app{}).page(response, httptest.NewRequest(http.MethodGet, path, nil))
		if response.Code != http.StatusGone || !strings.Contains(response.Body.String(), "Git commit") {
			t.Fatalf("historical URL silently substituted current content: %s: %d %s", path, response.Code, response.Body.String())
		}
	}
}

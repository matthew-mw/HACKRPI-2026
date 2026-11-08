package httpserver

import (
	"net/http/httptest"
	"strings"
	"testing"
)

func TestOrganizationLinks(t *testing.T) {
	for _, value := range []string{"", "https://example.org", "https://example.org/photo.jpg?size=large"} {
		if !validOrganizationURL(value) {
			t.Errorf("rejected %q", value)
		}
	}
	for _, value := range []string{"javascript:alert(1)", "data:image/svg+xml,test", "http://example.org", "https:///photo.jpg", "https://user:password@example.org", "//example.org"} {
		if validOrganizationURL(value) {
			t.Errorf("accepted %q", value)
		}
	}
}

func TestCharityRegistrationRoute(t *testing.T) {
	for _, tc := range []struct{ path, kind string }{{"/api/organization", "charity"}, {"/api/charity", "volunteer_group"}, {"/api/charity", "company"}} {
		request := httptest.NewRequest("POST", tc.path, strings.NewReader(`{"name":"Example","organization_type":"`+tc.kind+`","visibility":"public"}`))
		request.Header.Set("Content-Type", "application/json")
		_, err := (product{}).organization(httptest.NewRecorder(), request, "user")
		pe, ok := err.(*productError)
		if !ok || pe.status != 400 {
			t.Fatalf("%s accepted %s: %v", tc.path, tc.kind, err)
		}
	}
}

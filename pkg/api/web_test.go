package api_test

import (
	"net/http"
	"net/http/httptest"
	"strings"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/api"
	"github.com/gnomatix/enkente/pkg/parser"
)

var _ = Describe("Web UI", func() {
	var h http.Handler

	BeforeEach(func() {
		// No store: the page must still be served when persistence is off.
		h = api.NewServer(0, 1, func(int, parser.AntigravityMessage) {}).Handler()
	})

	expectPage := func(path string) {
		rr := get(h, path)
		Expect(rr.Code).To(Equal(http.StatusOK), path)
		Expect(rr.Header().Get("Content-Type")).To(HavePrefix("text/html"), path)
		Expect(rr.Body.String()).To(ContainSubstring("<html"), path)
	}

	It("serves the viewer at /", func() {
		expectPage("/")
	})

	It("serves the viewer at /ui and /ui/", func() {
		expectPage("/ui")
		expectPage("/ui/")
	})

	It("embeds a self-contained page with no external resources", func() {
		body := get(h, "/").Body.String()
		Expect(body).To(ContainSubstring("<script>"))
		Expect(body).To(ContainSubstring("<style>"))
		Expect(body).NotTo(MatchRegexp(`<script[^>]*\ssrc=`))
		Expect(body).NotTo(ContainSubstring(`<link `))
		Expect(body).NotTo(ContainSubstring("http://"))
		Expect(body).NotTo(ContainSubstring("https://"))
		Expect(body).To(ContainSubstring("prefers-color-scheme"))
		// Message text is rendered with textContent, never innerHTML.
		Expect(body).NotTo(ContainSubstring("innerHTML"))
	})

	It("returns 404 for unknown paths", func() {
		Expect(get(h, "/nope").Code).To(Equal(http.StatusNotFound))
		Expect(get(h, "/ui/nope").Code).To(Equal(http.StatusNotFound))
		Expect(get(h, "/static/index.html").Code).To(Equal(http.StatusNotFound))
	})

	It("rejects non-GET methods on the page routes", func() {
		rr := post(h, "/", `{}`)
		Expect(rr.Code).To(Equal(http.StatusMethodNotAllowed))
	})

	It("leaves the JSON endpoints unaffected", func() {
		rr := get(h, "/health")
		Expect(rr.Code).To(Equal(http.StatusOK))
		var body map[string]any
		decodeInto(rr, &body)
		Expect(body["status"]).To(Equal("ok"))

		// Without a store, /sessions answers 503 as JSON; the page's status
		// line surfaces this error text.
		rr = get(h, "/sessions")
		Expect(rr.Code).To(Equal(http.StatusServiceUnavailable))
		Expect(rr.Header().Get("Content-Type")).To(Equal("application/json"))
		Expect(strings.TrimSpace(rr.Body.String())).To(ContainSubstring("persistence is disabled"))
	})

	It("answers HEAD on the page routes", func() {
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, httptest.NewRequest(http.MethodHead, "/", nil))
		Expect(rr.Code).To(Equal(http.StatusOK))
		Expect(rr.Header().Get("Content-Type")).To(HavePrefix("text/html"))
	})
})

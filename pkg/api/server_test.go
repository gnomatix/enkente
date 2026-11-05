package api_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/api"
	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/storage"
)

// recorder collects every message dispatched to the worker pool.
type recorder struct {
	mu   sync.Mutex
	msgs []parser.AntigravityMessage
}

func (r *recorder) handle(_ int, msg parser.AntigravityMessage) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.msgs = append(r.msgs, msg)
}

func (r *recorder) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return len(r.msgs)
}

func (r *recorder) last() parser.AntigravityMessage {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.msgs[len(r.msgs)-1]
}

func post(h http.Handler, path string, body string) *httptest.ResponseRecorder {
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, req)
	return rr
}

func get(h http.Handler, path string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, path, nil))
	return rr
}

func decodeInto(rr *httptest.ResponseRecorder, v any) {
	Expect(rr.Header().Get("Content-Type")).To(Equal("application/json"))
	Expect(json.Unmarshal(rr.Body.Bytes(), v)).To(Succeed())
}

var _ = Describe("Ingestion API", func() {
	var rec *recorder

	BeforeEach(func() {
		rec = &recorder{}
	})

	Context("without a store", func() {
		var h http.Handler

		BeforeEach(func() {
			h = api.NewServer(0, 2, rec.handle).Handler()
		})

		It("reports health and that persistence is off", func() {
			rr := get(h, "/health")
			Expect(rr.Code).To(Equal(http.StatusOK))
			var body map[string]any
			decodeInto(rr, &body)
			Expect(body["status"]).To(Equal("ok"))
			Expect(body["persistence"]).To(BeFalse())
		})

		It("accepts a message with 202 and dispatches it to a worker", func() {
			rr := post(h, "/ingest", `{"type":"user","user":"brett","message":"Hello!"}`)
			Expect(rr.Code).To(Equal(http.StatusAccepted))

			var body struct {
				Status  string                    `json:"status"`
				Message parser.AntigravityMessage `json:"message"`
			}
			decodeInto(rr, &body)
			Expect(body.Status).To(Equal("accepted"))
			Expect(body.Message.SessionID).To(Equal(api.DefaultSessionID))
			Expect(body.Message.MessageID).To(Equal(0))
			Expect(body.Message.User).To(Equal("brett"))
			Expect(body.Message.Timestamp).NotTo(BeZero())

			Eventually(rec.count).Should(Equal(1))
			Expect(rec.last().Message).To(Equal("Hello!"))
		})

		It("rejects non-POST methods on /ingest", func() {
			rr := get(h, "/ingest")
			Expect(rr.Code).To(Equal(http.StatusMethodNotAllowed))
		})

		It("rejects malformed JSON", func() {
			rr := post(h, "/ingest", `{"type":`)
			Expect(rr.Code).To(Equal(http.StatusBadRequest))
			Consistently(rec.count, "100ms").Should(Equal(0))
		})

		It("rejects an empty or whitespace-only message", func() {
			Expect(post(h, "/ingest", `{"type":"user","message":""}`).Code).To(Equal(http.StatusBadRequest))
			Expect(post(h, "/ingest", `{"type":"user","message":"   \n"}`).Code).To(Equal(http.StatusBadRequest))
			Expect(post(h, "/ingest", `{"type":"user"}`).Code).To(Equal(http.StatusBadRequest))
			Consistently(rec.count, "100ms").Should(Equal(0))
		})

		It("rejects a body larger than the ingest cap with 413", func() {
			big := `{"type":"user","message":"` + strings.Repeat("x", api.MaxIngestBodyBytes) + `"}`
			rr := post(h, "/ingest", big)
			Expect(rr.Code).To(Equal(http.StatusRequestEntityTooLarge))
			Consistently(rec.count, "100ms").Should(Equal(0))
		})

		It("rejects an unparseable timestamp", func() {
			rr := post(h, "/ingest", `{"type":"user","message":"x","timestamp":"yesterday"}`)
			Expect(rr.Code).To(Equal(http.StatusBadRequest))
		})

		It("honours a client-supplied RFC 3339 timestamp, including the .NET round-trip format", func() {
			rr := post(h, "/ingest", `{"type":"user","message":"x","timestamp":"2026-03-18T05:15:06.1234567-04:00"}`)
			Expect(rr.Code).To(Equal(http.StatusAccepted))

			var body struct {
				Message parser.AntigravityMessage `json:"message"`
			}
			decodeInto(rr, &body)
			want := time.Date(2026, 3, 18, 9, 15, 6, 123456700, time.UTC)
			Expect(body.Message.Timestamp.Equal(want)).To(BeTrue())
		})

		It("defaults a blank type to user", func() {
			rr := post(h, "/ingest", `{"message":"x"}`)
			Expect(rr.Code).To(Equal(http.StatusAccepted))
			Eventually(rec.count).Should(Equal(1))
			Expect(rec.last().Type).To(Equal("user"))
		})

		It("answers read endpoints with 503 because nothing is persisted", func() {
			Expect(get(h, "/sessions").Code).To(Equal(http.StatusServiceUnavailable))
			Expect(get(h, "/sessions/live/messages").Code).To(Equal(http.StatusServiceUnavailable))
			Expect(get(h, "/sessions/live/messages/1").Code).To(Equal(http.StatusServiceUnavailable))
		})
	})

	Context("with a BoltDB store", func() {
		var (
			h     http.Handler
			store *storage.BoltStorage
		)

		BeforeEach(func() {
			var err error
			store, err = storage.NewBoltStorage(filepath.Join(GinkgoT().TempDir(), "api.db"))
			Expect(err).NotTo(HaveOccurred())
			h = api.NewServer(0, 2, rec.handle, api.WithStore(store)).Handler()
		})

		AfterEach(func() {
			Expect(store.Close()).To(Succeed())
		})

		It("reports that persistence is on", func() {
			var body map[string]any
			decodeInto(get(h, "/health"), &body)
			Expect(body["persistence"]).To(BeTrue())
		})

		It("persists an ingested message before dispatch, assigning a session and message ID", func() {
			rr := post(h, "/ingest", `{"type":"user","user":"brett","message":"persist me"}`)
			Expect(rr.Code).To(Equal(http.StatusCreated))

			var body struct {
				Message parser.AntigravityMessage `json:"message"`
			}
			decodeInto(rr, &body)
			Expect(body.Message.SessionID).To(Equal(api.DefaultSessionID))
			Expect(body.Message.MessageID).To(BeNumerically(">", 0))

			stored, err := store.GetMessage(body.Message.SessionID, body.Message.MessageID)
			Expect(err).NotTo(HaveOccurred())
			Expect(stored.Message).To(Equal("persist me"))
			Expect(stored.User).To(Equal("brett"))

			// The worker sees the same identity the client was told about.
			Eventually(rec.count).Should(Equal(1))
			Expect(rec.last().MessageID).To(Equal(body.Message.MessageID))
		})

		It("assigns increasing IDs across successive ingests and lists them in order", func() {
			for _, text := range []string{"one", "two", "three"} {
				rr := post(h, "/ingest", `{"type":"user","message":"`+text+`"}`)
				Expect(rr.Code).To(Equal(http.StatusCreated))
			}

			rr := get(h, "/sessions/live/messages")
			Expect(rr.Code).To(Equal(http.StatusOK))
			var msgs []parser.AntigravityMessage
			decodeInto(rr, &msgs)
			Expect(msgs).To(HaveLen(3))
			Expect(msgs[0].Message).To(Equal("one"))
			Expect(msgs[1].Message).To(Equal("two"))
			Expect(msgs[2].Message).To(Equal("three"))
			Expect(msgs[0].MessageID).To(BeNumerically("<", msgs[1].MessageID))
			Expect(msgs[1].MessageID).To(BeNumerically("<", msgs[2].MessageID))
		})

		It("namespaces messages by the client-supplied sessionId", func() {
			Expect(post(h, "/ingest", `{"sessionId":"proj-a","type":"user","message":"a"}`).Code).To(Equal(http.StatusCreated))
			Expect(post(h, "/ingest", `{"sessionId":"proj-b","type":"user","message":"b"}`).Code).To(Equal(http.StatusCreated))
			Expect(post(h, "/ingest", `{"sessionId":"proj-a","type":"system","message":"a2"}`).Code).To(Equal(http.StatusCreated))

			var sessions []storage.SessionInfo
			decodeInto(get(h, "/sessions"), &sessions)
			Expect(sessions).To(Equal([]storage.SessionInfo{
				{SessionID: "proj-a", MessageCount: 2},
				{SessionID: "proj-b", MessageCount: 1},
			}))

			var msgs []parser.AntigravityMessage
			decodeInto(get(h, "/sessions/proj-b/messages"), &msgs)
			Expect(msgs).To(HaveLen(1))
			Expect(msgs[0].Message).To(Equal("b"))
		})

		It("serves a single message and 404s for unknown identities", func() {
			rr := post(h, "/ingest", `{"type":"user","message":"single"}`)
			var body struct {
				Message parser.AntigravityMessage `json:"message"`
			}
			decodeInto(rr, &body)

			one := get(h, "/sessions/live/messages/"+itoa(body.Message.MessageID))
			Expect(one.Code).To(Equal(http.StatusOK))
			var msg parser.AntigravityMessage
			decodeInto(one, &msg)
			Expect(msg.Message).To(Equal("single"))

			Expect(get(h, "/sessions/live/messages/9999").Code).To(Equal(http.StatusNotFound))
			Expect(get(h, "/sessions/nope/messages/1").Code).To(Equal(http.StatusNotFound))
			Expect(get(h, "/sessions/nope/messages").Code).To(Equal(http.StatusNotFound))
			Expect(get(h, "/sessions/live/messages/abc").Code).To(Equal(http.StatusBadRequest))
			Expect(get(h, "/sessions/live/messages/-1").Code).To(Equal(http.StatusBadRequest))
		})

		It("exports a session in a shape ParseChatLog can read back", func() {
			Expect(post(h, "/ingest", `{"sessionId":"rt","type":"user","user":"brett","message":"round","timestamp":"2025-09-04T21:27:18.909Z"}`).Code).To(Equal(http.StatusCreated))
			Expect(post(h, "/ingest", `{"sessionId":"rt","type":"system","message":"trip"}`).Code).To(Equal(http.StatusCreated))

			rr := get(h, "/sessions/rt/messages")
			Expect(rr.Code).To(Equal(http.StatusOK))

			path := filepath.Join(GinkgoT().TempDir(), "export.json")
			Expect(os.WriteFile(path, rr.Body.Bytes(), 0o644)).To(Succeed())

			parsed, err := parser.ParseChatLog(path)
			Expect(err).NotTo(HaveOccurred())
			Expect(parsed).To(HaveLen(2))
			Expect(parsed[0].SessionID).To(Equal("rt"))
			Expect(parsed[0].User).To(Equal("brett"))
			Expect(parsed[0].Message).To(Equal("round"))
			want := time.Date(2025, 9, 4, 21, 27, 18, 909000000, time.UTC)
			Expect(parsed[0].Timestamp.Equal(want)).To(BeTrue())
			Expect(parsed[1].Type).To(Equal("system"))
		})

		It("does not dispatch to workers when persistence fails", func() {
			Expect(store.Close()).To(Succeed())
			rr := post(h, "/ingest", `{"type":"user","message":"lost"}`)
			Expect(rr.Code).To(Equal(http.StatusInternalServerError))
			Consistently(rec.count, "100ms").Should(Equal(0))

			// Reopen so AfterEach can close cleanly.
			var err error
			store, err = storage.NewBoltStorage(filepath.Join(GinkgoT().TempDir(), "reopen.db"))
			Expect(err).NotTo(HaveOccurred())
		})
	})
})

func itoa(i int) string {
	b, _ := json.Marshal(i)
	return string(b)
}

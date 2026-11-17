package api_test

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"path/filepath"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/gnomatix/enkente/pkg/api"
	"github.com/gnomatix/enkente/pkg/parser"
	"github.com/gnomatix/enkente/pkg/storage"
)

func put(h http.Handler, path, body string) *httptest.ResponseRecorder {
	rr := httptest.NewRecorder()
	h.ServeHTTP(rr, httptest.NewRequest(http.MethodPut, path, bytes.NewBufferString(body)))
	return rr
}

var _ = Describe("Entities API", func() {
	var (
		h     http.Handler
		store *storage.BoltStorage
	)

	BeforeEach(func() {
		var err error
		store, err = storage.NewBoltStorage(filepath.Join(GinkgoT().TempDir(), "entities.db"))
		Expect(err).NotTo(HaveOccurred())
		h = api.NewServer(0, 1, func(int, parser.AntigravityMessage) {}, api.WithStore(store)).Handler()
		Expect(post(h, "/ingest", `{"user":"carol","message":"starting #GraphDB"}`).Code).To(Equal(http.StatusCreated))
	})

	AfterEach(func() {
		Expect(store.Close()).To(Succeed())
	})

	It("shows a mentioned entity before anyone curates it", func() {
		rr := get(h, "/entities/tag/graphdb")
		Expect(rr.Code).To(Equal(http.StatusOK))
		var v storage.EntityView
		decodeInto(rr, &v)
		Expect(v.Curated).To(BeNil())
		Expect(v.Mentions.FirstSeen.User).To(Equal("carol"))
	})

	It("curates an entity with PUT and reads it back joined with mentions", func() {
		rr := put(h, "/entities/tag/GraphDB", `{"description":"property graph store","attributes":{"owner":"alice"}}`)
		Expect(rr.Code).To(Equal(http.StatusOK))

		var v storage.EntityView
		decodeInto(get(h, "/entities/tag/graphdb"), &v)
		Expect(v.Curated.Description).To(Equal("property graph store"))
		Expect(v.Curated.Attributes).To(HaveKeyWithValue("owner", "alice"))
		Expect(v.Mentions.Messages).To(Equal(1))

		var list []storage.Entity
		decodeInto(get(h, "/entities"), &list)
		Expect(list).To(HaveLen(1))
		Expect(list[0].Name).To(Equal("graphdb"))
	})

	It("appends notes", func() {
		rr := post(h, "/entities/user/alice/notes", `{"text":"leads the NLP work","author":"bob"}`)
		Expect(rr.Code).To(Equal(http.StatusCreated))
		rr = post(h, "/entities/user/alice/notes", `{"text":"prefers Go"}`)
		var e storage.Entity
		decodeInto(rr, &e)
		Expect(e.Notes).To(HaveLen(2))
		Expect(post(h, "/entities/user/alice/notes", `{"text":""}`).Code).To(Equal(http.StatusBadRequest))
	})

	It("deletes the curated record but keeps mentions", func() {
		Expect(put(h, "/entities/tag/graphdb", `{"description":"x"}`).Code).To(Equal(http.StatusOK))
		Expect(del(h, "/entities/tag/graphdb").Code).To(Equal(http.StatusNoContent))
		Expect(del(h, "/entities/tag/graphdb").Code).To(Equal(http.StatusNotFound))
		var v storage.EntityView
		decodeInto(get(h, "/entities/tag/graphdb"), &v)
		Expect(v.Curated).To(BeNil())
		Expect(v.Mentions).NotTo(BeNil())
	})

	It("validates input", func() {
		Expect(get(h, "/entities/tag/unknown").Code).To(Equal(http.StatusNotFound))
		Expect(get(h, "/entities/topic/x").Code).To(Equal(http.StatusBadRequest))
		Expect(get(h, "/entities/tag/42").Code).To(Equal(http.StatusBadRequest))
		Expect(put(h, "/entities/tag/x", `{"kind":"user"}`).Code).To(Equal(http.StatusBadRequest), "unknown fields rejected")
		Expect(put(h, "/entities/tag/x", `{"notes":[{"text":" "}]}`).Code).To(Equal(http.StatusBadRequest))
	})

	It("returns 503 without a store", func() {
		bare := api.NewServer(0, 1, func(int, parser.AntigravityMessage) {}).Handler()
		Expect(get(bare, "/entities").Code).To(Equal(http.StatusServiceUnavailable))
	})
})

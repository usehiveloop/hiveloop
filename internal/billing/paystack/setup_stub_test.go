package paystack

import (
	"encoding/json"
	"io"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"sync"
)

// paystackStub is an in-memory fake of Paystack's /plan API. It supports
// GET list (with paging), POST create, and PUT update. Every test uses a
// fresh stub; no shared state across tests. Lives in its own file so the
// actual tests stay under the 300-line ceiling.
type paystackStub struct {
	mu          sync.Mutex
	plans       map[string]*paystackPlan // keyed by plan_code
	createCalls int
	updateCalls int
	// forcePage overrides the perPage returned in list responses so we can
	// exercise pagination even with tiny fixtures.
	forcePage int
	nextID    int64
	nextCode  int
}

func newStub() *paystackStub {
	return &paystackStub{
		plans:    map[string]*paystackPlan{},
		nextID:   1,
		nextCode: 1,
	}
}

// Seed adds a plan directly (no createCalls increment). Used to pre-populate
// fixtures that simulate "existing" upstream state.
func (s *paystackStub) Seed(p paystackPlan) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if p.PlanCode == "" {
		p.PlanCode = "PLN_seed" + strconv.Itoa(s.nextCode)
		s.nextCode++
	}
	if p.ID == 0 {
		p.ID = s.nextID
		s.nextID++
	}
	s.plans[p.PlanCode] = &p
}

func (s *paystackStub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if got := r.Header.Get("Authorization"); !strings.HasPrefix(got, "Bearer ") {
		http.Error(w, "missing bearer", http.StatusUnauthorized)
		return
	}
	switch {
	case r.Method == http.MethodGet && r.URL.Path == "/plan":
		s.list(w, r)
	case r.Method == http.MethodPost && r.URL.Path == "/plan":
		s.create(w, r)
	case r.Method == http.MethodPut && strings.HasPrefix(r.URL.Path, "/plan/"):
		s.update(w, r)
	default:
		http.NotFound(w, r)
	}
}

func (s *paystackStub) list(w http.ResponseWriter, r *http.Request) {
	s.mu.Lock()
	defer s.mu.Unlock()

	perPage, _ := strconv.Atoi(r.URL.Query().Get("perPage"))
	if perPage == 0 {
		perPage = 50
	}
	if s.forcePage > 0 {
		perPage = s.forcePage
	}
	page, _ := strconv.Atoi(r.URL.Query().Get("page"))
	if page == 0 {
		page = 1
	}

	// Deterministic order by plan_code so tests are stable.
	codes := make([]string, 0, len(s.plans))
	for k := range s.plans {
		codes = append(codes, k)
	}
	sort.Strings(codes)

	total := len(codes)
	start := (page - 1) * perPage
	end := start + perPage
	if start > total {
		start = total
	}
	if end > total {
		end = total
	}
	slice := make([]paystackPlan, 0, end-start)
	for _, c := range codes[start:end] {
		slice = append(slice, *s.plans[c])
	}
	pageCount := (total + perPage - 1) / perPage
	if pageCount == 0 {
		pageCount = 1
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"status":  true,
		"message": "ok",
		"data":    slice,
		"meta":    map[string]int{"total": total, "page": page, "perPage": perPage, "pageCount": pageCount},
	})
}

type stubCreateBody struct {
	Name        string `json:"name"`
	Amount      int64  `json:"amount"`
	Interval    string `json:"interval"`
	Currency    string `json:"currency"`
	Description string `json:"description"`
}

func (s *paystackStub) create(w http.ResponseWriter, r *http.Request) {
	body, _ := io.ReadAll(r.Body)
	var req stubCreateBody
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.createCalls++
	p := paystackPlan{
		ID:          s.nextID,
		PlanCode:    "PLN_new" + strconv.Itoa(s.nextCode),
		Name:        req.Name,
		Amount:      req.Amount,
		Currency:    req.Currency,
		Interval:    req.Interval,
		Description: req.Description,
	}
	s.nextID++
	s.nextCode++
	s.plans[p.PlanCode] = &p
	writeJSON(w, http.StatusOK, map[string]any{"status": true, "message": "ok", "data": p})
}

type stubUpdateBody struct {
	Name                        string `json:"name"`
	Amount                      int64  `json:"amount"`
	Interval                    string `json:"interval"`
	Currency                    string `json:"currency"`
	Description                 string `json:"description"`
	UpdateExistingSubscriptions *bool  `json:"update_existing_subscriptions"`
}

func (s *paystackStub) update(w http.ResponseWriter, r *http.Request) {
	code := strings.TrimPrefix(r.URL.Path, "/plan/")
	body, _ := io.ReadAll(r.Body)
	var req stubUpdateBody
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.updateCalls++
	p, ok := s.plans[code]
	if !ok {
		http.Error(w, "plan not found", http.StatusNotFound)
		return
	}
	p.Name = req.Name
	p.Amount = req.Amount
	p.Interval = req.Interval
	p.Currency = req.Currency
	p.Description = req.Description
	writeJSON(w, http.StatusOK, map[string]any{"status": true, "message": "ok", "data": p})
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

package httpui

import (
	"net/http"
	"time"

	"mastergate/internal/domain"
	"mastergate/internal/workflow"
)

func (s *Server) HandleWorkspace(w http.ResponseWriter, r *http.Request) {
	if r.URL.Path != "/" {
		http.NotFound(w, r)
		return
	}
	b, err := assets.ReadFile("assets/index.html")
	if err != nil {
		writeError(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(b)
}

func (s *Server) HandleListCases(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	filter := workflow.CaseListFilter{ProgramCode: query.Get("program_code"), State: domain.CaseState(query.Get("state")), EngineerID: query.Get("engineer_id")}
	var err error
	if value := query.Get("approved_from"); value != "" {
		filter.ApprovedFrom, err = parseQueryTime(value)
		if err != nil {
			writeError(w, err)
			return
		}
	}
	if value := query.Get("approved_to"); value != "" {
		filter.ApprovedTo, err = parseQueryTime(value)
		if err != nil {
			writeError(w, err)
			return
		}
	}
	cases, err := s.service.SearchCases(r.Context(), filter)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"cases": cases})
}

func parseQueryTime(value string) (*time.Time, error) {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return nil, domain.NewError(domain.CodeInvalid, "批准时间筛选必须使用 RFC3339 格式")
	}
	return &parsed, nil
}

func (s *Server) HandleCreateCase(w http.ResponseWriter, r *http.Request) {
	var c workflow.CreateCaseCommand
	if err := decodeJSON(w, r, &c); err != nil {
		writeError(w, err)
		return
	}
	result, err := s.service.CreateCase(r.Context(), c)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) HandleCaseRoute(w http.ResponseWriter, r *http.Request) {
	caseID, action, ok := splitCasePath(r.URL.Path)
	if !ok {
		http.NotFound(w, r)
		return
	}
	queryActions := map[string]bool{"": true, "timeline": true, "verify": true, "preflight": true, "readiness": true}
	commandActions := map[string]bool{"metadata": true, "freeze": true, "segments": true, "segment-revisions": true, "segment-withdrawals": true, "measurements": true, "measurement-batches": true, "evaluate": true, "corrections": true, "joint-corrections": true, "retests": true, "joint-retests": true, "review": true}
	if queryActions[action] {
		if r.Method != http.MethodGet {
			w.Header().Set("Allow", http.MethodGet)
			writeError(w, domain.NewError(domain.CodeInvalid, "该查询端点仅支持 GET 方法"))
			return
		}
		switch action {
		case "":
			s.HandleGetCase(w, r, caseID)
		case "timeline":
			s.HandleTimeline(w, r, caseID)
		case "verify":
			s.HandleVerifyManifest(w, r, caseID)
		case "preflight":
			s.HandleSegmentPreflight(w, r, caseID)
		case "readiness":
			s.HandleReadiness(w, r, caseID)
		}
		return
	}
	if !commandActions[action] {
		http.NotFound(w, r)
		return
	}
	if r.Method != http.MethodPost {
		w.Header().Set("Allow", http.MethodPost)
		writeError(w, domain.NewError(domain.CodeInvalid, "该命令端点仅支持 POST 方法"))
		return
	}
	switch action {
	case "metadata":
		s.HandleReviseCase(w, r, caseID)
	case "freeze":
		s.HandleFreezeBaseline(w, r, caseID)
	case "segments":
		s.HandleAddSegment(w, r, caseID)
	case "segment-revisions":
		s.HandleReviseSegment(w, r, caseID)
	case "segment-withdrawals":
		s.HandleWithdrawSegment(w, r, caseID)
	case "measurements":
		s.HandleSubmitMeasurement(w, r, caseID)
	case "measurement-batches":
		s.HandleSubmitMeasurementBatch(w, r, caseID)
	case "evaluate":
		s.HandleEvaluate(w, r, caseID)
	case "corrections":
		s.HandleCorrectDeviation(w, r, caseID)
	case "joint-corrections":
		s.HandleCorrectDeviations(w, r, caseID)
	case "retests":
		s.HandleRetest(w, r, caseID)
	case "joint-retests":
		s.HandleJointRetest(w, r, caseID)
	case "review":
		s.HandleReview(w, r, caseID)
	}
}

func (s *Server) HandleSegmentPreflight(w http.ResponseWriter, r *http.Request, caseID string) {
	report, err := s.service.SegmentPreflight(r.Context(), caseID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}

func (s *Server) HandleGetCase(w http.ResponseWriter, r *http.Request, caseID string) {
	view, err := s.service.GetCase(r.Context(), caseID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, view)
}
func (s *Server) HandleReadiness(w http.ResponseWriter, r *http.Request, caseID string) {
	report, err := s.service.Readiness(r.Context(), caseID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, report)
}
func (s *Server) HandleTimeline(w http.ResponseWriter, r *http.Request, caseID string) {
	events, err := s.service.Timeline(r.Context(), caseID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"events": events})
}
func (s *Server) HandleVerifyManifest(w http.ResponseWriter, r *http.Request, caseID string) {
	result, err := s.service.VerifyManifest(r.Context(), caseID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

package httpui

import (
	"net/http"

	"mastergate/internal/workflow"
)

func (s *Server) HandleReviseCase(w http.ResponseWriter, r *http.Request, id string) {
	var c workflow.ReviseCaseCommand
	if err := decodeJSON(w, r, &c); err != nil {
		writeError(w, err)
		return
	}
	c.CaseID = id
	result, err := s.service.ReviseCase(r.Context(), c)
	respondCommand(w, result, err)
}

func (s *Server) HandleFreezeBaseline(w http.ResponseWriter, r *http.Request, id string) {
	var c workflow.FreezeBaselineCommand
	if err := decodeJSON(w, r, &c); err != nil {
		writeError(w, err)
		return
	}
	c.CaseID = id
	result, err := s.service.FreezeBaseline(r.Context(), c)
	respondCommand(w, result, err)
}
func (s *Server) HandleAddSegment(w http.ResponseWriter, r *http.Request, id string) {
	var c workflow.AddSegmentCommand
	if err := decodeJSON(w, r, &c); err != nil {
		writeError(w, err)
		return
	}
	c.CaseID = id
	result, err := s.service.AddSegment(r.Context(), c)
	respondCommand(w, result, err)
}
func (s *Server) HandleReviseSegment(w http.ResponseWriter, r *http.Request, id string) {
	var c workflow.ReviseSegmentCommand
	if err := decodeJSON(w, r, &c); err != nil {
		writeError(w, err)
		return
	}
	c.CaseID = id
	result, err := s.service.ReviseSegment(r.Context(), c)
	respondCommand(w, result, err)
}
func (s *Server) HandleWithdrawSegment(w http.ResponseWriter, r *http.Request, id string) {
	var c workflow.WithdrawSegmentCommand
	if err := decodeJSON(w, r, &c); err != nil {
		writeError(w, err)
		return
	}
	c.CaseID = id
	result, err := s.service.WithdrawSegment(r.Context(), c)
	respondCommand(w, result, err)
}
func (s *Server) HandleSubmitMeasurement(w http.ResponseWriter, r *http.Request, id string) {
	var c workflow.SubmitMeasurementCommand
	if err := decodeJSON(w, r, &c); err != nil {
		writeError(w, err)
		return
	}
	c.CaseID = id
	result, err := s.service.SubmitMeasurement(r.Context(), c)
	respondCommand(w, result, err)
}
func (s *Server) HandleSubmitMeasurementBatch(w http.ResponseWriter, r *http.Request, id string) {
	var c workflow.SubmitMeasurementBatchCommand
	if err := decodeJSON(w, r, &c); err != nil {
		writeError(w, err)
		return
	}
	c.CaseID = id
	result, err := s.service.SubmitMeasurementBatch(r.Context(), c)
	respondCommand(w, result, err)
}
func (s *Server) HandleEvaluate(w http.ResponseWriter, r *http.Request, id string) {
	var c workflow.EvaluateCommand
	if err := decodeJSON(w, r, &c); err != nil {
		writeError(w, err)
		return
	}
	c.CaseID = id
	result, err := s.service.Evaluate(r.Context(), c)
	respondCommand(w, result, err)
}
func (s *Server) HandleCorrectDeviation(w http.ResponseWriter, r *http.Request, id string) {
	var c workflow.CorrectDeviationCommand
	if err := decodeJSON(w, r, &c); err != nil {
		writeError(w, err)
		return
	}
	c.CaseID = id
	result, err := s.service.CorrectDeviation(r.Context(), c)
	respondCommand(w, result, err)
}
func (s *Server) HandleCorrectDeviations(w http.ResponseWriter, r *http.Request, id string) {
	var c workflow.CorrectDeviationsCommand
	if err := decodeJSON(w, r, &c); err != nil {
		writeError(w, err)
		return
	}
	c.CaseID = id
	result, err := s.service.CorrectDeviations(r.Context(), c)
	respondCommand(w, result, err)
}
func (s *Server) HandleRetest(w http.ResponseWriter, r *http.Request, id string) {
	var c workflow.RetestCommand
	if err := decodeJSON(w, r, &c); err != nil {
		writeError(w, err)
		return
	}
	c.CaseID = id
	result, err := s.service.Retest(r.Context(), c)
	respondCommand(w, result, err)
}
func (s *Server) HandleJointRetest(w http.ResponseWriter, r *http.Request, id string) {
	var c workflow.JointRetestCommand
	if err := decodeJSON(w, r, &c); err != nil {
		writeError(w, err)
		return
	}
	c.CaseID = id
	result, err := s.service.JointRetest(r.Context(), c)
	respondCommand(w, result, err)
}
func (s *Server) HandleReview(w http.ResponseWriter, r *http.Request, id string) {
	var c workflow.ReviewCommand
	if err := decodeJSON(w, r, &c); err != nil {
		writeError(w, err)
		return
	}
	c.CaseID = id
	result, err := s.service.Review(r.Context(), c)
	respondCommand(w, result, err)
}
func respondCommand(w http.ResponseWriter, result workflow.CommandResult, err error) {
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, result)
}

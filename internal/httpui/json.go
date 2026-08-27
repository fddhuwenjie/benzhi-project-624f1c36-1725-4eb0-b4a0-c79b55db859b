package httpui

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"

	"mastergate/internal/domain"
)

const maxRequestBytes = 1 << 20

type problem struct {
	Code            string                  `json:"code"`
	Title           string                  `json:"title"`
	Detail          string                  `json:"detail"`
	CurrentRevision int64                   `json:"current_revision,omitempty"`
	LatestBlockers  []domain.ReadinessCheck `json:"latest_blockers,omitempty"`
}

func decodeJSON(w http.ResponseWriter, r *http.Request, target any) error {
	if !strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/json") {
		return domain.NewError(domain.CodeInvalid, "Content-Type 必须为 application/json")
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxRequestBytes)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return domain.NewError(domain.CodeInvalid, "JSON 请求无效：%v", err)
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return domain.NewError(domain.CodeInvalid, "请求体只能包含一个 JSON 对象")
	}
	return nil
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, err error) {
	status := http.StatusInternalServerError
	p := problem{Code: "internal", Title: "服务器错误", Detail: "服务器无法完成请求"}
	if e, ok := err.(*domain.Error); ok {
		p.Code = string(e.Code)
		p.Detail = e.Message
		p.CurrentRevision = e.CurrentRevision
		p.LatestBlockers = e.LatestBlockers
		switch e.Code {
		case domain.CodeInvalid:
			status = http.StatusBadRequest
			p.Title = "输入无效"
		case domain.CodeNotFound:
			status = http.StatusNotFound
			p.Title = "资源不存在"
		case domain.CodeForbidden:
			status = http.StatusForbidden
			p.Title = "操作被拒绝"
		case domain.CodeConflict, domain.CodeIdempotent, domain.CodeState:
			status = http.StatusConflict
			p.Title = "操作冲突"
		case domain.CodeIntegrity:
			status = http.StatusUnprocessableEntity
			p.Title = "完整性校验失败"
		}
	}
	writeJSON(w, status, p)
}

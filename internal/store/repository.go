package store

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"mastergate/internal/domain"
)

type Repository struct {
	mu   sync.RWMutex
	path string
	data snapshot
	now  func() time.Time
}

func Open(path string) (*Repository, error) {
	r := &Repository{path: path, data: emptySnapshot(), now: time.Now}
	if path == "" {
		return r, nil
	}
	b, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return r, nil
	}
	if err != nil {
		return nil, fmt.Errorf("读取存储文件: %w", err)
	}
	if len(b) == 0 {
		return r, nil
	}
	if err := json.Unmarshal(b, &r.data); err != nil {
		return nil, fmt.Errorf("解析存储文件: %w", err)
	}
	if r.data.Cases == nil {
		r.data.Cases = make(map[string]*CaseData)
	}
	if r.data.Idempotency == nil {
		r.data.Idempotency = make(map[string]domain.IdempotencyRecord)
	}
	// 完整性异常由只读 verify 查询逐项报告，打开存储时不自动修复或覆盖封存数据。
	return r, nil
}

func (r *Repository) Close() error { return nil }

func (r *Repository) Execute(ctx context.Context, m Mutation, fn func(*CaseData) (any, error)) (Result, error) {
	if err := ctx.Err(); err != nil {
		return Result{}, err
	}
	if err := domain.ValidateIdentifier("request_id", m.RequestID); err != nil {
		return Result{}, err
	}
	if m.Fingerprint == "" {
		return Result{}, domain.NewError(domain.CodeInvalid, "请求指纹不能为空")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if record, ok := r.data.Idempotency[m.RequestID]; ok {
		if record.Fingerprint != m.Fingerprint {
			return Result{}, domain.NewError(domain.CodeIdempotent, "request_id 已被不同请求使用")
		}
		return Result{Response: append([]byte(nil), record.Response...), StatusCode: record.StatusCode, Replayed: true}, nil
	}
	working, err := cloneSnapshot(r.data)
	if err != nil {
		return Result{}, err
	}
	data, exists := working.Cases[m.CaseID]
	if m.DraftIdentity != "" {
		for id, existing := range working.Cases {
			if id != m.CaseID && existing.Case.State == domain.StateDraft && DraftIdentity(existing.Case) == m.DraftIdentity {
				return Result{}, domain.NewError(domain.CodeConflict, "检测到重复草稿：case_id=%s，revision=%d", existing.Case.CaseID, existing.Case.Revision)
			}
		}
	}
	if m.Create {
		if exists {
			return Result{}, domain.NewError(domain.CodeConflict, "案件标识已存在")
		}
		if m.ExpectedRevision != 0 {
			return Result{}, domain.Conflict(0)
		}
		data = &CaseData{}
		working.Cases[m.CaseID] = data
	} else {
		if !exists {
			return Result{}, domain.NewError(domain.CodeNotFound, "案件不存在")
		}
		if data.Case.Revision != m.ExpectedRevision {
			return Result{}, domain.Conflict(data.Case.Revision)
		}
		if data.Case.State.Terminal() {
			return Result{}, domain.NewError(domain.CodeState, "终态案件禁止修改")
		}
	}
	beforeRevision := data.Case.Revision
	value, err := fn(data)
	if err != nil {
		return Result{}, err
	}
	if m.Create {
		if data.Case.Revision != 1 {
			return Result{}, domain.NewError(domain.CodeIntegrity, "新案件初始修订号必须为 1")
		}
	} else if data.Case.Revision != beforeRevision+1 {
		return Result{}, domain.NewError(domain.CodeIntegrity, "每个命令必须且只能推进一个修订号")
	}
	responseValue := value
	eventData := value
	var finalize func(*CaseData, domain.Event) (any, error)
	if change, ok := value.(Change); ok {
		responseValue = change.Response
		if change.EventData != nil {
			eventData = change.EventData
		}
		finalize = change.Finalize
	}
	previous := ""
	if n := len(data.Events); n > 0 {
		previous = data.Events[n-1].Digest
	}
	event, err := domain.BuildEvent(int64(len(data.Events)+1), m.CaseID, m.EventType, m.ActorID, data.Case.Revision, eventData, previous, r.now())
	if err != nil {
		return Result{}, err
	}
	data.Events = append(data.Events, event)
	if finalize != nil {
		responseValue, err = finalize(data, event)
		if err != nil {
			return Result{}, err
		}
	}
	response, err := json.Marshal(responseValue)
	if err != nil {
		return Result{}, fmt.Errorf("编码事务响应: %w", err)
	}
	status := m.StatusCode
	if status == 0 {
		status = 200
	}
	working.Idempotency[m.RequestID] = domain.IdempotencyRecord{RequestID: m.RequestID, Fingerprint: m.Fingerprint, Response: response, StatusCode: status}
	if err := validateCase(data); err != nil {
		return Result{}, err
	}
	if err := r.persist(working); err != nil {
		return Result{}, err
	}
	r.data = working
	return Result{Response: response, StatusCode: status}, nil
}

func cloneSnapshot(source snapshot) (snapshot, error) {
	b, err := json.Marshal(source)
	if err != nil {
		return snapshot{}, err
	}
	var target snapshot
	if err := json.Unmarshal(b, &target); err != nil {
		return snapshot{}, err
	}
	return target, nil
}

func (r *Repository) persist(next snapshot) error {
	if r.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(r.path), 0700); err != nil {
		return fmt.Errorf("创建存储目录: %w", err)
	}
	b, err := json.MarshalIndent(next, "", "  ")
	if err != nil {
		return fmt.Errorf("编码存储快照: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(r.path), "mastergate-*.tmp")
	if err != nil {
		return fmt.Errorf("创建事务临时文件: %w", err)
	}
	tmpName := tmp.Name()
	ok := false
	defer func() {
		tmp.Close()
		if !ok {
			_ = os.Remove(tmpName)
		}
	}()
	if err := tmp.Chmod(0600); err != nil {
		return err
	}
	if _, err := tmp.Write(b); err != nil {
		return fmt.Errorf("写入事务快照: %w", err)
	}
	if err := tmp.Sync(); err != nil {
		return fmt.Errorf("同步事务快照: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, r.path); err != nil {
		return fmt.Errorf("提交事务快照: %w", err)
	}
	ok = true
	return nil
}

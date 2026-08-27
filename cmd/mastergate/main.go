package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"mastergate/internal/domain"
	"mastergate/internal/httpui"
	"mastergate/internal/store"
	"mastergate/internal/workflow"
)

func resolveAddr(value string) (string, error) {
	if value == "" {
		if port := os.Getenv("PORT"); port != "" {
			value = "127.0.0.1:" + port
		} else {
			value = "127.0.0.1:19081"
		}
	}
	if !strings.HasPrefix(value, "127.0.0.1:") {
		return "", fmt.Errorf("监听地址必须绑定 127.0.0.1")
	}
	parts := strings.Split(value, ":")
	if len(parts) != 2 {
		return "", fmt.Errorf("监听地址格式必须为 127.0.0.1:<port>")
	}
	port, err := strconv.Atoi(parts[1])
	if err != nil || port < 1024 || port > 65535 {
		return "", fmt.Errorf("端口必须位于 1024 至 65535")
	}
	return value, nil
}

func newServer(addr, storagePath string) (*http.Server, *workflow.Service, error) {
	repo, err := store.Open(storagePath)
	if err != nil {
		return nil, nil, err
	}
	service := workflow.New(repo)
	handler := httpui.New(service)
	return &http.Server{Addr: addr, Handler: handler.Handler(), ReadHeaderTimeout: 3 * time.Second, ReadTimeout: 10 * time.Second, WriteTimeout: 15 * time.Second, IdleTimeout: 60 * time.Second}, service, nil
}

func main() {
	addrFlag := flag.String("addr", "", "回环监听地址")
	selftest := flag.Bool("selftest", false, "运行有界完整流程自检")
	storage := flag.String("storage", filepath.Join(os.TempDir(), "mastergate.json"), "本地事务快照路径")
	flag.Parse()
	addr, err := resolveAddr(*addrFlag)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(2)
	}
	if *selftest {
		if err := runSelftest(addr); err != nil {
			fmt.Fprintln(os.Stderr, "自检失败:", err)
			os.Exit(1)
		}
		fmt.Println("自检通过")
		return
	}
	server, _, err := newServer(addr, *storage)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintln(os.Stderr, err)
		}
	}()
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, syscall.SIGINT, syscall.SIGTERM)
	<-signals
	ctx, cancel := context.WithTimeout(context.Background(), 8*time.Second)
	defer cancel()
	_ = server.Shutdown(ctx)
}

func runSelftest(addr string) error {
	path, err := os.CreateTemp("", "mastergate-selftest-*.json")
	if err != nil {
		return err
	}
	pathName := path.Name()
	_ = path.Close()
	defer os.Remove(pathName)
	server, service, err := newServer(addr, pathName)
	if err != nil {
		return err
	}
	listenerErr := make(chan error, 1)
	go func() { listenerErr <- server.ListenAndServe() }()
	time.Sleep(60 * time.Millisecond)
	client := &http.Client{Timeout: 3 * time.Second}
	ctx := context.Background()
	sha := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	post := func(path string, body any) ([]byte, error) {
		b, err := jsonMarshal(body)
		if err != nil {
			return nil, err
		}
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, "http://"+addr+path, strings.NewReader(string(b)))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		resp, err := client.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()
		data, _ := io.ReadAll(resp.Body)
		if resp.StatusCode >= 300 {
			return nil, fmt.Errorf("POST %s %s: %s", path, resp.Status, string(data))
		}
		return data, nil
	}
	base := map[string]any{"request_id": "req-create", "expected_revision": 0, "case_id": "case-demo", "program_code": "NEWS01", "master_version": "v1", "engineer_id": "eng01", "master_sha256": sha, "standard_profile": map[string]any{"name": "EBU-R128", "target_integrated_lufs": -23, "integrated_tolerance_lu": 1, "max_loudness_range_lu": 20, "max_true_peak_dbtp": -1, "expected_duration_millis": 1000}}
	if _, err := post("/api/cases", base); err != nil {
		return err
	}
	rev := int64(1)
	send := func(path, request, actor string, extra map[string]any) error {
		body := map[string]any{"request_id": request, "expected_revision": rev, "actor_id": actor}
		for k, v := range extra {
			body[k] = v
		}
		if _, err := post("/api/cases/case-demo/"+path, body); err != nil {
			return err
		}
		rev++
		return nil
	}
	if err := send("freeze", "req-freeze", "eng01", nil); err != nil {
		return err
	}
	if err := send("segments", "req-segment", "eng01", map[string]any{"segment": map[string]any{"segment_id": "seg01", "title": "开场", "start_millis": 0, "end_millis": 1000, "channel_layout": "stereo", "audio_sha256": sha, "calibration_ref": "CAL-01"}}); err != nil {
		return err
	}
	bad := func(id, scopeType, scopeID string) map[string]any {
		return map[string]any{"measurement": map[string]any{"measurement_id": id, "scope_type": scopeType, "scope_id": scopeID, "integrated_lufs": -20, "loudness_range_lu": 10, "true_peak_dbtp": -2, "gate_threshold_lu": -10, "integrated_unit": "LUFS", "range_unit": "LU", "peak_unit": "dBTP", "gate_unit": "LU", "evidence_sha256": sha}}
	}
	programBad := bad("m1", "program", "case-demo")
	programBad["measurement"].(map[string]any)["true_peak_dbtp"] = 0
	if err := send("measurements", "req-m1", "eng01", programBad); err != nil {
		return err
	}
	segGood := bad("m2", "segment", "seg01")
	segGood["measurement"].(map[string]any)["integrated_lufs"] = -23
	if err := send("measurements", "req-m2", "eng01", segGood); err != nil {
		return err
	}
	if err := send("evaluate", "req-eval", "eng01", nil); err != nil {
		return err
	}
	view, err := service.GetCase(ctx, "case-demo")
	if err != nil || len(view.Deviations) != 2 {
		return fmt.Errorf("未产生同范围两项偏差")
	}
	deviationIDs := []string{view.Deviations[0].DeviationID, view.Deviations[1].DeviationID}
	if err := send("joint-corrections", "req-correct", "eng01", map[string]any{"deviation_ids": deviationIDs, "root_cause": "增益链共同偏移", "correction_summary": "重新校准并归一化", "replacement_audio_sha256": sha}); err != nil {
		return err
	}
	good := bad("m3", "program", "case-demo")
	good["measurement"].(map[string]any)["integrated_lufs"] = -23
	goodMeasurement := good["measurement"].(map[string]any)
	goodMeasurement["scope_type"] = view.Deviations[0].ScopeType
	goodMeasurement["scope_id"] = view.Deviations[0].ScopeID
	goodMeasurement["supersedes_id"] = view.Deviations[0].FailedMeasurementID
	if err := send("joint-retests", "req-retest", "eng01", map[string]any{"deviation_ids": deviationIDs, "measurement": goodMeasurement}); err != nil {
		return err
	}
	view, err = service.GetCase(ctx, "case-demo")
	if err != nil {
		return err
	}
	readinessResponse, err := client.Get("http://" + addr + "/api/cases/case-demo/readiness")
	if err != nil {
		return err
	}
	defer readinessResponse.Body.Close()
	var readiness domain.ReadinessReport
	if readinessResponse.StatusCode != http.StatusOK || json.NewDecoder(readinessResponse.Body).Decode(&readiness) != nil || !readiness.Ready || readiness.Revision != rev {
		return fmt.Errorf("复核就绪度矩阵未通过")
	}
	annotations := []map[string]any{{"check_type": "baseline", "comment": "冻结标准已核对"}, {"check_type": "evidence", "comment": "测量证据已核对"}, {"check_type": "rules", "comment": "规则结果已核对"}, {"check_type": "remediation", "comment": "整改历史已核对"}}
	if err := send("review", "req-review", "reviewer01", map[string]any{"reviewer_id": "reviewer01", "decision": "approve", "annotations": annotations}); err != nil {
		return err
	}
	verification, err := service.VerifyManifest(ctx, "case-demo")
	if err != nil || !verification.Valid {
		return fmt.Errorf("清单校验失败: %v", err)
	}
	shutdownCtx, cancel := context.WithTimeout(ctx, time.Second)
	defer cancel()
	_ = server.Shutdown(shutdownCtx)
	select {
	case err := <-listenerErr:
		if err != nil && err != http.ErrServerClosed {
			return err
		}
	default:
	}
	return nil
}

func jsonMarshal(v any) ([]byte, error) { return json.Marshal(v) }

var _ = domain.RuleIntegrated

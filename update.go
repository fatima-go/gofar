/*
 * Copyright 2026 github.com/fatima-go
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * @project fatima-core
 * @author jin
 * @date 26. 11. 24.
 */

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"runtime/debug"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// 신규 버전 안내
//
// 실제 빌드를 진행할 때 백그라운드로 최신 버전을 조회하여 /tmp/fatima-<uid>/gofar-update.yml 에 기록하고,
// 빌드가 성공적으로 끝난 시점에 더 높은 버전이 있으면 업그레이드 방법을 안내한다.
// - 조회는 하루에 1번만 한다 (checked 의 로컬 날짜 기준)
// - 빌드 종료 시점에 조회가 끝나지 않았으면 기다리지 않고 안내를 생략한다
// - 모든 실패는 조용히 무시한다
// - GOFAR_NO_UPDATE_CHECK 또는 CI 환경변수가 있으면 동작하지 않는다

const (
	gofarModulePath      = "github.com/fatima-go/gofar/v2"
	updateProxyURL       = "https://proxy.golang.org/" + gofarModulePath + "/@latest"
	updateCheckTimeout   = 3 * time.Second
	updateStateFilename  = "gofar-update.yml"
	updateDisableEnvName = "GOFAR_NO_UPDATE_CHECK"
)

type updateState struct {
	Checked time.Time `yaml:"checked"` // 실제 조회한 시스템 시각
	Latest  string    `yaml:"latest"`  // 조회 결과. 실패시 ""
	Current string    `yaml:"current"` // 조회 당시 바이너리 버전 (참고용)
}

type UpdateChecker struct {
	dir     string
	current string
	now     func() time.Time
	fetch   func(ctx context.Context) (string, error)
	done    chan struct{}
	state   *updateState
}

// GetVersion 은 실행중인 바이너리의 버전을 v 접두어를 붙여 반환한다.
// go install 로 설치한 경우 빌드 정보의 모듈 버전을, 로컬 빌드((devel))는 소스의 version 을 사용한다
func GetVersion() string {
	if info, ok := debug.ReadBuildInfo(); ok {
		v := info.Main.Version
		if v != "" && v != "(devel)" {
			return v
		}
	}
	return "v" + version
}

// StartUpdateCheck 는 필요한 경우 백그라운드로 최신 버전 조회를 시작한다. 호출자를 블록하지 않는다
func StartUpdateCheck() *UpdateChecker {
	if os.Getenv(updateDisableEnvName) != "" || os.Getenv("CI") != "" {
		return nil
	}
	u := &UpdateChecker{
		dir:     updateStateDir(),
		current: GetVersion(),
		now:     time.Now,
		fetch:   fetchLatestVersion,
	}
	u.start()
	return u
}

func updateStateDir() string {
	base := "/tmp"
	if runtime.GOOS == "windows" {
		base = os.TempDir()
	}
	return filepath.Join(base, fmt.Sprintf("fatima-%d", os.Getuid()))
}

func (u *UpdateChecker) start() {
	u.done = make(chan struct{})

	state := readUpdateState(filepath.Join(u.dir, updateStateFilename))
	if !needsUpdateCheck(state, u.now()) {
		u.state = state
		close(u.done)
		return
	}

	go func() {
		defer close(u.done)
		defer func() { _ = recover() }() // 어떤 경우에도 빌드에 영향을 주지 않는다

		ctx, cancel := context.WithTimeout(context.Background(), updateCheckTimeout)
		defer cancel()

		latest, err := u.fetch(ctx)
		if err != nil {
			latest = "" // 실패도 기록하여 당일 재조회를 막는다
		}
		state := &updateState{Checked: u.now(), Latest: latest, Current: u.current}
		_ = writeUpdateState(u.dir, state)
		u.state = state
	}()
}

// Notify 는 조회가 끝났고 더 높은 버전이 있으면 안내를 출력한다. 조회가 끝나지 않았으면 기다리지 않는다
func (u *UpdateChecker) Notify(w io.Writer) {
	if u == nil {
		return
	}
	select {
	case <-u.done:
	default:
		return
	}
	if u.state == nil || !isNewerVersion(u.state.Latest, u.current) {
		return
	}

	line := strings.Repeat("─", 60)
	fmt.Fprintf(w, "%s\n", line)
	fmt.Fprintf(w, " gofar 새 버전 %s 이 있습니다 (현재 %s)\n", u.state.Latest, u.current)
	fmt.Fprintf(w, " 업그레이드 : go install %s@latest\n", gofarModulePath)
	fmt.Fprintf(w, " 변경 내역 : https://github.com/fatima-go/gofar#release\n")
	fmt.Fprintf(w, "%s\n", line)
}

// needsUpdateCheck 는 오늘(로컬 날짜) 조회한 기록이 없으면 true 를 반환한다
func needsUpdateCheck(state *updateState, now time.Time) bool {
	if state == nil || state.Checked.IsZero() {
		return true
	}
	checked := state.Checked.In(now.Location())
	if checked.After(now) {
		return true // 시스템 시계가 바뀐 경우 영영 조회하지 않는 것을 막는다
	}
	cy, cm, cd := checked.Date()
	ny, nm, nd := now.Date()
	return cy != ny || cm != nm || cd != nd
}

func readUpdateState(path string) *updateState {
	b, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	var state updateState
	if err := yaml.Unmarshal(b, &state); err != nil {
		return nil
	}
	return &state
}

// writeUpdateState 는 동시에 실행되는 gofar 가 깨진 파일을 읽지 않도록 임시 파일에 쓴 뒤 rename 한다
func writeUpdateState(dir string, state *updateState) error {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	b, err := yaml.Marshal(state)
	if err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, updateStateFilename+".*.tmp")
	if err != nil {
		return err
	}
	tmp := f.Name()
	_, werr := f.Write(b)
	cerr := f.Close()
	if werr != nil || cerr != nil {
		_ = os.Remove(tmp)
		return fmt.Errorf("fail to write update state")
	}
	if err := os.Rename(tmp, filepath.Join(dir, updateStateFilename)); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

func fetchLatestVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, updateProxyURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", "gofar/"+GetVersion())
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	var info struct {
		Version string `json:"Version"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 64*1024)).Decode(&info); err != nil {
		return "", err
	}
	if _, ok := parseSemver(info.Version); !ok {
		return "", fmt.Errorf("invalid version %q", info.Version)
	}
	return info.Version, nil
}

type semver struct {
	core [3]int
	pre  string
}

// parseSemver 는 vMAJOR.MINOR.PATCH[-pre][+build] 형식을 파싱한다
func parseSemver(v string) (semver, bool) {
	var s semver
	if !strings.HasPrefix(v, "v") {
		return s, false
	}
	v = v[1:]
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	if i := strings.IndexByte(v, '-'); i >= 0 {
		s.pre = v[i+1:]
		v = v[:i]
	}
	parts := strings.Split(v, ".")
	if len(parts) != 3 {
		return s, false
	}
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return s, false
		}
		s.core[i] = n
	}
	return s, true
}

// isNewerVersion 은 latest 가 current 보다 높은 버전이면 true 를 반환한다. 파싱할 수 없으면 false
func isNewerVersion(latest, current string) bool {
	l, ok := parseSemver(latest)
	if !ok {
		return false
	}
	c, ok := parseSemver(current)
	if !ok {
		return false
	}
	for i := 0; i < 3; i++ {
		if l.core[i] != c.core[i] {
			return l.core[i] > c.core[i]
		}
	}
	// 코어가 같으면 정식 버전이 pre-release(로컬 빌드의 pseudo-version 포함) 보다 높다
	if l.pre == "" || c.pre == "" {
		return l.pre == "" && c.pre != ""
	}
	return l.pre > c.pre
}

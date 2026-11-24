package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestIsNewerVersion(t *testing.T) {
	cases := []struct {
		latest, current string
		want            bool
	}{
		{"v2.9.0", "v2.8.1", true},
		{"v3.0.0", "v2.99.99", true},
		{"v2.8.10", "v2.8.9", true},
		{"v2.8.1", "v2.8.1", false},
		{"v2.8.0", "v2.8.1", false},
		{"v2.8.1", "v2.8.2-0.20261124000000-abcdef123456", false}, // 로컬 빌드가 더 앞섬
		{"v2.8.2", "v2.8.2-0.20261124000000-abcdef123456", true},
		{"v2.8.2", "v2.8.2+dirty", false},
		{"", "v2.8.1", false},
		{"v2.9.0", "(devel)", false},
		{"garbage", "v2.8.1", false},
	}
	for _, c := range cases {
		assert.Equal(t, c.want, isNewerVersion(c.latest, c.current), "%s vs %s", c.latest, c.current)
	}
}

func TestNeedsUpdateCheck(t *testing.T) {
	loc := time.FixedZone("KST", 9*3600)
	now := time.Date(2026, 11, 24, 14, 0, 0, 0, loc)

	assert.True(t, needsUpdateCheck(nil, now))
	assert.True(t, needsUpdateCheck(&updateState{}, now))
	assert.False(t, needsUpdateCheck(&updateState{Checked: time.Date(2026, 11, 24, 0, 0, 1, 0, loc)}, now))
	assert.True(t, needsUpdateCheck(&updateState{Checked: time.Date(2026, 11, 23, 23, 59, 59, 0, loc)}, now))
	// UTC 로 기록되어 있어도 로컬 날짜로 판단 (UTC 11/23 16:00 = KST 11/24 01:00)
	assert.False(t, needsUpdateCheck(&updateState{Checked: time.Date(2026, 11, 23, 16, 0, 0, 0, time.UTC)}, now))
	// 미래 시각은 재조회
	assert.True(t, needsUpdateCheck(&updateState{Checked: now.Add(time.Hour)}, now))
}

func newTestChecker(t *testing.T, now time.Time, latest string, fetchErr error) (*UpdateChecker, *int) {
	calls := 0
	return &UpdateChecker{
		dir:     filepath.Join(t.TempDir(), "fatima-test"),
		current: "v2.8.1",
		now:     func() time.Time { return now },
		fetch: func(ctx context.Context) (string, error) {
			calls++
			return latest, fetchErr
		},
	}, &calls
}

func waitDone(t *testing.T, u *UpdateChecker) {
	select {
	case <-u.done:
	case <-time.After(time.Second):
		t.Fatal("update check not finished")
	}
}

func TestUpdateCheckerOncePerDay(t *testing.T) {
	now := time.Date(2026, 11, 24, 14, 32, 7, 0, time.Local)
	u, calls := newTestChecker(t, now, "v2.9.0", nil)

	u.start()
	waitDone(t, u)
	assert.Equal(t, 1, *calls)

	state := readUpdateState(filepath.Join(u.dir, updateStateFilename))
	require.NotNil(t, state)
	assert.True(t, now.Equal(state.Checked))
	assert.Equal(t, "v2.9.0", state.Latest)
	assert.Equal(t, "v2.8.1", state.Current)

	var out bytes.Buffer
	u.Notify(&out)
	assert.Contains(t, out.String(), "v2.9.0")
	assert.Contains(t, out.String(), "go install github.com/fatima-go/gofar/v2@latest")

	// 같은 날 재실행은 조회하지 않고 기록된 결과로 안내
	u.start()
	waitDone(t, u)
	assert.Equal(t, 1, *calls)
	out.Reset()
	u.Notify(&out)
	assert.Contains(t, out.String(), "v2.9.0")

	// 다음 날은 다시 조회
	u.now = func() time.Time { return now.Add(24 * time.Hour) }
	u.start()
	waitDone(t, u)
	assert.Equal(t, 2, *calls)
}

func TestUpdateCheckerUpgradedSameDay(t *testing.T) {
	now := time.Date(2026, 11, 24, 14, 0, 0, 0, time.Local)
	u, _ := newTestChecker(t, now, "v2.9.0", nil)
	u.start()
	waitDone(t, u)

	// 당일 업그레이드 후에는 기록된 current 와 무관하게 안내하지 않는다
	u.current = "v2.9.0"
	u.start()
	waitDone(t, u)
	var out bytes.Buffer
	u.Notify(&out)
	assert.Empty(t, out.String())
}

func TestUpdateCheckerFetchFailure(t *testing.T) {
	now := time.Date(2026, 11, 24, 14, 0, 0, 0, time.Local)
	u, calls := newTestChecker(t, now, "", errors.New("network down"))
	u.start()
	waitDone(t, u)

	var out bytes.Buffer
	u.Notify(&out)
	assert.Empty(t, out.String())

	// 실패도 기록되어 당일 재조회하지 않는다
	state := readUpdateState(filepath.Join(u.dir, updateStateFilename))
	require.NotNil(t, state)
	assert.Equal(t, "", state.Latest)
	u.start()
	waitDone(t, u)
	assert.Equal(t, 1, *calls)
}

func TestUpdateCheckerNotifyDoesNotWait(t *testing.T) {
	now := time.Date(2026, 11, 24, 14, 0, 0, 0, time.Local)
	release := make(chan struct{})
	u, _ := newTestChecker(t, now, "v2.9.0", nil)
	u.fetch = func(ctx context.Context) (string, error) {
		<-release
		return "v2.9.0", nil
	}
	u.start()

	var out bytes.Buffer
	start := time.Now()
	u.Notify(&out)
	assert.Less(t, time.Since(start), 100*time.Millisecond)
	assert.Empty(t, out.String())

	close(release)
	waitDone(t, u)
}

func TestUpdateCheckerCorruptedState(t *testing.T) {
	now := time.Date(2026, 11, 24, 14, 0, 0, 0, time.Local)
	u, calls := newTestChecker(t, now, "v2.9.0", nil)
	require.NoError(t, os.MkdirAll(u.dir, 0700))
	require.NoError(t, os.WriteFile(filepath.Join(u.dir, updateStateFilename), []byte("::not yaml"), 0600))

	u.start()
	waitDone(t, u)
	assert.Equal(t, 1, *calls)
}

func TestStartUpdateCheckDisabled(t *testing.T) {
	t.Setenv(updateDisableEnvName, "1")
	assert.Nil(t, StartUpdateCheck())
	var u *UpdateChecker
	u.Notify(&bytes.Buffer{}) // nil 이어도 안전
}

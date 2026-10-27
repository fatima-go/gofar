package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var knowledgeBinaries = []string{
	"guide-bundle", "knowcli", "knowledge-backfill", "knowledge-diagram", "knowledge-git-credential",
	"knowledge-query", "knowledge-scope-review", "knowledge-verify", "knowledgesvc",
}

const knowledgeConfig = `---
build:
  exclude: [knowcli, guide-bundle, knowledge-verify]
  strip: tools
install:
  exclude: [knowledge-backfill, knowledge-scope-review]
---
fatima:
  profile: local
install:
  include: [knowledge-backfill, knowledge-scope-review]
`

func TestResolveKnowledgeConfig(t *testing.T) {
	config, err := parseProjectConfig([]byte(knowledgeConfig))
	require.NoError(t, err)

	plan, err := config.Resolve(knowledgeBinaries, "knowledgesvc", false)
	require.NoError(t, err)
	assert.Empty(t, plan.Warnings)
	assert.Equal(t, []string{"guide-bundle", "knowcli", "knowledge-verify"}, plan.Excluded)
	assert.Len(t, plan.Specs, 6)

	assert.Equal(t, BinarySpec{Role: BinaryRoleProcess}, plan.Specs["knowledgesvc"])
	for _, name := range []string{"knowledge-query", "knowledge-diagram", "knowledge-git-credential"} {
		assert.Equal(t, BinarySpec{Role: BinaryRoleTool, Strip: true}, plan.Specs[name], name)
	}
	localOnly := &InstallRule{Default: false, Profiles: map[string]bool{"local": true}}
	for _, name := range []string{"knowledge-backfill", "knowledge-scope-review"} {
		assert.Equal(t, BinarySpec{Role: BinaryRoleTool, Strip: true, Install: localOnly}, plan.Specs[name], name)
	}

	b, err := json.Marshal(plan.Specs["knowledge-backfill"])
	require.NoError(t, err)
	assert.JSONEq(t, `{"role":"tool","strip":true,"install":{"default":false,"profiles":{"local":true}}}`, string(b))
	b, err = json.Marshal(plan.Specs["knowledgesvc"])
	require.NoError(t, err)
	assert.JSONEq(t, `{"role":"process"}`, string(b))
}

func TestResolveStrip(t *testing.T) {
	names := []string{"svc", "tool"}
	cases := []struct {
		strip     string
		force     bool
		svc, tool bool
	}{
		{"", false, false, false},
		{"none", false, false, false},
		{"tools", false, false, true},
		{"all", false, true, true},
		{"none", true, true, true},
		{"tools", true, true, true},
	}
	for _, c := range cases {
		config, err := parseProjectConfig([]byte("build:\n  strip: " + c.strip + "\n"))
		require.NoError(t, err)
		plan, err := config.Resolve(names, "svc", c.force)
		require.NoError(t, err)
		assert.Equal(t, c.svc, plan.Specs["svc"].Strip, "strip=%q force=%v svc", c.strip, c.force)
		assert.Equal(t, c.tool, plan.Specs["tool"].Strip, "strip=%q force=%v tool", c.strip, c.force)
	}
}

func TestResolveProfileExclude(t *testing.T) {
	config, err := parseProjectConfig([]byte(`
fatima:
  profile: prod
install:
  exclude: [debug-tool]
`))
	require.NoError(t, err)
	plan, err := config.Resolve([]string{"svc", "debug-tool"}, "svc", false)
	require.NoError(t, err)
	assert.Equal(t, &InstallRule{Default: true, Profiles: map[string]bool{"prod": false}}, plan.Specs["debug-tool"].Install)
	assert.Nil(t, plan.Specs["svc"].Install)
}

func TestResolveRedundantInclude(t *testing.T) {
	// 기본 설치 대상을 profile 에서 다시 include 해도 규칙은 생기지 않는다
	config, err := parseProjectConfig([]byte("---\nfatima:\n  profile: local\ninstall:\n  include: [tool]\n"))
	require.NoError(t, err)
	plan, err := config.Resolve([]string{"svc", "tool"}, "svc", false)
	require.NoError(t, err)
	assert.Nil(t, plan.Specs["tool"].Install)
}

func TestResolveWarnsUnusedInstallExclude(t *testing.T) {
	config, err := parseProjectConfig([]byte("install:\n  exclude: [tool]\n"))
	require.NoError(t, err)
	plan, err := config.Resolve([]string{"svc", "tool"}, "svc", false)
	require.NoError(t, err)
	require.Len(t, plan.Warnings, 1)
	assert.Contains(t, plan.Warnings[0], "tool")
	assert.Contains(t, plan.Warnings[0], "build.exclude")
}

func TestResolveErrors(t *testing.T) {
	cases := []struct {
		name   string
		config string
		want   string
	}{
		{"unknown binary", "build:\n  exclude: [guide-bundfle]\n", "unknown binary guide-bundfle"},
		{"unknown binary in profile", "fatima:\n  profile: local\ninstall:\n  include: [nope]\n", "unknown binary nope"},
		{"build exclude process", "build:\n  exclude: [svc]\n", "cannot be excluded from build"},
		{"install exclude process", "install:\n  exclude: [svc]\n", "cannot be excluded from install"},
		{"profile exclude process", "fatima:\n  profile: qa\ninstall:\n  exclude: [svc]\n", "cannot be excluded from install"},
		{"install excluded build binary", "---\nbuild:\n  exclude: [tool]\n---\nfatima:\n  profile: local\ninstall:\n  include: [tool]\n", "already excluded from build"},
		{"only process remains excluded", "build:\n  exclude: [tool]\ninstall:\n  exclude: [tool]\n", "already excluded from build"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			config, err := parseProjectConfig([]byte(c.config))
			require.NoError(t, err)
			_, err = config.Resolve([]string{"svc", "tool"}, "svc", false)
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.want)
		})
	}
}

func TestParseErrors(t *testing.T) {
	cases := []struct {
		name   string
		config string
		want   string
	}{
		{"unknown strip", "build:\n  strip: some\n", "unknown build.strip"},
		{"build in profile", "fatima:\n  profile: local\nbuild:\n  strip: all\n", "build is not allowed in profile"},
		{"duplicated profile", "---\nfatima:\n  profile: qa\n---\nfatima:\n  profile: qa\n", "duplicated profile"},
		{"empty profile", "fatima:\n  profile: \"\"\n", "fatima.profile is empty"},
		{"missing profile", "fatima: {}\n", "fatima.profile is empty"},
		{"two common documents", "---\nbuild:\n  strip: all\n---\ninstall:\n  exclude: [a]\n", "only one common document"},
		{"include in common document", "install:\n  include: [a]\n", "install.include is only allowed in profile"},
		{"include and exclude", "fatima:\n  profile: qa\ninstall:\n  include: [a]\n  exclude: [a]\n", "both included and excluded"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			_, err := parseProjectConfig([]byte(c.config))
			require.Error(t, err)
			assert.Contains(t, err.Error(), c.want)
		})
	}
}

func TestParseIgnoresUnknownFields(t *testing.T) {
	// 상위 버전 gofar 의 설정이나 오타는 무시하고 경고한다
	config, err := parseProjectConfig([]byte("---\nbuild:\n  exclud: [a]\n  strip: tools\n  future: {x: 1}\nversion: 3\n---\nfatima:\n  profile: qa\n  zone: kr\ninstall:\n  include: [b]\n"))
	require.NoError(t, err)
	assert.Equal(t, StripTools, config.Build.Strip)
	assert.Empty(t, config.Build.Exclude)
	assert.Equal(t, []string{"b"}, config.Profiles["qa"].Include)
	require.Len(t, config.Warnings, 4)
	assert.Contains(t, config.Warnings[0], "document 1 line 3 : unknown field build.exclud is ignored")
	assert.Contains(t, config.Warnings[1], "document 1 line 5 : unknown field build.future is ignored")
	assert.Contains(t, config.Warnings[2], "document 1 line 6 : unknown field version is ignored")
	assert.Contains(t, config.Warnings[3], "document 2 line 10 : unknown field fatima.zone is ignored")

	config, err = parseProjectConfig([]byte(knowledgeConfig))
	require.NoError(t, err)
	assert.Empty(t, config.Warnings)

	// 알려진 키의 값이 잘못된 경우는 여전히 에러
	_, err = parseProjectConfig([]byte("build:\n  cgo: maybe\n"))
	assert.Error(t, err)
}

func TestLoadProjectConfigUnknownFieldWarning(t *testing.T) {
	base := t.TempDir()
	path := writeProjectConfig(t, base, "svc.yaml", "build:\n  strip: tools\n  future: true\n")
	config, warnings, err := loadProjectConfig(base, "svc")
	require.NoError(t, err)
	assert.Equal(t, StripTools, config.Build.Strip)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], path+" : document 1 line 3 : unknown field build.future is ignored")
}

func TestParseEmptyDocuments(t *testing.T) {
	config, err := parseProjectConfig([]byte("---\n# comment only\n---\nbuild:\n  strip: tools\n---\n"))
	require.NoError(t, err)
	assert.Equal(t, StripTools, config.Build.Strip)
	assert.Empty(t, config.Profiles)
}

func writeProjectConfig(t *testing.T, base, name, content string) string {
	t.Helper()
	require.NoError(t, os.MkdirAll(filepath.Join(base, ProjectConfigDir), 0755))
	path := filepath.Join(base, ProjectConfigDir, name)
	require.NoError(t, os.WriteFile(path, []byte(content), 0644))
	return path
}

func TestLoadProjectConfig(t *testing.T) {
	base := t.TempDir()
	config, warnings, err := loadProjectConfig(base, "knowledgesvc")
	require.NoError(t, err)
	assert.Nil(t, config, "no config file")
	assert.Empty(t, warnings)

	// 공통 설정만 있으면 공통 설정을 사용한다
	common := writeProjectConfig(t, base, ProjectConfigFile, knowledgeConfig)
	config, warnings, err = loadProjectConfig(base, "knowledgesvc")
	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, common, config.Path)
	assert.Contains(t, config.Profiles, "local")
	assert.Empty(t, warnings)

	// process 설정이 있으면 공통 설정보다 우선한다 (병합하지 않는다)
	own := writeProjectConfig(t, base, "knowledgesvc.yaml", "build:\n  strip: all\n")
	config, warnings, err = loadProjectConfig(base, "knowledgesvc")
	require.NoError(t, err)
	assert.Equal(t, own, config.Path)
	assert.Equal(t, StripAll, config.Build.Strip)
	assert.Empty(t, config.Profiles)
	assert.Empty(t, warnings)

	// 다른 process 설정만 있으면 공통 설정을 사용하되 경고한다
	config, warnings, err = loadProjectConfig(base, "other")
	require.NoError(t, err)
	assert.Equal(t, common, config.Path)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "other.yaml not found")
	assert.Contains(t, warnings[0], "knowledgesvc.yaml")

	// 공통 설정도 없으면 설정 없이 빌드하되 경고한다
	require.NoError(t, os.Remove(common))
	config, warnings, err = loadProjectConfig(base, "other")
	require.NoError(t, err)
	assert.Nil(t, config)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "build without project config")
}

func TestLoadProjectConfigIgnoresUnsafeProcessName(t *testing.T) {
	base := t.TempDir()
	writeProjectConfig(t, base, ProjectConfigFile, "build:\n  strip: tools\n")
	require.NoError(t, os.WriteFile(filepath.Join(base, "evil.yaml"), []byte("build:\n  strip: all\n"), 0644))
	for _, name := range []string{"../evil", ".hidden", "a/b"} {
		config, _, err := loadProjectConfig(base, name)
		require.NoError(t, err)
		assert.Equal(t, StripTools, config.Build.Strip, name)
	}
}

func TestLoadProjectConfigInvalidProcessFile(t *testing.T) {
	base := t.TempDir()
	writeProjectConfig(t, base, ProjectConfigFile, "build:\n  strip: tools\n")
	writeProjectConfig(t, base, "svc.yaml", "build:\n  strip: wrong\n")
	_, _, err := loadProjectConfig(base, "svc")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "svc.yaml")
}

func TestResolveWarnsMissingProcessBinary(t *testing.T) {
	config, err := parseProjectConfig([]byte("build:\n  strip: tools\n"))
	require.NoError(t, err)
	plan, err := config.Resolve([]string{"tool-a", "tool-b"}, "svc", false)
	require.NoError(t, err)
	require.Len(t, plan.Warnings, 1)
	assert.Contains(t, plan.Warnings[0], "process binary svc not found")
	assert.True(t, plan.Specs["tool-a"].Strip)
}

func TestApplyProjectConfigPerProcess(t *testing.T) {
	// 한 프로젝트에 process 가 2개인 경우 각자의 설정으로 서로를 제외한다
	base := t.TempDir()
	names := []string{"svc-a", "svc-b", "shared-tool"}
	writeProjectConfig(t, base, "svc-a.yaml", "build:\n  exclude: [svc-b]\n  strip: tools\n")
	writeProjectConfig(t, base, "svc-b.yaml", "build:\n  exclude: [svc-a]\n")

	build := func(process string) *BuildContext {
		ctx := &BuildContext{ProjectBaseDir: base, ExposeProcessName: process}
		for _, name := range names {
			ctx.ProcessList = append(ctx.ProcessList, CmdRecord{Path: filepath.Join(base, "cmd", name)})
		}
		require.NoError(t, applyProjectConfig(ctx))
		return ctx
	}

	a := build("svc-a")
	assert.Equal(t, []string{"svc-b"}, a.ExcludedBinaries)
	assert.Equal(t, BinaryRoleProcess, a.binarySpecs["svc-a"].Role)
	assert.True(t, a.isStripBinary("shared-tool"))

	b := build("svc-b")
	assert.Equal(t, []string{"svc-a"}, b.ExcludedBinaries)
	assert.Equal(t, BinaryRoleProcess, b.binarySpecs["svc-b"].Role)
	assert.False(t, b.isStripBinary("shared-tool"))
}

func TestApplyProjectConfig(t *testing.T) {
	base := t.TempDir()
	ctx := &BuildContext{ProjectBaseDir: base, ExposeProcessName: "knowledgesvc"}
	for _, name := range knowledgeBinaries {
		ctx.ProcessList = append(ctx.ProcessList, CmdRecord{Path: filepath.Join(base, "cmd", name)})
	}

	// 설정 파일이 없으면 기존 동작 그대로
	require.NoError(t, applyProjectConfig(ctx))
	assert.Nil(t, ctx.ProjectConfig)
	assert.Nil(t, ctx.binarySpecs)
	assert.Len(t, ctx.ProcessList, len(knowledgeBinaries))
	stripEnable = true
	assert.True(t, ctx.isStripBinary("knowledgesvc"))
	stripEnable = false
	assert.False(t, ctx.isStripBinary("knowledge-query"))

	require.NoError(t, os.MkdirAll(filepath.Join(base, ProjectConfigDir), 0755))
	require.NoError(t, os.WriteFile(filepath.Join(base, ProjectConfigDir, ProjectConfigFile), []byte(knowledgeConfig), 0644))
	require.NoError(t, applyProjectConfig(ctx))
	require.NotNil(t, ctx.ProjectConfig)
	assert.Len(t, ctx.ProcessList, 6)
	for _, record := range ctx.ProcessList {
		assert.NotContains(t, ctx.ExcludedBinaries, record.GetBinaryname())
	}
	assert.False(t, ctx.isStripBinary("knowledgesvc"))
	assert.True(t, ctx.isStripBinary("knowledge-query"))
}

func TestCgoEnabled(t *testing.T) {
	defer func() { cgoEnable = false }()

	// 프로젝트 설정이 없으면 -c 옵션을 따른다
	ctx := BuildContext{}
	assert.False(t, ctx.isCgoEnabled())
	cgoEnable = true
	assert.True(t, ctx.isCgoEnabled())
	cgoEnable = false

	config, err := parseProjectConfig([]byte("build:\n  cgo: true\n"))
	require.NoError(t, err)
	ctx.ProjectConfig = config
	assert.True(t, ctx.isCgoEnabled())

	config, err = parseProjectConfig([]byte("build:\n  strip: tools\n"))
	require.NoError(t, err)
	ctx.ProjectConfig = config
	assert.False(t, ctx.isCgoEnabled())
	cgoEnable = true
	assert.True(t, ctx.isCgoEnabled())
}

func TestParseCgoOnlyInCommonDocument(t *testing.T) {
	_, err := parseProjectConfig([]byte("---\nfatima:\n  profile: local\nbuild:\n  cgo: true\n"))
	assert.ErrorContains(t, err, "build is not allowed in profile(local) document")
}

func TestCgoCrossCompileWarnings(t *testing.T) {
	platforms := []PlatformItem{
		{Os: "linux", Arch: "amd64", CC: "x86_64-pc-linux-gcc"},
		{Os: "linux", Arch: "arm64"},
	}

	config, err := parseProjectConfig([]byte("build:\n  cgo: true\n"))
	require.NoError(t, err)
	ctx := BuildContext{ProjectConfig: config}
	warnings := ctx.cgoCrossCompileWarnings(platforms)
	require.Len(t, warnings, 1)
	assert.Contains(t, warnings[0], "linux_arm64 has no cc")

	// cgo 를 쓰지 않으면 경고하지 않는다
	assert.Empty(t, BuildContext{}.cgoCrossCompileWarnings(platforms))
}

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
		{"unknown key", "build:\n  exclud: [a]\n", "exclud"},
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

func TestParseEmptyDocuments(t *testing.T) {
	config, err := parseProjectConfig([]byte("---\n# comment only\n---\nbuild:\n  strip: tools\n---\n"))
	require.NoError(t, err)
	assert.Equal(t, StripTools, config.Build.Strip)
	assert.Empty(t, config.Profiles)
}

func TestLoadProjectConfig(t *testing.T) {
	base := t.TempDir()
	config, err := loadProjectConfig(base)
	require.NoError(t, err)
	assert.Nil(t, config, "no config file")

	require.NoError(t, os.MkdirAll(filepath.Join(base, ProjectConfigDir), 0755))
	path := filepath.Join(base, ProjectConfigDir, ProjectConfigFile)
	require.NoError(t, os.WriteFile(path, []byte(knowledgeConfig), 0644))
	config, err = loadProjectConfig(base)
	require.NoError(t, err)
	require.NotNil(t, config)
	assert.Equal(t, path, config.Path)
	assert.Contains(t, config.Profiles, "local")
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

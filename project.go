/*
 * Copyright 2023 github.com/fatima-go
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
 * @project fatima-go
 * @author dave
 * @date 26. 10. 25. 오후 2:00
 */

package main

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

const (
	ProjectConfigDir  = ".gofar"
	ProjectConfigFile = "gofar.yaml"

	StripNone  = "none"
	StripTools = "tools"
	StripAll   = "all"

	BinaryRoleProcess = "process"
	BinaryRoleTool    = "tool"
)

/*
.gofar/gofar.yaml 은 공통 문서 1개와 profile 문서 N개로 구성된 multi-doc yaml 이다.
빌드는 profile 과 무관하게 1번만 수행되므로 build 는 공통 문서에만 둘 수 있다.

---
build:

	exclude: [knowcli]          # 빌드도, far 포함도 하지 않는다
	strip: tools                # none(기본) | tools(process 외 바이너리) | all

install:

	exclude: [knowledge-backfill] # far 에는 포함하지만 기본적으로 설치하지 않는다

---
fatima:

	profile: local

install:

	include: [knowledge-backfill] # local profile 에서는 설치한다
*/
type projectDocument struct {
	Fatima  *projectFatima  `yaml:"fatima"`
	Build   *projectBuild   `yaml:"build"`
	Install *projectInstall `yaml:"install"`
}

type projectFatima struct {
	Profile *string `yaml:"profile"`
}

type projectBuild struct {
	Exclude []string `yaml:"exclude"`
	Strip   string   `yaml:"strip"`
}

type projectInstall struct {
	Include []string `yaml:"include"`
	Exclude []string `yaml:"exclude"`
}

// ProjectConfig 프로젝트별 gofar 설정
type ProjectConfig struct {
	Path     string
	Build    projectBuild
	Install  projectInstall
	Profiles map[string]projectInstall
}

// BinarySpec deployment.json 의 binaries 항목. 배포 클라이언트는 install 이 없으면 항상 설치하고,
// 있으면 profiles[배포 대상 profile] 을, 해당 profile 이 없으면 default 를 따른다
type BinarySpec struct {
	Role    string       `json:"role"`
	Strip   bool         `json:"strip,omitempty"`
	Install *InstallRule `json:"install,omitempty"`
}

type InstallRule struct {
	Default  bool            `json:"default"`
	Profiles map[string]bool `json:"profiles,omitempty"`
}

// BinaryPlan 프로젝트 설정을 실제 바이너리 목록에 적용한 결과
type BinaryPlan struct {
	Excluded []string
	Specs    map[string]BinarySpec
	Warnings []string
}

// loadProjectConfig 프로젝트의 .gofar/gofar.yaml 을 로드한다. 파일이 없으면 nil 을 리턴한다
func loadProjectConfig(projectBaseDir string) (*ProjectConfig, error) {
	path := filepath.Join(projectBaseDir, ProjectConfigDir, ProjectConfigFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("fail to read %s : %s", path, err.Error())
	}

	config, err := parseProjectConfig(data)
	if err != nil {
		return nil, fmt.Errorf("invalid %s : %s", path, err.Error())
	}
	config.Path = path
	return config, nil
}

func parseProjectConfig(data []byte) (*ProjectConfig, error) {
	config := &ProjectConfig{Profiles: make(map[string]projectInstall)}
	baseFound := false

	decoder := yaml.NewDecoder(strings.NewReader(string(data)))
	decoder.KnownFields(true)
	for index := 1; ; index++ {
		var doc projectDocument
		err := decoder.Decode(&doc)
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("document %d : %s", index, err.Error())
		}

		if doc.Fatima == nil && doc.Build == nil && doc.Install == nil {
			// 비어 있는 문서
			continue
		}

		if doc.Fatima == nil {
			if baseFound {
				return nil, fmt.Errorf("document %d : only one common document (without fatima.profile) is allowed", index)
			}
			baseFound = true
			if doc.Build != nil {
				config.Build = *doc.Build
			}
			if doc.Install != nil {
				if len(doc.Install.Include) > 0 {
					return nil, fmt.Errorf("document %d : install.include is only allowed in profile document", index)
				}
				config.Install = *doc.Install
			}
			continue
		}

		if doc.Fatima.Profile == nil || len(strings.TrimSpace(*doc.Fatima.Profile)) == 0 {
			return nil, fmt.Errorf("document %d : fatima.profile is empty", index)
		}
		profile := strings.TrimSpace(*doc.Fatima.Profile)
		if doc.Build != nil {
			return nil, fmt.Errorf("document %d : build is not allowed in profile(%s) document. build is profile independent", index, profile)
		}
		if _, ok := config.Profiles[profile]; ok {
			return nil, fmt.Errorf("document %d : duplicated profile document %s", index, profile)
		}
		install := projectInstall{}
		if doc.Install != nil {
			install = *doc.Install
		}
		for _, name := range install.Include {
			if containsString(install.Exclude, name) {
				return nil, fmt.Errorf("document %d : %s is both included and excluded in profile %s", index, name, profile)
			}
		}
		config.Profiles[profile] = install
	}

	switch config.Build.Strip {
	case "":
		config.Build.Strip = StripNone
	case StripNone, StripTools, StripAll:
	default:
		return nil, fmt.Errorf("unknown build.strip value %q (none, tools, all)", config.Build.Strip)
	}

	return config, nil
}

// Resolve 프로젝트 설정을 바이너리 목록에 적용한다. forceStrip 은 -s 옵션으로 모든 바이너리를 strip 한다
func (p *ProjectConfig) Resolve(binaryNames []string, processName string, forceStrip bool) (*BinaryPlan, error) {
	known := make(map[string]bool)
	for _, name := range binaryNames {
		known[name] = true
	}

	if err := p.checkUnknownBinaries(known, binaryNames); err != nil {
		return nil, err
	}

	buildExcluded := make(map[string]bool)
	for _, name := range p.Build.Exclude {
		if name == processName {
			return nil, fmt.Errorf("process binary %s cannot be excluded from build", processName)
		}
		buildExcluded[name] = true
	}

	profileNames := p.sortedProfileNames()
	if err := p.checkInstallTargets(buildExcluded, processName, profileNames); err != nil {
		return nil, err
	}

	plan := &BinaryPlan{Specs: make(map[string]BinarySpec)}
	for _, name := range binaryNames {
		if buildExcluded[name] {
			plan.Excluded = append(plan.Excluded, name)
			continue
		}

		spec := BinarySpec{Role: BinaryRoleTool}
		if name == processName {
			spec.Role = BinaryRoleProcess
		}
		spec.Strip = forceStrip || p.Build.Strip == StripAll || (p.Build.Strip == StripTools && spec.Role == BinaryRoleTool)
		spec.Install = p.buildInstallRule(name, profileNames)
		if spec.Install != nil && !spec.Install.Default && len(spec.Install.Profiles) == 0 {
			plan.Warnings = append(plan.Warnings,
				fmt.Sprintf("%s is excluded by install.exclude but not included in any profile. move it to build.exclude", name))
		}
		plan.Specs[name] = spec
	}

	if len(plan.Specs) == 0 {
		return nil, fmt.Errorf("there is no binary to build")
	}

	return plan, nil
}

func (p *ProjectConfig) checkUnknownBinaries(known map[string]bool, binaryNames []string) error {
	referenced := make([]string, 0)
	referenced = append(referenced, p.Build.Exclude...)
	referenced = append(referenced, p.Install.Exclude...)
	for _, install := range p.Profiles {
		referenced = append(referenced, install.Include...)
		referenced = append(referenced, install.Exclude...)
	}

	unknown := make([]string, 0)
	for _, name := range referenced {
		if !known[name] && !containsString(unknown, name) {
			unknown = append(unknown, name)
		}
	}
	if len(unknown) == 0 {
		return nil
	}

	sort.Strings(unknown)
	available := append([]string{}, binaryNames...)
	sort.Strings(available)
	return fmt.Errorf("unknown binary %s (available : %s)", strings.Join(unknown, ","), strings.Join(available, ","))
}

func (p *ProjectConfig) checkInstallTargets(buildExcluded map[string]bool, processName string, profileNames []string) error {
	check := func(where string, names []string, exclude bool) error {
		for _, name := range names {
			if buildExcluded[name] {
				return fmt.Errorf("%s : %s is already excluded from build", where, name)
			}
			if exclude && name == processName {
				return fmt.Errorf("%s : process binary %s cannot be excluded from install", where, name)
			}
		}
		return nil
	}

	if err := check("install.exclude", p.Install.Exclude, true); err != nil {
		return err
	}
	for _, profile := range profileNames {
		install := p.Profiles[profile]
		if err := check(fmt.Sprintf("profile %s install.include", profile), install.Include, false); err != nil {
			return err
		}
		if err := check(fmt.Sprintf("profile %s install.exclude", profile), install.Exclude, true); err != nil {
			return err
		}
	}
	return nil
}

// buildInstallRule 바이너리의 설치 규칙을 구한다. 모든 profile 에 설치되면 nil 을 리턴한다
func (p *ProjectConfig) buildInstallRule(name string, profileNames []string) *InstallRule {
	rule := &InstallRule{Default: !containsString(p.Install.Exclude, name), Profiles: make(map[string]bool)}
	for _, profile := range profileNames {
		install := p.Profiles[profile]
		if !rule.Default && containsString(install.Include, name) {
			rule.Profiles[profile] = true
		}
		if rule.Default && containsString(install.Exclude, name) {
			rule.Profiles[profile] = false
		}
	}

	if rule.Default && len(rule.Profiles) == 0 {
		return nil
	}
	return rule
}

func (p *ProjectConfig) sortedProfileNames() []string {
	names := make([]string, 0, len(p.Profiles))
	for name := range p.Profiles {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

func containsString(list []string, value string) bool {
	for _, v := range list {
		if v == value {
			return true
		}
	}
	return false
}

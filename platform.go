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
 * @author dave_01
 * @date 23. 9. 5. 오전 10:33
 */

package main

import (
	"bytes"
	"errors"
	"fmt"
	"gopkg.in/yaml.v3"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

const (
	ConfigDir          = ".fatima"
	ConfigPlatformFile = "gofar.yaml"
)

var buildPlatformList YamlBuildPlatformConfig

// loadPlatform 빌드 타겟 플랙폼 정보를 로드한다
// 기본적으로 $HOME/.gofar/platform.yaml 파일로 관리한다
func loadPlatform() {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		panic(fmt.Errorf("not found user home directory"))
	}

	yamlFilePath := filepath.Join(homeDir, ConfigDir, ConfigPlatformFile)
	data, err := os.ReadFile(yamlFilePath)
	if err != nil {
		prepareDefaultPlatformFile()
		// try once again
		data, err = os.ReadFile(yamlFilePath)
		if err != nil {
			panic(fmt.Errorf("fail to read platform config : %s", err.Error()))
		}
	}

	err = yaml.Unmarshal(data, &buildPlatformList)
	if err != nil {
		panic(fmt.Errorf("invalid gofar platform yaml file : %s", err.Error()))
	}
}

// prepareDefaultPlatformFile 기본 빌드 플랫폼 정보 파일을 생성한다
func prepareDefaultPlatformFile() {
	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ConfigDir)
	err := os.Mkdir(configDir, 0744)
	if err != nil {
		if !errors.Is(err, os.ErrExist) {
			panic(fmt.Errorf("fail to create %s dir : %s", configDir, err.Error()))
		}
	}

	platformList := newDefaultBuildPlatformConfig()

	var comment = "---\n" +
		"# if you want to check platform support list, use below command\n" +
		"# $ go tool dist list\n" +
		"# \n"

	var buff bytes.Buffer
	buff.WriteString(comment)
	yamlEncoder := yaml.NewEncoder(&buff)
	yamlEncoder.SetIndent(2)
	_ = yamlEncoder.Encode(&platformList)
	//buff.Write(data)

	yamlFilePath := filepath.Join(configDir, ConfigPlatformFile)
	err = os.WriteFile(yamlFilePath, buff.Bytes(), 0744)
	if err != nil {
		panic(fmt.Errorf("fail to create default config yaml file : %s", err.Error()))
	}
}

/*
platform_list:
  - os : linux
    arch : amd64
  - os : linux
    arch : arm64
*/
type YamlBuildPlatformConfig struct {
	Platforms []PlatformItem `yaml:"platform_list"`
}

func (y YamlBuildPlatformConfig) GetLocalPlatform() PlatformItem {
	for _, platform := range y.Platforms {
		if platform.Os == runtime.GOOS && platform.Arch == runtime.GOARCH {
			return platform
		}
	}

	// invalid something...
	return PlatformItem{Os: runtime.GOOS, Arch: runtime.GOARCH}
}

func (y YamlBuildPlatformConfig) GetAdditionalPlatforms() []PlatformItem {
	list := make([]PlatformItem, 0)
	for _, platform := range y.Platforms {
		if platform.Os == runtime.GOOS && platform.Arch == runtime.GOARCH {
			continue
		}
		list = append(list, platform)
	}

	return list
}

type PlatformItem struct {
	Os   string `yaml:"os"`
	Arch string `yaml:"arch"`
}

// getPlatformDirectory os_arch 형태의 플랫폼 구분 디렉토리명을 구한다
func (p PlatformItem) getPlatformDirectory() string {
	return fmt.Sprintf("%s_%s", p.Os, p.Arch)
}

func newDefaultBuildPlatformConfig() YamlBuildPlatformConfig {
	config := YamlBuildPlatformConfig{}
	config.Platforms = make([]PlatformItem, 0)
	config.Platforms = append(config.Platforms, PlatformItem{Os: "linux", Arch: "amd64"})
	config.Platforms = append(config.Platforms, PlatformItem{Os: "linux", Arch: "arm64"})

	// 로컬환경을 default 로 추가하자
	if strings.Compare(runtime.GOOS, "linux") != 0 {
		config.Platforms = append(config.Platforms, PlatformItem{Os: runtime.GOOS, Arch: runtime.GOARCH})
	}

	return config
}

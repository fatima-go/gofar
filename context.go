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
 * @project fatima-core
 * @author jin
 * @date 23. 4. 14. 오후 6:11
 */

package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

const (
	resourceDirname = "resources"
	cmdDirname      = "cmd"
	gitDirname      = ".git"
	gitConfigfile   = "config"
	procTypeGeneral = "GENERAL"
	procTypeUI      = "USER_INTERACTIVE"
)

type CmdRecord struct {
	Path string
}

func (c CmdRecord) GetBinaryname() string {
	return filepath.Base(c.Path)
}

func (c CmdRecord) GetMainSourcePath() string {
	return filepath.Join(c.Path, fmt.Sprintf("%s.go", c.GetBinaryname()))
}

type BuildContext struct {
	ProjectBaseDir    string
	ResourceDir       string
	ProcessList       []CmdRecord
	GitSupport        bool
	ExposeProcessName string
	ProjectConfig     *ProjectConfig
	ExcludedBinaries  []string
	binarySpecs       map[string]BinarySpec
	workingDir        string
	procType          string
	farPath           string
}

// isStripBinary 바이너리의 strip 여부를 구한다. 프로젝트 설정이 없으면 -s 옵션을 따른다
func (b BuildContext) isStripBinary(binName string) bool {
	if b.binarySpecs == nil {
		return stripEnable
	}
	return b.binarySpecs[binName].Strip
}

func (b BuildContext) Print() {
	fmt.Printf("--------------------------------------------------\n")
	defer func() {
		fmt.Printf("--------------------------------------------------\n")
	}()
	fmt.Printf("project base dir : %s\n", b.ProjectBaseDir)
	fmt.Printf("resource dir : %s\n", b.ResourceDir)
	fmt.Printf("expose process name : %s\n", b.ExposeProcessName)

	if len(b.ProcessList) > 0 {
		binList := ""
		for i, v := range b.ProcessList {
			if i == 0 {
				binList = v.GetBinaryname()
			} else {
				binList = binList + "," + v.GetBinaryname()
			}
		}
		fmt.Printf("binary : %s\n", binList)
	} else {
		fmt.Printf("binary process : %s\n", b.ExposeProcessName)
	}

	if b.ProjectConfig != nil {
		fmt.Printf("project config : %s\n", b.ProjectConfig.Path)
		if len(b.ExcludedBinaries) > 0 {
			fmt.Printf("build excluded : %s\n", strings.Join(b.ExcludedBinaries, ","))
		}
		stripList := make([]string, 0)
		for _, v := range b.ProcessList {
			if b.isStripBinary(v.GetBinaryname()) {
				stripList = append(stripList, v.GetBinaryname())
			}
		}
		if len(stripList) > 0 {
			fmt.Printf("strip : %s\n", strings.Join(stripList, ","))
		}
	}
}

func (b *BuildContext) Packaging() error {
	var err error
	b.workingDir, err = os.MkdirTemp("", b.ExposeProcessName)
	if err != nil {
		return fmt.Errorf("fail to create tmp dir : %s", err.Error())
	}

	fmt.Printf("working directory : %s\n", b.workingDir)
	defer func() {
		_ = os.RemoveAll(b.workingDir)
	}()

	err = b.prepareBinary()
	if err != nil {
		return err
	}

	err = b.prepareResource()
	if err != nil {
		return err
	}

	err = b.createDeployment()
	if err != nil {
		return err
	}

	err = b.compress()
	if err != nil {
		return err
	}

	fmt.Printf("\nSUCCESS to packaging...\nArtifact :: %s\n\n", b.farPath)

	return nil
}

func getGOPath() string {
	gopath := os.Getenv("GOPATH")

	idx := strings.Index(gopath, fmt.Sprintf("%c", os.PathListSeparator))
	if idx < 0 {
		return gopath
	}

	return gopath[:idx]
}

func (b *BuildContext) compress() error {
	farDir := filepath.Join(getGOPath(), "far", b.ExposeProcessName)
	fmt.Printf("\n>> compress to %s\n", farDir)

	err := EnsureDirectory(farDir)
	if err != nil {
		return fmt.Errorf("fail to prepare far dir : %s", err.Error())
	}

	farName := fmt.Sprintf("%s.far", b.ExposeProcessName)
	b.farPath = filepath.Join(farDir, farName)
	err = ZipArtifact(b.workingDir, b.farPath)
	if err != nil {
		return fmt.Errorf("fail to compress : %s", err.Error())
	}

	return nil
}

const (
	yyyyMMddHHmmss = "2006-01-02 15:04:05"
)

// create deployment...
func (b *BuildContext) createDeployment() error {
	// create deployment.json
	m := make(map[string]interface{})
	m["process"] = b.ExposeProcessName
	m["process_type"] = b.procType

	build := make(map[string]interface{})
	zoneName, _ := time.Now().Zone()
	build["time"] = time.Now().Format(yyyyMMddHHmmss) + " " + zoneName
	// find author
	user, err := ExecuteShell(".", "whoami")
	if err != nil {
		fmt.Fprintf(os.Stderr, "whoami error : %s\n", err.Error())
		user = "unknown"
	}
	build["user"] = strings.TrimSpace(user)
	if b.GitSupport {
		gitInfo := readGitInfo(b.ProjectBaseDir)
		if gitInfo.Valid {
			build["git"] = gitInfo.ToMap()
		}
	}
	m["build"] = build
	if b.binarySpecs != nil {
		// 프로젝트 설정(.gofar/gofar.yaml)이 있을 때만 바이너리별 정책을 기록한다
		m["binaries"] = b.binarySpecs
	}
	dat, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("fail to create deployment : %s", err.Error())
	}

	depfile := filepath.Join(b.workingDir, "deployment.json")
	err = os.WriteFile(depfile, dat, 0644)
	if err != nil {
		return fmt.Errorf("fail to write deployment.json : %s", err.Error())
	}

	return nil
}

// prepare resource...
func (b *BuildContext) prepareResource() error {
	err := b.loadResourceFiles()
	if err != nil {
		return err
	}

	// determine proc type
	uiProcXml := filepath.Join(b.workingDir, fmt.Sprintf("%s.ui.xml", b.ExposeProcessName))
	err = CheckFileExist(uiProcXml)
	if err == nil {
		// exist ui xml
		b.procType = procTypeUI
	}

	return nil
}

func (b *BuildContext) loadResourceFiles() error {
	if len(b.ResourceDir) == 0 {
		return b.loadResourceFromProject()
	}
	return b.loadResourceFromDesginatedDir()
}

func (b *BuildContext) loadResourceFromDesginatedDir() error {
	fmt.Printf("\n>> copying resources...\n")
	command := fmt.Sprintf("cp -r * %s", b.workingDir)
	out, err := ExecuteShell(b.ResourceDir, command)
	if err != nil {
		return fmt.Errorf("fail to execute command : %s\n%s\n", err.Error(), out)
	}
	if len(out) > 0 {
		return fmt.Errorf("fail to copy resources\n%s\n", out)
	}
	fmt.Printf("resources directory copied...\n")
	return nil
}

var includeSuffixList = [...]string{"properties", "xml", "json", "yaml", "sh", "yml"}

func (b *BuildContext) loadResourceFromProject() error {
	resourceFileList, err := findResourceFromDirectory(b.ProjectBaseDir)
	if err != nil {
		return err
	}

	for _, resourceFilePath := range resourceFileList {
		targetFile := filepath.Join(b.workingDir, filepath.Base(resourceFilePath))
		err = CopyFile(resourceFilePath, targetFile)
		if err != nil {
			return fmt.Errorf("fail to copy resource %s : %s", resourceFilePath, err.Error())
		}
		if strings.HasSuffix(targetFile, ".sh") {
			os.Chmod(targetFile, 0755)
		}
	}

	fmt.Printf("total %d resource files copied...\n", len(resourceFileList))
	return nil
}

func findResourceFromDirectory(baseDir string) ([]string, error) {
	resourceFileList := make([]string, 0)

	files, err := os.ReadDir(baseDir)
	if err != nil {
		return resourceFileList, fmt.Errorf("findResourceFromDirectory error : %s\n", err.Error())
	}

	for _, file := range files {
		if file.Name()[0] == '.' {
			continue
		}

		if file.IsDir() {
			foundFileList, err := findResourceFromDirectory(filepath.Join(baseDir, file.Name()))
			if err != nil {
				return resourceFileList, err
			}
			resourceFileList = append(resourceFileList, foundFileList...)
			continue
		}

		for _, s := range includeSuffixList {
			if strings.HasSuffix(file.Name(), s) {
				resourceFileList = append(resourceFileList, filepath.Join(baseDir, file.Name()))
				break
			}
		}
	}
	return resourceFileList, nil
}

// prepare binaries...
func (b *BuildContext) prepareBinary() error {
	if len(b.ProcessList) == 0 {
		return fmt.Errorf("not found target process list")
	}

	return b.prepareCmdRecordBinary()
}

func (b *BuildContext) prepareCmdRecordBinary() error {
	// build process list
	for _, cmdRecord := range b.ProcessList {
		cmdBinName := cmdRecord.GetBinaryname()
		fmt.Printf("\n>> compiling %s...\n", cmdBinName)

		var compileError uint32 = 0

		// local 플랫폼을 먼저 빌드한다, 이후 에러가 없을 경우 추가 플랫폼을 빌드한다

		CgoCCLink := ""
		strip := b.isStripBinary(cmdBinName)
		compileRequest := createCompileRequest(buildPlatformList.GetLocalPlatform(), cmdRecord, b.workingDir, CgoCCLink, strip)
		compileBinary(&compileError, compileRequest)
		if compileError > 0 {
			return fmt.Errorf("fail to prepare binary %s\n", cmdBinName)
		}

		// 추가 플랫폼을 빌드한다
		wg := sync.WaitGroup{}
		additionalPlatforms := buildPlatformList.GetAdditionalPlatforms()
		wg.Add(len(additionalPlatforms))
		for _, platform := range additionalPlatforms {
			nextCompileRequest := createCompileRequest(platform, cmdRecord, b.workingDir, platform.CC, strip)
			go func() {
				defer wg.Done()
				compileBinary(&compileError, nextCompileRequest)
			}()
		}
		wg.Wait()

		if compileError > 0 {
			return fmt.Errorf("fail to prepare binary %s\n", cmdBinName)
		}
	}

	return nil
}

func createCompileRequest(platform PlatformItem, cmdRecord CmdRecord, workingDir, cgoLink string, strip bool) BinCompileRequest {
	request := BinCompileRequest{}
	request.Strip = strip
	request.TargetDir = filepath.Join(workingDir, PlatformDirName, platform.getPlatformDirectory())
	request.BinName = cmdRecord.GetBinaryname()
	request.BinSourcePath = cmdRecord.Path
	request.Os = platform.Os
	request.Arch = platform.Arch
	request.BuildCGOLink = cgoLink
	return request
}

const (
	PlatformDirName = "platform"
)

type BinCompileRequest struct {
	TargetDir     string
	BinSourcePath string
	BinName       string
	Os            string
	Arch          string
	BuildCGOLink  string
	Strip         bool
}

// compileBinary 바이너리를 컴파일한다
func compileBinary(compileError *uint32, request BinCompileRequest) {
	err := os.MkdirAll(request.TargetDir, 0744)
	if err != nil {
		if !errors.Is(err, os.ErrExist) {
			fmt.Printf("fail to prepare platform dir %s : %s\n", request.TargetDir, err.Error())
			atomic.AddUint32(compileError, 1)
			return
		}
	}

	targetBin := filepath.Join(request.TargetDir, request.BinName)
	command := fmt.Sprintf("go build -o %s", targetBin)

	command = fmt.Sprintf("GOOS=%s GOARCH=%s go build -o %s", request.Os, request.Arch, targetBin)
	if cgoEnable {
		if len(request.BuildCGOLink) > 0 {
			command = fmt.Sprintf("CC=%s %s", request.BuildCGOLink, command)
		}
		command = fmt.Sprintf("CGO_ENABLED=1 %s", command)
	}
	if request.Strip {
		command = fmt.Sprintf("%s -ldflags='-s -w'", command)
	}

	fmt.Printf("%s\n", command)
	out, err := ExecuteShell(request.BinSourcePath, command)
	if err != nil {
		fmt.Printf("fail to execute command : %s\n%s\n", err.Error(), out)
		atomic.AddUint32(compileError, 1)
		return
	}

	if len(out) > 0 {
		fmt.Printf("fail to build binary %s : %s\n%s\n", filepath.Base(request.TargetDir), request.BinName, out)
		atomic.AddUint32(compileError, 1)
		return
	}

	_ = os.Chmod(targetBin, 0755)
}

func NewBuildContext(procName string) (*BuildContext, error) {
	loadPlatform()

	ctx := &BuildContext{}
	ctx.GitSupport = false
	ctx.ExposeProcessName = procName
	ctx.procType = procTypeGeneral

	err := determineProjectBaseDir(ctx)
	if err != nil {
		return nil, fmt.Errorf("fail to build context. %s", err.Error())
	}

	determineResourceDir(ctx)
	determineCmdList(ctx)

	err = applyProjectConfig(ctx)
	if err != nil {
		return nil, fmt.Errorf("fail to apply project config. %s", err.Error())
	}

	return ctx, nil
}

// applyProjectConfig 프로젝트 설정(.gofar/gofar.yaml)이 있으면 빌드 대상과 바이너리별 정책을 결정한다
func applyProjectConfig(ctx *BuildContext) error {
	config, err := loadProjectConfig(ctx.ProjectBaseDir)
	if err != nil {
		return err
	}
	if config == nil {
		return nil
	}

	binaryNames := make([]string, 0, len(ctx.ProcessList))
	for _, record := range ctx.ProcessList {
		binaryNames = append(binaryNames, record.GetBinaryname())
	}

	plan, err := config.Resolve(binaryNames, ctx.ExposeProcessName, stripEnable)
	if err != nil {
		return err
	}
	for _, warning := range plan.Warnings {
		fmt.Printf("warning : %s\n", warning)
	}

	processList := make([]CmdRecord, 0, len(plan.Specs))
	for _, record := range ctx.ProcessList {
		if _, ok := plan.Specs[record.GetBinaryname()]; ok {
			processList = append(processList, record)
		}
	}

	ctx.ProjectConfig = config
	ctx.ExcludedBinaries = plan.Excluded
	ctx.ProcessList = processList
	ctx.binarySpecs = plan.Specs
	return nil
}

func determineResourceDir(ctx *BuildContext) {
	resourceDir := filepath.Join(ctx.ProjectBaseDir, resourceDirname)

	err := CheckDirExist(resourceDir)
	if err != nil {
		fmt.Errorf("ensure resource dir : %s", err.Error())
		return
	}

	ctx.ResourceDir = resourceDir
}

// find project base dir
func determineProjectBaseDir(ctx *BuildContext) error {
	currentWd, _ := os.Getwd()

	// check git support
	foundBaseDir, err := FindGitConfig(currentWd)
	if err == nil {
		ctx.ProjectBaseDir = foundBaseDir
		ctx.GitSupport = true
		return nil
	}

	if !errors.Is(err, errGitNotFound) {
		return fmt.Errorf("fail to check git support. %s", err.Error())
	}

	// git not found case
	// $GOPATH/src 하위에서 프로세스 이름으로 된 디렉토리를 찾는다
	foundBase := false
	for gopath, _ := range buildGopathMap() {
		gopathSrcDir := filepath.Join(gopath, "src")
		foundBaseDir, err := FindDirectory(gopathSrcDir, ctx.ExposeProcessName)
		if err == nil {
			ctx.ProjectBaseDir = foundBaseDir
			foundBase = true
			break
		}
	}

	if !foundBase {
		return fmt.Errorf("cannot find project base directory")
	}
	return nil
}

// find project base dir
func determineCmdList(ctx *BuildContext) {
	ctx.ProcessList = make([]CmdRecord, 0)

	foundDir, err := FindDirectory(ctx.ProjectBaseDir, cmdDirname)
	if err != nil {
		record := CmdRecord{}
		record.Path = ctx.ProjectBaseDir
		ctx.ProcessList = append(ctx.ProcessList, record)
		return
	}

	for _, cmd := range FindSubDirectories(foundDir) {
		record := CmdRecord{}
		record.Path = cmd
		ctx.ProcessList = append(ctx.ProcessList, record)
	}
}

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
	BuildCGOLink      string
	workingDir        string
	procType          string
	farPath           string
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
}

func (b *BuildContext) Packaging() error {
	var err error
	b.workingDir, err = os.MkdirTemp("", b.ExposeProcessName)
	if err != nil {
		return fmt.Errorf("fail to create tmp dir : %s", err.Error())
	}

	fmt.Printf("working directory : %s\n", b.workingDir)
	defer func() {
		os.RemoveAll(b.workingDir)
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
		wg := sync.WaitGroup{}
		wg.Add(len(buildPlatformList.Platforms))
		for _, platform := range buildPlatformList.Platforms {
			request := BinCompileRequest{}
			request.TargetDir = filepath.Join(b.workingDir, PlatformDirName, platform.getPlatformDirectory())
			request.BinName = cmdBinName
			request.BinSourcePath = cmdRecord.Path
			request.Os = platform.Os
			request.Arch = platform.Arch
			request.BuildCGOLink = b.BuildCGOLink
			go compileBinary(&wg, &compileError, request)
		}
		wg.Wait()

		if compileError > 0 {
			return fmt.Errorf("fail to prepare binary %s\n", cmdBinName)
		}
	}

	return nil
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
}

// compileBinary 바이너리를 컴파일한다
func compileBinary(wg *sync.WaitGroup, compileError *uint32, request BinCompileRequest) {
	defer wg.Done()

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
	if len(request.BuildCGOLink) == 0 {
		command = fmt.Sprintf("GOOS=%s GOARCH=%s go build -o %s", request.Os, request.Arch, targetBin)
	} else {
		command = fmt.Sprintf("CC=%s GOOS=%s GOARCH=%s CGO_ENABLED=1 go build -o %s -ldflags='-s -w'",
			request.BuildCGOLink, request.Os, request.Arch, targetBin)
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

func NewBuildContext(procName, cgoLink string) (*BuildContext, error) {
	loadPlatform()

	ctx := &BuildContext{}
	ctx.GitSupport = false
	ctx.ExposeProcessName = procName
	ctx.procType = procTypeGeneral
	if len(cgoLink) > 0 {
		ctx.BuildCGOLink = cgoLink
	}

	err := determineProjectBaseDir(ctx)
	if err != nil {
		return nil, fmt.Errorf("fail to build context. %s", err.Error())
	}

	determineResourceDir(ctx)
	determineCmdList(ctx)

	return ctx, nil
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

/*
 * //
 * // Licensed to the Apache Software Foundation (ASF) under one
 * // or more contributor license agreements.  See the NOTICE file
 * // distributed with p work for additional information
 * // regarding copyright ownership.  The ASF licenses p file
 * // to you under the Apache License, Version 2.0 (the
 * // "License"); you may not use p file except in compliance
 * // with the License.  You may obtain a copy of the License at
 * //
 * //   http://www.apache.org/licenses/LICENSE-2.0
 * //
 * // Unless required by applicable law or agreed to in writing,
 * // software distributed under the License is distributed on an
 * // "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * // KIND, either express or implied.  See the License for the
 * // specific language governing permissions and limitations
 * // under the License.
 * //
 * // @project fatima
 * // @author DeockJin Chung (jin.freestyle@gmail.com)
 * // @date 21. 8. 8. 오후 9:24
 * //
 */

package main

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
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
	ExposeProcessName string
	BuildOS           string
	BuildArc          string
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
		fmt.Printf("binary : %s\n", b.ExposeProcessName)
	}
	if len(b.BuildOS) > 0 {
		fmt.Printf("build : GOOS=%s GOARC=%s\n", b.BuildOS, b.BuildArc)
	}
}

func (b *BuildContext) Packaging() error {
	var err error
	b.workingDir, err = ioutil.TempDir("/tmp", b.ExposeProcessName)
	if err != nil {
		return fmt.Errorf("fail to create tmp dir : %s", err.Error())
	}

	log.Printf("working directory : %s\n", b.workingDir)
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
	return os.Getenv("GOPATH")
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
	err = Zipit(b.workingDir, b.farPath)
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
	//if len(cmdFlag.ExtraBin) > 0 {
	//	binNameList := make([]string, 0)
	//	for _, v := range cmdFlag.ExtraBin {
	//		binNameList = append(binNameList, filepath.Base(v))
	//	}
	//	m["extra_bin"] = binNameList
	//}
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
	gitBranch, err := ReadGitBranch(b.ProjectBaseDir)
	if err == nil {
		if len(gitBranch) > 0 {
			git := make(map[string]string)
			git["branch"] = gitBranch
			gitCommit := ReadGitCommit(b.ProjectBaseDir, gitBranch)
			git["commit"] = gitCommit
			fmt.Printf("\n>> mark build info : branch=%s, commit=%s\n", gitBranch, gitCommit)
			build["git"] = git
		}
	}

	m["build"] = build

	dat, err := json.Marshal(m)
	if err != nil {
		return fmt.Errorf("fail to create deployment : %s", err.Error())
	}

	depfile := filepath.Join(b.workingDir, "deployment.json")
	err = ioutil.WriteFile(depfile, dat, 0644)
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

	files, err := ioutil.ReadDir(baseDir)
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
		return b.preparePrecompiledBinary()
	}

	return b.prepareCmdRecordBinary()
}

func (b *BuildContext) preparePrecompiledBinary() error {
	var err error
	// check pre-compiled binary
	precompiledBin := filepath.Join(getGOPath(), "bin", b.ExposeProcessName)
	if len(b.BuildOS) > 0 {
		osArch := fmt.Sprintf("%s_%s", b.BuildOS, b.BuildArc)
		precompiledBin = filepath.Join(getGOPath(), "bin", osArch, b.ExposeProcessName)
	}
	err = CheckFileExist(precompiledBin)
	if err != nil {
		return fmt.Errorf("cannot find precompiled binary : %s", b.ExposeProcessName)
	}
	cmdRecord := CmdRecord{}
	cmdRecord.Path = precompiledBin
	fmt.Printf("using precompiled binary : %s (%s)\n", precompiledBin, getFileModtime(precompiledBin))

	// binary 복사
	targetBin := filepath.Join(b.workingDir, b.ExposeProcessName)
	err = CopyFile(precompiledBin, targetBin)
	if err != nil {
		return fmt.Errorf("fail to precompiled binary copy : %s\n", err.Error())
	}
	os.Chmod(targetBin, 0755)
	return nil
}

func (b *BuildContext) prepareCmdRecordBinary() error {
	// build process list
	for _, cmdRecord := range b.ProcessList {
		cmdBinName := cmdRecord.GetBinaryname()
		fmt.Printf("\n>> compiling %s...\n", cmdBinName)
		targetBin := filepath.Join(b.workingDir, cmdBinName)
		command := fmt.Sprintf("go build -o %s", targetBin)
		if len(b.BuildOS) > 0 {
			if len(b.BuildCGOLink) == 0 {
				command = fmt.Sprintf("GOOS=%s GOARCH=%s go build -o %s", b.BuildOS, b.BuildArc, targetBin)
			} else {
				command = fmt.Sprintf("CC=%s GOOS=%s GOARCH=%s CGO_ENABLED=1 go build -o %s -ldflags='-s -w'",
					b.BuildCGOLink, b.BuildOS, b.BuildArc, targetBin)
			}
		}
		fmt.Printf("%s\n", command)
		out, err := ExecuteShell(cmdRecord.Path, command)
		if err != nil {
			return fmt.Errorf("fail to execute command : %s\n%s\n", err.Error(), out)
		}
		if len(out) > 0 {
			return fmt.Errorf("fail to build binary %s\n%s\n", cmdBinName, out)
		}
		os.Chmod(targetBin, 0755)
	}

	return nil
}

func NewBuildContext(procName, osArc, cgoLink string) (*BuildContext, error) {
	ctx := &BuildContext{}
	ctx.ExposeProcessName = procName
	ctx.procType = procTypeGeneral
	if len(osArc) > 0 {
		tokenList := strings.Split(osArc, "_")
		if len(tokenList) != 2 {
			return nil, fmt.Errorf("invalid os arc (%s)", osArc)
		}
		ctx.BuildOS = strings.TrimSpace(tokenList[0])
		ctx.BuildArc = strings.TrimSpace(tokenList[1])
	}
	if len(cgoLink) > 0 {
		ctx.BuildCGOLink = cgoLink
	}

	var err error
	currentWd, _ := os.Getwd()
	ctx.ProjectBaseDir, err = determineProjectBaseDir(currentWd, procName)
	if err != nil {
		return nil, fmt.Errorf("fail to build context. %s", err.Error())
	}

	determineResourceDir(ctx)
	if err = determineCmdList(ctx); err != nil {
		//return nil, fmt.Errorf("fail to build context. %s", err.Error())
	}

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
func determineProjectBaseDir(baseDir, procName string) (string, error) {
	foundBaseDir, err := FindGitConfig(baseDir)
	if err == nil {
		return foundBaseDir, err
	}

	if err != errGitNotFound {
		return foundBaseDir, err
	}

	// search $GOPATH/src
	gopathSrcDir := filepath.Join(getGOPath(), "src")

	return FindDirectory(gopathSrcDir, procName)
}

// find project base dir
func determineCmdList(ctx *BuildContext) error {
	foundDir, err := FindDirectory(ctx.ProjectBaseDir, cmdDirname)
	if err != nil {
		return err
	}

	ctx.ProcessList = make([]CmdRecord, 0)
	for _, cmd := range FindSubDirectories(foundDir) {
		record := CmdRecord{}
		record.Path = cmd
		ctx.ProcessList = append(ctx.ProcessList, record)
	}

	return nil
}

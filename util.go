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
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"
)

func CheckDirExist(path string) error {
	stat, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("not exist dir : %s", path)
		}
		return fmt.Errorf("error checking : %s (%s)", path, err.Error())
	}

	if !stat.IsDir() {
		return fmt.Errorf("exist but it is not directory")
	}

	return nil
}

func EnsureFileInDirectory(dir, targetFilename string) bool {
	files, err := os.ReadDir(dir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fail to read dir %s : %s", dir, err.Error())
		return false
	}

	// find .git
	for _, file := range files {
		if file.Name() == targetFilename {
			if !file.IsDir() {
				return true
			}
			fmt.Fprintf(os.Stderr, "%s found but it is directory", dir)
			return false
		}
	}

	// not found
	return false
}

var errGopathNotFound = fmt.Errorf("not found GOPATH. (gopath is nil)")
var errGitNotFound = fmt.Errorf("not found git")

func FindGitConfig(dir string) (string, error) {
	gopath := os.Getenv("GOPATH")
	if len(gopath) == 0 {
		return "", errGopathNotFound
	}

	//gopathMap := buildGopathMap()
	// 여러개의 gopath 가 정의되어 있는 경우도 처리하도록 한다
	for gopath, _ := range buildGopathMap() {
		if dir == filepath.Join(gopath, "src") {
			return "", errGitNotFound
		}
	}

	files, err := os.ReadDir(dir)
	if err != nil {
		return "", err
	}

	// find .git
	for _, file := range files {
		if file.Name() == gitDirname {
			if file.IsDir() && EnsureFileInDirectory(filepath.Join(dir, file.Name()), gitConfigfile) {
				return dir, nil
			}
		}
	}

	parentDir := filepath.Dir(dir)
	return FindGitConfig(parentDir)
}

// buildGopathMap gopath 변수들을 set 형태로 구한다
func buildGopathMap() map[string]struct{} {
	tokens := strings.Split(os.Getenv("GOPATH"), fmt.Sprintf("%c", os.PathListSeparator))
	m := make(map[string]struct{})
	for _, token := range tokens {
		m[strings.TrimSpace(token)] = struct{}{}
	}
	return m
}

func FindDirectory(baseDir, targetDir string) (string, error) {
	files, err := os.ReadDir(baseDir)
	if err != nil {
		return "", err
	}

	nextPathList := make([]os.DirEntry, 0)
	for _, file := range files {
		if file.Name() == targetDir {
			if file.IsDir() {
				return filepath.Join(baseDir, targetDir), nil
			}
			continue
		}

		if !file.IsDir() {
			continue
		}

		nextPathList = append(nextPathList, file)
	}

	for _, nextPath := range nextPathList {
		foundDir, err := FindDirectory(filepath.Join(baseDir, nextPath.Name()), targetDir)
		if err == nil {
			return foundDir, nil
		}
	}

	return "", fmt.Errorf("%s not found", targetDir)
}

func FindSubDirectories(baseDir string) []string {
	list := make([]string, 0)
	files, err := os.ReadDir(baseDir)
	if err != nil {
		fmt.Fprintf(os.Stderr, "fail to read dir : %s", err.Error())
		return list
	}

	for _, file := range files {
		if file.IsDir() {
			list = append(list, filepath.Join(baseDir, file.Name()))
			continue
		}
	}

	return list
}

func getFileModtime(path string) string {
	info, err := os.Stat(path)
	if err != nil {
		return err.Error()
	}
	return info.ModTime().String()
}

func CheckFileExist(path string) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("not exist file : %s", path)
		}
	}

	return nil
}

func CopyFile(src string, dst string) error {
	fmt.Printf("copy : %s\n", src)
	sFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer sFile.Close()

	eFile, err := os.Create(dst)
	if err != nil {
		return err
	}
	defer eFile.Close()

	_, err = io.Copy(eFile, sFile) // first var shows number of bytes
	if err != nil {
		return err
	}

	err = eFile.Sync()
	if err != nil {
		return err
	}

	return nil
}

func ExecuteCommand(wd, command string) (string, error) {
	if len(command) == 0 {
		return "", errors.New("empty command")
	}

	var cmd *exec.Cmd
	s := regexp.MustCompile("\\s+").Split(command, -1)
	i := len(s)
	if i == 0 {
		return "", errors.New("empty command")
	} else if i == 1 {
		cmd = exec.Command(s[0])
	} else {
		cmd = exec.Command(s[0], s[1:]...)
	}

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Dir = wd
	err := cmd.Run()
	if err != nil {
		return out.String(), err
	}
	return out.String(), nil
}

func ExecuteShell(wd, command string) (string, error) {
	if len(command) == 0 {
		return "", errors.New("empty command")
	}

	var cmd *exec.Cmd
	cmd = exec.Command("/bin/sh", "-c", command)

	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	cmd.Dir = wd
	err := cmd.Run()
	if err != nil {
		return out.String(), err
	}
	return out.String(), nil
}

type fileMeta struct {
	Path  string
	IsDir bool
}

func ZipArtifact(baseDir, artifactFile string) error {
	var files []fileMeta
	err := filepath.Walk(baseDir, func(path string, info os.FileInfo, err error) error {
		files = append(files, fileMeta{Path: path, IsDir: info.IsDir()})
		return nil
	})
	if err != nil {
		return err
	}

	z, err := os.Create(artifactFile)
	if err != nil {
		return err
	}
	defer z.Close()

	zw := zip.NewWriter(z)
	defer zw.Close()

	for _, f := range files {
		path := f.Path

		if len(baseDir) == len(path) {
			path = ""
		} else if len(baseDir) < len(path) {
			path = fmt.Sprintf("%c%s", os.PathSeparator, path[len(baseDir)+1:])
		}

		if f.IsDir {
			path = fmt.Sprintf("%s%c", path, os.PathSeparator)
		}

		err = copyIntoZip(zw, path, f)
		if err != nil {
			return err
		}
	}

	return nil
}

func copyIntoZip(zw *zip.Writer, path string, f fileMeta) error {
	header := &zip.FileHeader{
		Name:     path,
		Method:   zip.Deflate,
		Modified: time.Now(),
	}

	// check platform support relative file
	if strings.HasPrefix(path, fmt.Sprintf("/%s", PlatformDirName)) {
		// platform support binary file should be set execute mode
		header.SetMode(0755)
	}

	w, err := zw.CreateHeader(header)
	if err != nil {
		return err
	}

	if f.IsDir {
		return nil
	}

	file, err := os.Open(f.Path)
	if err != nil {
		return err
	}
	defer file.Close()

	_, err = io.Copy(w, file)
	return err
}

func EnsureDirectory(dir string) error {
	if _, err := os.Stat(dir); err != nil {
		if os.IsNotExist(err) {
			return os.MkdirAll(dir, 0755)
		}
	}

	return nil
}

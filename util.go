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
	"archive/zip"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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
	files, err := ioutil.ReadDir(dir)
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

	if dir == filepath.Join(gopath, "src") {
		return "", errGitNotFound
	}

	files, err := ioutil.ReadDir(dir)
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

func FindDirectory(baseDir, targetDir string) (string, error) {
	files, err := ioutil.ReadDir(baseDir)
	if err != nil {
		return "", err
	}

	nextPathList := make([]os.FileInfo, 0)
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
	files, err := ioutil.ReadDir(baseDir)
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
	//s := regexp.MustCompile("\\s+").Split(command, -1)
	//i := len(s)
	//if i == 0 {
	//	return "", errors.New("empty command")
	//} else if i == 1 {
	//	cmd = exec.Command(s[0])
	//} else {
	//	cmd = exec.Command(s[0], s[1:]...)
	//}

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

func ReadGitBranch(baseDir string) (string, error) {
	headFile := filepath.Join(baseDir, ".git", "HEAD")

	dat, err := ioutil.ReadFile(headFile)
	if err != nil {
		return "", fmt.Errorf("not found git head")
	}

	head := strings.Trim(string(dat), " \r\n\t")
	idx := strings.LastIndex(head, "/")
	return head[idx+1:], nil
}

func ReadGitCommit(baseDir string, branch string) string {
	commitFile := filepath.Join(baseDir, ".git", "refs", "heads", branch)
	dat, err := ioutil.ReadFile(commitFile)
	if err != nil {
		return ""
	}

	return string(dat)[:12]
}

func Zipit(source, target string) error {
	zipfile, err := os.Create(target)
	if err != nil {
		return err
	}
	defer zipfile.Close()

	archive := zip.NewWriter(zipfile)
	defer archive.Close()

	_, err = os.Stat(source)
	if err != nil {
		return nil
	}

	var baseDir string
	//if info.IsDir() {
	//	baseDir = filepath.Base(source)
	//}

	filepath.Walk(source, func(path string, info os.FileInfo, err error) error {
		//fmt.Printf("zip path : %s\n", path)
		if source == path {
			return nil
		}

		if err != nil {
			return err
		}

		header, err := zip.FileInfoHeader(info)
		if err != nil {
			return err
		}

		if baseDir != "" {
			header.Name = filepath.Join(baseDir, strings.TrimPrefix(path, source))
		}

		if info.IsDir() {
			header.Name += "/"
		} else {
			header.Method = zip.Deflate
		}

		writer, err := archive.CreateHeader(header)
		if err != nil {
			return err
		}

		if info.IsDir() {
			return nil
		}

		file, err := os.Open(path)
		if err != nil {
			return err
		}
		defer file.Close()
		_, err = io.Copy(writer, file)
		return err
	})

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
